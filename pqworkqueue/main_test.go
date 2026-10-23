package pqworkqueue

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gobuffalo/nulls"
	"github.com/jmoiron/sqlx"
	"github.com/ntbosscher/gobase/er"
	"github.com/ntbosscher/gobase/jv"
	"github.com/ntbosscher/gobase/model"
)

func TestBasic(t *testing.T) {
	model.SetStructNameMapping(model.SnakeCaseStructNameMapping)

	debounceCounter := atomic.Int32{}

	StartWorkers(&WorkerInfo{
		QueueName: "test-queue",
		Callback: func(ctx context.Context, input json.RawMessage, _ WorkerJobMeta) []byte {
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
			status, err = GetStatus(ctx, GetStatusInput{ID: id})
			return err
		}))

		if status.CompletedAt.Valid {

			var data []byte

			er.Check(model.WithTx(context.Background(), func(ctx context.Context, tx *sqlx.Tx) error {
				data, err = GetResult(ctx, GetResultInput{ID: id})
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

func TestMerge(t *testing.T) {
	model.SetStructNameMapping(model.SnakeCaseStructNameMapping)

	type Arg struct {
		Name string
		IDs  []int
	}

	var queue = NewQueue2[*Arg]("testing_queue")

	add := func(key string, arg *Arg) {
		defer er.HandleErrors(func(input *er.HandlerInput) {
			fmt.Println(input)
		})

		ctx := context.Background()

		er.Check(model.WithTx(ctx, func(ctx context.Context, tx *sqlx.Tx) error {
			queue.MustAddOpt(ctx, arg, &AddOption2[*Arg]{
				DebounceKey: key,
				StartAfter:  time.Now().Add(1 * time.Second),
				DebounceMerge: func(exist *Arg, new *Arg) (*Arg, error) {
					exist.IDs = jv.Unique(append(exist.IDs, new.IDs...))
					return exist, nil
				},
			})

			return nil
		}))
	}

	result := make(chan *Arg)

	queue.RegisterWorker(1, func(ctx context.Context, arg *Arg) []byte {
		t.Log("worker", arg.Name, arg.IDs)
		result <- arg
		return nil
	})

	add("a", &Arg{
		Name: "A",
		IDs:  []int{1},
	})

	add("b", &Arg{
		Name: "B",
		IDs:  []int{3},
	})

	add("a", &Arg{
		Name: "A",
		IDs:  []int{2},
	})

	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()

	for i := 0; i < 2; i++ {
		select {
		case out := <-result:
			if out.Name == "A" {
				if !jv.ArrayItemCompare(out.IDs, []int{1, 2}) {
					t.Error("unexpected merge", out.IDs)
				}
			} else if out.Name == "B" {
				if !jv.ArrayItemCompare(out.IDs, []int{3}) {
					t.Error("unexpected merge", out.IDs)
				}
			}
		case <-ctx.Done():
			t.Fatal("timed out")
		}
	}

}

func TestDeadlockLockWait(t *testing.T) {
	model.SetStructNameMapping(model.SnakeCaseStructNameMapping)

	StartWorkers(&WorkerInfo{
		QueueName: "test-deadlock-lock-wait",
		Callback: func(ctx context.Context, input json.RawMessage, meta WorkerJobMeta) []byte {
			// the claim transaction holds the lock on this row
			er.Check(model.ExecContext(ctx, `update pq_worker_queue set debounce_key = 'x' where id = $1`, meta.ID))
			return []byte("result")
		},
	})

	q := NewQueue("test-deadlock-lock-wait")

	id := ""
	er.Check(model.WithTx(context.Background(), func(ctx context.Context, tx *sqlx.Tx) error {
		id = q.MustAdd(ctx, "arg")
		return nil
	}))

	deadline := time.Now().Add(3 * DeadlockCheckInterval)
	tc := time.NewTicker(100 * time.Millisecond)
	defer tc.Stop()

	for range tc.C {
		commitErr := nulls.String{}
		er.Check(model.WithTx(context.Background(), func(ctx context.Context, tx *sqlx.Tx) error {
			return model.GetContext(ctx, &commitErr, `select commit_error from pq_worker_queue where id = $1`, id)
		}))

		if commitErr.Valid {
			if !strings.Contains(commitErr.String, ErrDeadlockLockWait.Error()) {
				t.Fatal("unexpected commit error", commitErr.String)
			}

			break
		}

		if deadline.Before(time.Now()) {
			t.Fatal("missed deadline")
		}
	}
}
