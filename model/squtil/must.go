package squtil

import (
	"context"
	sq "github.com/Masterminds/squirrel"
	"github.com/ntbosscher/gobase/er"
	"github.com/ntbosscher/gobase/model"
)

// MustSelectContext runs a query created with model.Builder.Select and scans the resulting
// rows into dest.
// Dest must be a pointer to a array-type (e.g. *[]*Person)
func MustSelectContext(ctx context.Context, dest interface{}, qr sq.Sqlizer) {
	sqlStr, args, err := qr.ToSql()
	if err != nil {
		verboseLog(err, sqlStr, args...)
		er.Check(err)
	}

	verboseLog(err, sqlStr, args...)
	model.MustSelectContext(ctx, dest, sqlStr, args...)
}

// MustQueryRowContext runs the query and expects exactly 1 row. The results can be collected
// by calling .Scan() on the result
func MustQueryRowContext(ctx context.Context, qr sq.Sqlizer) *model.MustQueryRowResult {
	sqlStr, args, err := qr.ToSql()
	if err != nil {
		verboseLog(err, sqlStr, args...)
		er.Check(err)
	}

	return model.MustQueryRowContext(ctx, sqlStr, args...)
}

// GetContext runs the query expecting exactly 1 resulting row. That row is scanned into dest.
func MustGetContext(ctx context.Context, dest interface{}, qr sq.Sqlizer) {
	sqlStr, args, err := qr.ToSql()
	if err != nil {
		verboseLog(err, sqlStr, args...)
		er.Check(err)
	}

	model.MustGetContext(ctx, dest, sqlStr, args...)
}

// MustExecContext runs the query without expecting any output
func MustExecContext(ctx context.Context, qr sq.Sqlizer) {
	sqlStr, args, err := qr.ToSql()
	if err != nil {
		verboseLog(err, sqlStr, args...)
		er.Check(err)
	}

	model.MustExecContext(ctx, sqlStr, args...)
}

// MustInsert works just like ExecContext except that it returns the inserted id
//
// MustInsert uses QueryRow for postgres b/c the driver doesn't support .LastInsertId()
// For your postgres insert query, be sure to include "returning <id-column>"
func MustInsert(ctx context.Context, qr sq.Sqlizer) (id int64) {
	sqlStr, args, err := qr.ToSql()
	if err != nil {
		verboseLog(err, sqlStr, args...)
		er.Check(err)
	}

	return model.MustInsert(ctx, sqlStr, args...)
}
