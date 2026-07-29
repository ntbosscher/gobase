package pqworkqueue

import (
	"context"
	"encoding/json"
	"time"

	"github.com/gobuffalo/nulls"
	"github.com/ntbosscher/gobase/er"
)

func NewQueue2[T any](name string) *Queue2[T] {
	return &Queue2[T]{
		Queue: NewQueue(name),
	}
}

type AddOption2[T any] struct {
	// StartAfter schedules the job to start after the given time
	StartAfter time.Time

	// DebounceKey combined with QueueName are used to skip duplicate jobs
	// if DebounceKey is empty, no debouncing will be done
	DebounceKey string

	// DebounceKeepOriginalStart if true, will keep the original startAfter if another (QueueName,DebounceKey) exists
	// if false, the new deadline will replace the old one
	DebounceKeepOriginalStart bool

	DebounceMerge func(a T, b T) (T, error)

	// User optionally scopes the job to a user for multi-tenant systems.
	// Leave zero (invalid) for single-tenant setups.
	User nulls.Int

	// Company optionally scopes the job to a company/tenant for multi-tenant systems.
	// Leave zero (invalid) for single-tenant setups.
	Company nulls.Int
}

type Queue2[T any] struct {
	*Queue
}

func (q *Queue2[T]) MustAdd(ctx context.Context, arg T) string {
	return q.Queue.MustAdd(ctx, arg)
}

func (q *Queue2[T]) Add(ctx context.Context, arg T) (string, error) {
	return q.Queue.Add(ctx, arg)
}

func (q *Queue2[T]) MustAddOpt(ctx context.Context, arg T, opt *AddOption2[T]) string {
	return q.Queue.MustAddOpt(ctx, arg, q.convertOpt(opt))
}

func (q *Queue2[T]) AddOpt(ctx context.Context, arg T, opt *AddOption2[T]) (string, error) {
	return q.Queue.AddOpt(ctx, arg, q.convertOpt(opt))
}

type callbackFx[T any] func(ctx context.Context, arg T) []byte
type configUpdateFx func(*WorkerInfo)

func (q *Queue2[T]) RegisterWorker(concurrent int, callback callbackFx[T], updateConfig ...configUpdateFx) {

	config := &WorkerInfo{
		QueueName:   q.name,
		NConcurrent: concurrent,
		// The worker owns its transaction (the ctx passed to `callback` is inside a model tx). If the
		// callback panics, we recover it here and record the error as the job's result — the job is
		// marked complete and is NOT retried. Because the panic is recovered here (rather than
		// propagating out to roll back the tx), any work the callback committed, or wrote before
		// panicking, stays committed. It's the worker's responsibility to keep its own transaction
		// consistent (e.g. roll back explicitly on partial failure) if that matters.
		Callback: func(ctx context.Context, input json.RawMessage, _ WorkerJobMeta) (out []byte) {
			defer er.HandleErrors(func(input *er.HandlerInput) {
				// Full details are logged server-side under a correlation id;
				// the persisted result (readable via GetResult) carries only the
				// safe view by default. See er.SafeError.
				safe := er.SafeError(input)
				out, _ = json.Marshal(map[string]interface{}{
					"error":         safe.Error(),
					"stack":         safe.ClientStack,
					"correlationId": safe.CorrelationID,
				})
			})

			var arg T
			er.Check(json.Unmarshal(input, &arg))

			return callback(ctx, arg)
		},
	}

	for _, updater := range updateConfig {
		updater(config)
	}

	NewWorkerGroup(config)
}

func (q *Queue2[T]) convertOpt(opt *AddOption2[T]) *AddOption {
	var debounceMerge func(a, b []byte) ([]byte, error)

	if opt.DebounceMerge != nil {
		debounceMerge = func(a, b []byte) ([]byte, error) {
			var argA, argB T

			if err := json.Unmarshal(a, &argA); err != nil {
				return nil, err
			}

			if err := json.Unmarshal(b, &argB); err != nil {
				return nil, err
			}

			up, err := opt.DebounceMerge(argA, argB)
			if err != nil {
				return nil, err
			}

			out, err := json.Marshal(up)
			if err != nil {
				return nil, err
			}

			return out, nil
		}
	}

	return &AddOption{
		StartAfter:                opt.StartAfter,
		DebounceKey:               opt.DebounceKey,
		DebounceKeepOriginalStart: opt.DebounceKeepOriginalStart,
		DebounceMerge:             debounceMerge,
		User:                      opt.User,
		Company:                   opt.Company,
	}
}
