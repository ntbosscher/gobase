package pqworkqueue

import (
	"context"
	"encoding/json"

	"github.com/ntbosscher/gobase/er"
)

func NewQueue2[T any](name string) *Queue2[T] {
	return &Queue2[T]{
		Queue: NewQueue(name),
	}
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

func (q *Queue2[T]) MustAddOpt(ctx context.Context, arg T, opt *AddOption) string {
	return q.Queue.MustAddOpt(ctx, arg, opt)
}

func (q *Queue2[T]) AddOpt(ctx context.Context, arg T, opt *AddOption) (string, error) {
	return q.Queue.AddOpt(ctx, arg, opt)
}

type callbackFx[T any] func(ctx context.Context, arg T) []byte
type configUpdateFx func(*WorkerInfo)

func (q *Queue2[T]) RegisterWorker(concurrent int, callback callbackFx[T], updateConfig ...configUpdateFx) {

	config := &WorkerInfo{
		QueueName:   q.name,
		NConcurrent: concurrent,
		Callback: func(ctx context.Context, id string, input json.RawMessage) (out []byte) {
			defer er.HandleErrors(func(input *er.HandlerInput) {
				out, _ = json.Marshal(map[string]interface{}{
					"error": input.Error,
					"stack": input.StackTrace,
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
