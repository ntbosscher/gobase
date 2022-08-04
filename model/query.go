package model

import (
	"context"
	"database/sql"
)

// ExecContext runs the query without expecting any output
func ExecContext(ctx context.Context, query string, args ...interface{}) error {
	after := preHook(ctx, "ExecContext", query, args)
	_, err := Tx(ctx).ExecContext(ctx, query, args...)
	after(err)

	verboseLog(err, query, args...)
	return err
}

// Insert works just like ExecContext except that it returns the inserted id
//
// Insert uses QueryRow for postgres b/c the driver doesn't support .LastInsertId()
// For your postgres insert query, be sure to include "returning <id-column>"
func Insert(ctx context.Context, query string, args ...interface{}) (id int64, err error) {

	if defaultDbType == "postgres" {
		after := preHook(ctx, "Insert", query, args)
		err = Tx(ctx).QueryRowContext(ctx, query, args...).Scan(&id)
		after(err)

		if err != nil {
			verboseLog(err, query, args...)
			return
		}

		return
	}

	after := preHook(ctx, "Insert", query, args)
	result, err := Tx(ctx).ExecContext(ctx, query, args...)
	if err != nil {
		after(err)
		verboseLog(err, query, args...)
		return
	}

	id, err = result.LastInsertId()
	after(err)

	verboseLog(err, query, args...)
	return
}

// GetContext runs the query expecting exactly 1 resulting row. That row is scanned into dest.
func GetContext(ctx context.Context, dest interface{}, query string, args ...interface{}) error {
	after := preHook(ctx, "GetContext", query, args)
	err := Tx(ctx).GetContext(ctx, dest, query, args...)
	after(err)
	verboseLog(err, query, args...)
	return err
}

// SelectContext runs the query and scans the resulting rows into dest.
// Dest must be a pointer to a array-type (e.g. *[]*Person)
func SelectContext(ctx context.Context, dest interface{}, sql string, args ...interface{}) error {
	after := preHook(ctx, "SelectContext", sql, args)
	err := Tx(ctx).SelectContext(ctx, dest, sql, args...)
	if err != nil {
		after(err)
		verboseLog(err, sql, args...)
		return err
	}

	after(err)
	verboseLog(err, sql, args...)
	return nil
}

// QueryRowContext runs the query and expects exactly 1 row. The results can be collected
// by calling .Scan() on the result
func QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row {
	after := preHook(ctx, "QueryRowContext", query, args)
	verboseLog(nil, query, args...)
	rw := Tx(ctx).QueryRowContext(ctx, query, args...)
	after(nil)
	return rw
}
