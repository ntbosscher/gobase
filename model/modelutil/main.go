package modelutil

import (
	"context"
	"encoding/base64"
	"fmt"
	"github.com/Masterminds/squirrel"
	"github.com/jmoiron/sqlx"
	"github.com/ntbosscher/gobase/model"
	"github.com/ntbosscher/gobase/model/squtil"
	"reflect"
	"strings"
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

		types, err := rows.ColumnTypes()
		if err != nil {
			return err
		}

		var columnValues []*stringScanner
		var interfaceValues []interface{}
		for i := range cols {
			scanner := &stringScanner{
				dbType: types[i].DatabaseTypeName(),
			}
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
	Value  string
	dbType string
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
		if s.dbType == "NUMERIC" {
			s.Value = string(v)
			break
		}

		s.Value = base64.StdEncoding.EncodeToString(v)
	case string:
		s.Value = v
	case time.Time:
		s.Value = v.Format("2006-Jan-02 15:04:05")
	}

	return nil
}

func containsFieldName(list []string, test string) bool {
	for _, value := range list {
		if value == test || strings.HasPrefix(test, value+".") {
			return true
		}
	}

	return false
}

func InsertStruct(ctx context.Context, table string, value interface{}, ignoreFields ...string) int {

	insert := squirrel.Eq{}

	tx := model.Tx(ctx)
	withDbNames := tx.Mapper.FieldMap(reflect.ValueOf(value))

	ignoreFields = append(ignoreFields, "id")

	for k, v := range withDbNames {
		if strings.Contains(k, ".") { // ignore sub properties
			continue
		}

		if containsFieldName(ignoreFields, k) {
			continue
		}

		insert[k] = v.Interface()
	}

	qr := model.Builder.Insert(table).SetMap(insert).Suffix("returning id")
	return int(squtil.MustInsert(ctx, qr))
}

func UpdateStruct(ctx context.Context, table string, value interface{}, id int, ignoreFields ...string) {

	update := squirrel.Eq{}

	tx := model.Tx(ctx)
	withDbNames := tx.Mapper.FieldMap(reflect.ValueOf(value))

	ignoreFields = append(ignoreFields, "id")

	for k, v := range withDbNames {
		if strings.Contains(k, ".") { // ignore sub properties
			continue
		}

		if containsFieldName(ignoreFields, k) {
			continue
		}

		update[k] = v.Interface()
	}

	qr := model.Builder.Update(table).
		SetMap(update).
		Where(squirrel.Eq{"id": id})

	squtil.MustExecContext(ctx, qr)
}
