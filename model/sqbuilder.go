package model

import (
	"context"
	"database/sql"
	sq "github.com/Masterminds/squirrel"
	"github.com/ntbosscher/gobase/model/squtil"
	"log"
)

var EnableVerboseLogging = false

func verboseLog(err error, query string, args ...interface{}) {
	if !EnableVerboseLogging {
		return
	}

	if err != nil {
		log.Println("query failed: " + err.Error())
	}
	log.Println("query input: ", query, args)
}

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

func GetContext(ctx context.Context, dest interface{}, query string, args ...interface{}) error {
	err := Tx(ctx).GetContext(ctx, dest, query, args...)
	verboseLog(err, query, args...)
	return err
}

func SelectContextString(ctx context.Context, dest interface{}, sql string, args ...interface{}) error {
	err := Tx(ctx).SelectContext(ctx, dest, sql, args...)
	if err != nil {
		verboseLog(err, sql, args...)
		return err
	}

	verboseLog(err, sql, args...)
	return nil
}

func SelectContext(ctx context.Context, dest interface{}, qr sq.Sqlizer) error {
	sql, args, err := qr.ToSql()
	if err != nil {
		verboseLog(err, sql, args...)
		return err
	}

	verboseLog(err, sql, args...)
	return SelectContextString(ctx, dest, sql, args...)
}

func QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row {
	verboseLog(nil, query, args...)
	return Tx(ctx).QueryRowContext(ctx, query, args...)
}

var Builder = sq.StatementBuilder.PlaceholderFormat(sq.Dollar)

// UnionBuilder is a (rather hack) implementation of Unions for squirrel query builder. They
// currently don't offer this feature. When they do, this code should be trashed
func UnionBuilder(a sq.SelectBuilder, b sq.SelectBuilder) squtil.UnionBuilder {
	return squtil.Union(a, b)
}
