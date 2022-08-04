package model

import (
	"context"
	"time"
)

func preHook(ctx context.Context, method string, query string, args []interface{}) (after func(err error)) {
	v := ctx.Value(hookContextKey)
	if v == nil {
		return func(err error) {
			// noop
		}
	}

	start := time.Now()

	return func(err error) {
		end := time.Now()

		fx := v.(Hook)
		fx(ctx, method, query, args, err, start, end)
	}
}

type Hook func(ctx context.Context, method string, query string, args []interface{}, resultError error, start time.Time, end time.Time)
type hookContextKeyType string

const hookContextKey hookContextKeyType = "hook-key"

func SetHook(ctx context.Context, hook Hook) context.Context {
	return context.WithValue(ctx, hookContextKey, hook)
}
