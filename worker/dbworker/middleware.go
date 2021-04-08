package dbworker

import (
	"context"
	"github.com/jmoiron/sqlx"
	"github.com/ntbosscher/gobase/model"
	"github.com/ntbosscher/gobase/worker"
)

// Middleware executes the worker in a database transaction
func Middleware(next worker.Exec) worker.Exec {
	return func(ctx context.Context, input int) error {
		return model.WithTx(ctx, func(ctx context.Context, tx *sqlx.Tx) error {
			return next(ctx, input)
		})
	}
}
