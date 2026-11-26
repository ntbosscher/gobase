package pqprogress

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/ntbosscher/gobase/er"
	"github.com/ntbosscher/gobase/model"
	"github.com/ntbosscher/gobase/pqworkqueue"
)

func TestMain(m *testing.M) {
	model.EnableForTestingMode()
	os.Exit(m.Run())
}

func TestSetGet(t *testing.T) {
	model.SetStructNameMapping(model.SnakeCaseStructNameMapping)

	reported := make(chan struct{})
	release := make(chan struct{})

	q := pqworkqueue.NewQueue2[string]("test-pqprogress")
	q.RegisterWorker(1, func(ctx context.Context, arg string) []byte {
		Set(ctx, "working", 150)
		close(reported)
		<-release
		return []byte("result")
	}, Middleware())

	id := ""
	er.Check(model.WithTx(context.Background(), func(ctx context.Context, tx *sqlx.Tx) error {
		id = q.MustAdd(ctx, "arg")
		return nil
	}))

	select {
	case <-reported:
	case <-time.After(10 * time.Second):
		t.Fatal("job did not start")
	}

	p := waitForProgress(t, id, func(p *Progress) bool { return p != nil })
	if p.Description != "working" || p.Percent != 100 {
		t.Fatal("unexpected progress", p.Description, p.Percent)
	}

	close(release)

	deadline := time.Now().Add(10 * time.Second)
	tc := time.NewTicker(100 * time.Millisecond)
	defer tc.Stop()

	for range tc.C {
		var status *pqworkqueue.Status
		er.Check(model.WithTx(context.Background(), func(ctx context.Context, tx *sqlx.Tx) error {
			var err error
			status, err = pqworkqueue.GetStatus(ctx, pqworkqueue.GetStatusInput{ID: id})
			return err
		}))

		if status.CompletedAt.Valid {
			break
		}

		if deadline.Before(time.Now()) {
			t.Fatal("missed deadline")
		}
	}

	waitForProgress(t, id, func(p *Progress) bool { return p == nil })
}

// waitForProgress polls Get until ok accepts the report, because the reports are written in the background.
func waitForProgress(t *testing.T, id string, ok func(p *Progress) bool) *Progress {
	deadline := time.Now().Add(5 * time.Second)

	for {
		p, err := Get(context.Background(), id)
		er.Check(err)

		if ok(p) {
			return p
		}

		if deadline.Before(time.Now()) {
			t.Fatal("unexpected progress before the deadline", p)
		}

		<-time.After(50 * time.Millisecond)
	}
}
