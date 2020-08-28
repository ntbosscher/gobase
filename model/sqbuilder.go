package model

import (
	"context"
	"fmt"
	sq "github.com/Masterminds/squirrel"
	"log"
)

func ExecContext(ctx context.Context, query string, args ...interface{}) error {
	_, err := Tx(ctx).ExecContext(ctx, query, args...)
	return err
}

func Insert(ctx context.Context, query string, args ...interface{}) (id int64, err error) {
	result, err := Tx(ctx).ExecContext(ctx, query, args...)
	if err != nil {
		return
	}

	id, err = result.LastInsertId()
	return
}

func GetContext(ctx context.Context, dest interface{}, query string, args ...interface{}) error {
	return Tx(ctx).GetContext(ctx, dest, query, args...)
}

func SelectContextString(ctx context.Context, dest interface{}, sql string, args ...interface{}) error {
	err := Tx(ctx).SelectContext(ctx, dest, sql, args...)
	if err != nil {
		log.Println(sql)
		log.Println(err.Error())
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

	if VerboseLogging {
		fmt.Println("----")
		fmt.Println(sql)
		fmt.Print("args: ")
		fmt.Println(args...)
		fmt.Println("----")
	}

	return SelectContextString(ctx, dest, sql, args...)
}

var Builder = sq.StatementBuilder.PlaceholderFormat(sq.Dollar)
