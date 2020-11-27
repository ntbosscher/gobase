package model

import (
	"context"
	"database/sql"
	"github.com/ntbosscher/gobase/er"
)

func MustExecContext(ctx context.Context, query string, params ...interface{}) {
	err := ExecContext(ctx, query, params...)
	er.Check(err)
}

func MustInsert(ctx context.Context, query string, params ...interface{}) int64 {
	id, err := Insert(ctx, query, params...)
	er.Check(err)
	return id
}

type MustQueryRowResult struct {
	row *sql.Row
}

func (q *MustQueryRowResult) Scan(dest ...interface{}) {
	err := q.row.Scan(dest...)
	er.Check(err)
}

func MustQueryRowContext(ctx context.Context, query string, params ...interface{}) *MustQueryRowResult {
	return &MustQueryRowResult{
		row: QueryRowContext(ctx, query, params...),
	}
}

func MustSelectContext(ctx context.Context, dest interface{}, query string, params ...interface{}) {
	err := SelectContext(ctx, dest, query, params...)
	er.Check(err)
}

func MustGetContext(ctx context.Context, dest interface{}, query string, params ...interface{}) {
	err := GetContext(ctx, dest, query, params...)
	er.Check(err)
}
