package modelutil

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/csv"
	"fmt"
	"github.com/Masterminds/squirrel"
	"github.com/ntbosscher/gobase/encoding/tsv"
	"github.com/ntbosscher/gobase/model"
	"github.com/ntbosscher/gobase/model/squtil"
	"os"
	"reflect"
	"strings"
	"text/tabwriter"
	"time"
)

type Table struct {
	Headers []string
	Rows    [][]string
}

func SelectTable(ctx context.Context, query string, args ...interface{}) (*Table, error) {
	table := &Table{}

	if !model.HasTx(ctx) {
		tctx, cancel, err := model.BeginTx(ctx, "select-table")
		if err != nil {
			return nil, err
		}

		defer cancel()
		ctx = tctx
	}

	rows, err := model.Tx(ctx).QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	table.Headers = cols

	types, err := rows.ColumnTypes()
	if err != nil {
		return nil, err
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
			return nil, err
		}

		var row []string
		for _, col := range columnValues {
			row = append(row, col.Value)
		}

		table.Rows = append(table.Rows, row)
	}

	return table, err
}

func (t *Table) ToCSV() []byte {
	buf := &bytes.Buffer{}
	c := csv.NewWriter(buf)

	_ = c.Write(t.Headers)
	_ = c.WriteAll(t.Rows)

	c.Flush()
	return buf.Bytes()
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

// BuildUpdateWL works the same as BuildUpdate except that the list of fields provided is used as a white-list
// instead of the black list method used by BuildUpdate.
func BuildUpdateWL(ctx context.Context, table string, value interface{}, id int, allowedFields ...string) squirrel.UpdateBuilder {
	update := squirrel.Eq{}

	tx := model.Tx(ctx)
	withDbNames := tx.Mapper.FieldMap(reflect.ValueOf(value))

	for k, v := range withDbNames {
		if strings.Contains(k, ".") { // ignore sub properties
			continue
		}

		if !containsFieldName(allowedFields, k) {
			continue
		}

		update[k] = v.Interface()
	}

	return model.Builder.Update(table).
		SetMap(update).
		Where(squirrel.Eq{"id": id})
}

func BuildUpdate(ctx context.Context, table string, value interface{}, id int, ignoreFields ...string) squirrel.UpdateBuilder {
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

	return model.Builder.Update(table).
		SetMap(update).
		Where(squirrel.Eq{"id": id})

}

// UpdateStruct updates the columns based on the struct provided.
//
// recommended to use UpdateStructWL instead since structs can change over time and caused unexpected
// columns to be updated if not specified in the ignoreFields.
func UpdateStruct(ctx context.Context, table string, value interface{}, id int, ignoreFields ...string) {
	qr := BuildUpdate(ctx, table, value, id, ignoreFields...)
	squtil.MustExecContext(ctx, qr)
}

// UpdateStructWL updates the columns specified by allowedFields
func UpdateStructWL(ctx context.Context, table string, value interface{}, id int, allowedFields ...string) {
	qr := BuildUpdateWL(ctx, table, value, id, allowedFields...)
	squtil.MustExecContext(ctx, qr)
}

func PrintTable(ctx context.Context, query string, args ...interface{}) {
	tbl, err := SelectTable(ctx, query, args...)
	if err != nil {
		return
	}

	wr := tabwriter.NewWriter(os.Stdout, 4, 1, 1, ' ', 0)
	cols := tsv.NewEncoder(wr)
	cols.WriteRow(tbl.Headers)
	for _, row := range tbl.Rows {
		cols.WriteRow(row)
	}

	wr.Flush()
}
