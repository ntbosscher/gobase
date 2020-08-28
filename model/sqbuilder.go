package model

import (
	"context"
	sq "github.com/Masterminds/squirrel"
	"log"
)

var EnableVerboseLogging = false

func verboseLog(err error, query string, args ...interface{}) {
	if !EnableVerboseLogging {
		return
	}

	log.Println("query failed: " + err.Error())
	log.Println("query input: ", query, args)
}

func ExecContext(ctx context.Context, query string, args ...interface{}) error {
	_, err := Tx(ctx).ExecContext(ctx, query, args...)
	verboseLog(err, query, args...)
	return err
}

func Insert(ctx context.Context, query string, args ...interface{}) (id int64, err error) {
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

	return nil
}

func SelectContext(ctx context.Context, dest interface{}, qr sq.SelectBuilder) error {
	sql, args, err := qr.ToSql()
	if err != nil {
		log.Println(err)
		return err
	}

	verboseLog(err, sql, args...)

	return SelectContextString(ctx, dest, sql, args...)
}

var Builder = sq.StatementBuilder.PlaceholderFormat(sq.Dollar)
