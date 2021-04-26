package pqworkqueue

import (
	"context"
	"encoding/json"
	"github.com/jmoiron/sqlx"
	"github.com/ntbosscher/gobase/er"
	"github.com/ntbosscher/gobase/model"
	"testing"
	"time"
)

func TestBasic(t *testing.T) {
	model.SetStructNameMapping(model.SnakeCaseStructNameMapping)

	StartWorkers(&WorkerInfo{
		QueueName: "test-queue",
		Callback: func(ctx context.Context, id string, input json.RawMessage) []byte {
			info := ""
			err := json.Unmarshal(input, &info)
			if err != nil {
				t.Fatal(err)
				return nil
			}

			if info != "test-info" {
				t.Fatal("invalid info", info)
			}

			<-time.After(200 * time.Millisecond)

			return []byte("result")
		},
	})

	q := NewQueue("test-queue")

	var err error
	var id string
	er.Check(model.WithTx(context.Background(), func(ctx context.Context, tx *sqlx.Tx) error {
		id, err = q.Add(ctx, "test-info")
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
}
