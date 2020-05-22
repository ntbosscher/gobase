package model

import (
	"context"
	"fmt"
	sq "github.com/Masterminds/squirrel"
	"log"
)

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

	err = Tx(ctx).SelectContext(ctx, dest, sql, args...)
	if err != nil {
		log.Println(sql)
		log.Println(err.Error())
		return err
	}

	return nil
}

var Builder = sq.StatementBuilder.PlaceholderFormat(sq.Dollar)
