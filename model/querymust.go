package model

import (
	"context"

	"github.com/ntbosscher/gobase/er"
)

func MustExecContext(ctx context.Context, query string, params ...interface{}) {
	err := ExecContext(ctx, query, params...)
	reportTxError(ctx, err)
	er.Check(err)
}

func MustInsert(ctx context.Context, query string, params ...interface{}) int64 {
	id, err := Insert(ctx, query, params...)
	reportTxError(ctx, err)
	er.Check(err)
	return id
}

func MustQueryRowContext(ctx context.Context, query string, params ...interface{}) *Row {
	rw := QueryRowContext(ctx, query, params...)
	rw.must = true
	return rw
}

func MustSelectContext(ctx context.Context, dest interface{}, query string, params ...interface{}) {
	err := SelectContext(ctx, dest, query, params...)
	reportTxError(ctx, err)
	er.Check(err)
}

func MustGetContext(ctx context.Context, dest interface{}, query string, params ...interface{}) {
	err := GetContext(ctx, dest, query, params...)
	reportTxError(ctx, err)
	er.Check(err)
}
