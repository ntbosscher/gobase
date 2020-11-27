package squtil

import (
	"context"
	"database/sql"
	sq "github.com/Masterminds/squirrel"
	"github.com/ntbosscher/gobase/model"
	"log"
)

func verboseLog(err error, query string, args ...interface{}) {
	if !model.EnableVerboseLogging {
		return
	}

	if err != nil {
		log.Println("query failed: " + err.Error())
	}

	log.Println("query input: ", query, args)
}

// SelectContext runs a query created with model.Builder.Select and scans the resulting
// rows into dest.
// Dest must be a pointer to a array-type (e.g. *[]*Person)
func SelectContext(ctx context.Context, dest interface{}, qr sq.Sqlizer) error {
	sqlStr, args, err := qr.ToSql()
	if err != nil {
		verboseLog(err, sqlStr, args...)
		return err
	}

	verboseLog(err, sqlStr, args...)
	return model.SelectContext(ctx, dest, sqlStr, args...)
}

// QueryRowContext runs the query and expects exactly 1 row. The results can be collected
// by calling .Scan() on the result
func QueryRowContext(ctx context.Context, qr sq.Sqlizer) *sql.Row {
	sqlStr, args, err := qr.ToSql()
	if err != nil {
		verboseLog(err, sqlStr, args...)
	}

	return model.QueryRowContext(ctx, sqlStr, args...)
}

// GetContext runs the query expecting exactly 1 resulting row. That row is scanned into dest.
func GetContext(ctx context.Context, dest interface{}, qr sq.Sqlizer) error {
	sqlStr, args, err := qr.ToSql()
	if err != nil {
		verboseLog(err, sqlStr, args...)
		return err
	}

	return model.GetContext(ctx, dest, sqlStr, args...)
}

// ExecContext runs the query without expecting any output
func ExecContext(ctx context.Context, qr sq.Sqlizer) error {
	sqlStr, args, err := qr.ToSql()
	if err != nil {
		verboseLog(err, sqlStr, args...)
		return err
	}

	return model.ExecContext(ctx, sqlStr, args...)
}

// Insert works just like ExecContext except that it returns the inserted id
//
// Insert uses QueryRow for postgres b/c the driver doesn't support .LastInsertId()
// For your postgres insert query, be sure to include "returning <id-column>"
func Insert(ctx context.Context, qr sq.Sqlizer) (id int64, err error) {
	sqlStr, args, err := qr.ToSql()
	if err != nil {
		verboseLog(err, sqlStr, args...)
		return 0, err
	}

	return model.Insert(ctx, sqlStr, args...)
}
