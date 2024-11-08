package pqworkqueue

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/ntbosscher/gobase/er"
	"github.com/ntbosscher/gobase/model"
)

func TestBasic(t *testing.T) {
	model.SetStructNameMapping(model.SnakeCaseStructNameMapping)

	debounceCounter := atomic.Int32{}

	StartWorkers(&WorkerInfo{
		QueueName: "test-queue",
		Callback: func(ctx context.Context, id string, input json.RawMessage) []byte {
			info := ""
			err := json.Unmarshal(input, &info)
			if err != nil {
				t.Fatal(err)
				return nil
			}

			if info == "test-info" {
				<-time.After(200 * time.Millisecond)
				return []byte("result")
			}

			if info == "demo" {
				debounceCounter.Add(1)
				<-time.After(200 * time.Millisecond)
				return []byte("result")
			}

			t.Fatal("unexpected info", info)
			return nil
		},
	})

	q := NewQueue("test-queue")

	var err error
	var id string
	er.Check(model.WithTx(context.Background(), func(ctx context.Context, tx *sqlx.Tx) error {
		id, err = q.Add(ctx, "test-info")

		for i := 0; i < 10; i++ {
			q.MustAddOpt(ctx, "demo", &AddOption{
				DebounceKey:               "test",
				DebounceKeepOriginalStart: false,
			})
		}

		return err
	}))

	t.Log("added", id)

	deadline := time.Now().Add(10 * time.Second)
	tc := time.NewTicker(100 * time.Millisecond)
	defer tc.Stop()

	for range tc.C {
		var status *Status
		er.Check(model.WithTx(context.Background(), func(ctx context.Context, tx *sqlx.Tx) error {
			status, err = GetStatus(ctx, id)
			return err
		}))

		if status.CompletedAt.Valid {

			var data []byte

			er.Check(model.WithTx(context.Background(), func(ctx context.Context, tx *sqlx.Tx) error {
				data, err = GetResult(ctx, id)
				return err
			}))

			if data == nil {
				t.Fatal("missing result")
			}

			if string(data) != "result" {
				t.Fatal("incorrect result", data)
			}

			break
		}

		if deadline.Before(time.Now()) {
			t.Fatal("missed deadline")
		}
	}

	value := debounceCounter.Load()
	if value != 1 {
		t.Fatal("unexpected debounce count", value)
	}
}
