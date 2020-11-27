package model

import (
	"context"
	"database/sql"
)

// ExecContext runs the query without expecting any output
func ExecContext(ctx context.Context, query string, args ...interface{}) error {
	_, err := Tx(ctx).ExecContext(ctx, query, args...)
	verboseLog(err, query, args...)
	return err
}

// Insert works just like ExecContext except that it returns the inserted id
//
// Insert uses QueryRow for postgres b/c the driver doesn't support .LastInsertId()
// For your postgres insert query, be sure to include "returning <id-column>"
func Insert(ctx context.Context, query string, args ...interface{}) (id int64, err error) {

	if dbType == "postgres" {
		err = Tx(ctx).QueryRowContext(ctx, query, args...).Scan(&id)
		if err != nil {
			verboseLog(err, query, args...)
			return
		}

		return
	}

	result, err := Tx(ctx).ExecContext(ctx, query, args...)
	if err != nil {
		verboseLog(err, query, args...)
		return
	}

	id, err = result.LastInsertId()
	verboseLog(err, query, args...)
	return
}

// GetContext runs the query expecting exactly 1 resulting row. That row is scanned into dest.
func GetContext(ctx context.Context, dest interface{}, query string, args ...interface{}) error {
	err := Tx(ctx).GetContext(ctx, dest, query, args...)
	verboseLog(err, query, args...)
	return err
}

// SelectContext runs the query and scans the resulting rows into dest.
// Dest must be a pointer to a array-type (e.g. *[]*Person)
func SelectContext(ctx context.Context, dest interface{}, sql string, args ...interface{}) error {
	err := Tx(ctx).SelectContext(ctx, dest, sql, args...)
	if err != nil {
		verboseLog(err, sql, args...)
		return err
	}

	verboseLog(err, sql, args...)
	return nil
}

// QueryRowContext runs the query and expects exactly 1 row. The results can be collected
// by calling .Scan() on the result
func QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row {
	verboseLog(nil, query, args...)
	return Tx(ctx).QueryRowContext(ctx, query, args...)
}
