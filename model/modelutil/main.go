package modelutil

import (
	"context"
	"encoding/base64"
	"fmt"
	"github.com/jmoiron/sqlx"
	"github.com/ntbosscher/gobase/model"
	"time"
)

type Table struct {
	Headers []string
	Rows    [][]string
}

func SelectTable(ctx context.Context, query string, args ...interface{}) (*Table, error) {
	table := &Table{}

	err := model.WithTx(ctx, func(ctx context.Context, tx *sqlx.Tx) error {
		rows, err := model.Tx(ctx).QueryContext(ctx, query, args...)
		if err != nil {
			return err
		}

		defer rows.Close()

		cols, err := rows.Columns()
		if err != nil {
			return err
		}

		table.Headers = cols

		var columnValues []*stringScanner
		var interfaceValues []interface{}
		for range cols {
			scanner := &stringScanner{}
			columnValues = append(columnValues, scanner)
			interfaceValues = append(interfaceValues, scanner)
		}

		for rows.Next() {
			err := rows.Scan(interfaceValues...)
			if err != nil {
				return err
			}

			var row []string
			for _, col := range columnValues {
				row = append(row, col.Value)
			}

			table.Rows = append(table.Rows, row)
		}

		return nil
	})

	return table, err
}

type stringScanner struct {
	Value string
}

func (s *stringScanner) Scan(src interface{}) error {

	if src == nil {
		s.Value = "null"
		return nil
	}

	switch v := src.(type) {
	case int64:
		s.Value = fmt.Sprint(v)
	case float64:
		s.Value = fmt.Sprint(v)
	case bool:
		s.Value = fmt.Sprint(v)
	case []byte:
		s.Value = base64.StdEncoding.EncodeToString(v)
	case string:
		s.Value = v
	case time.Time:
		s.Value = v.Format("2006-Jan-02 15:04:05")
	}

	return nil
}
