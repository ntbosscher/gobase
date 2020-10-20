package model

import (
	"context"
	"database/sql"
	"github.com/Masterminds/squirrel"
	"github.com/ntbosscher/gobase/er"
)

func MustExecContext(ctx context.Context, query string, params ...interface{}) {
	err := ExecContext(ctx, query, params...)
	er.Check(err)
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

func MustSelectContext(ctx context.Context, dest interface{}, query squirrel.SelectBuilder) {
	err := SelectContext(ctx, dest, query)
	er.Check(err)
}

func MustSelectContextString(ctx context.Context, dest interface{}, query string, params ...interface{}) {
	err := SelectContextString(ctx, dest, query, params...)
	er.Check(err)
}

func MustGetContext(ctx context.Context, dest interface{}, query string, params ...interface{}) {
	err := GetContext(ctx, dest, query, params...)
	er.Check(err)
}
