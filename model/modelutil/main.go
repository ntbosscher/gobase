package modelutil

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/csv"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"regexp"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/Masterminds/squirrel"
	"github.com/lib/pq"
	"github.com/ntbosscher/gobase/encoding/tsv"
	"github.com/ntbosscher/gobase/env"
	"github.com/ntbosscher/gobase/er"
	"github.com/ntbosscher/gobase/jv"
	"github.com/ntbosscher/gobase/model"
	"github.com/ntbosscher/gobase/model/modelperf"
	"github.com/ntbosscher/gobase/model/squtil"
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

var regexpReplace = regexp.MustCompile("\\$[0-9]+")

func PrintDebugSQL(qr squirrel.SelectBuilder) {
	sql, args, _ := qr.ToSql()

	strs := map[string]string{}

	for i, arg := range args {
		value := ""

		switch v := arg.(type) {
		case string:
			value = fmt.Sprintf("'%s'", v)
		default:
			value = fmt.Sprint(v)
		}

		strs[fmt.Sprintf("$%d", i+1)] = value
	}

	sql = regexpReplace.ReplaceAllStringFunc(sql, func(s string) string {
		value, ok := strs[s]
		if !ok {
			return s
		}

		return value
	})

	fmt.Println(sql)
}

var runPerf = env.OptionalBool("DEBUG_PERF", false)

func PerfMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Header.Get("X-Debug-Perf") == "" && !runPerf {
			next.ServeHTTP(w, req)
			return
		}

		ctx, cancel, perf := modelperf.New(req.Context(), &modelperf.PerfInput{})
		defer cancel()

		req = req.WithContext(ctx)
		next.ServeHTTP(w, req)

		sum := perf.GetSummaries()

		urlCopy, _ := url.Parse(req.URL.String())
		urlCopy.RawQuery = ""

		select {
		case perfC <- &PerfInfo{
			Request: req.Method + " " + urlCopy.String(),
			Info:    sum,
		}:
		default:
		}
	})
}

type PerfInfo struct {
	Request string
	Info    []*modelperf.Summary
}

var perfC = make(chan *PerfInfo, 100)

func init() {
	go func() {
		defer er.HandleErrors(func(input *er.HandlerInput) {
			log.Println(input)
		})

		merged := map[string][]*PerfInfo{}
		tc := time.NewTicker(5 * time.Second)

		defer tc.Stop()

		changed := false

		for {
			select {
			case p := <-perfC:
				merged[p.Request] = append(merged[p.Request], p)
				changed = true
			case <-tc.C:
				if !changed {
					continue
				}

				printPerfMap(merged)
			}
		}
	}()
}

func ellipsis(qr string, max int) string {
	if len(qr) <= max {
		return qr
	}

	return qr[:max] + "..."

}

func printPerfMap(merged map[string][]*PerfInfo) {
	buf := &bytes.Buffer{}

	queries := map[string]string{}

	list := jv.GetMapValues(merged)

	list = jv.Reverse(jv.SortFx(list, func(a []*PerfInfo) float64 {
		times := jv.Mapper(a, func(a *PerfInfo) time.Duration {
			sum := time.Duration(0)

			for _, item := range a.Info {
				sum += item.TotalDuration
			}

			return sum
		})

		worstTime := time.Duration(0)
		for _, tm := range times {
			if tm > worstTime {
				worstTime = tm
			}
		}

		return float64(worstTime)
	}))

	for _, infos := range list {
		buf.WriteString("------------------------------------------------------\n")
		buf.WriteString(infos[0].Request + "\n")

		have := map[string]bool{}

		sum := []*modelperf.Summary{}
		for _, item := range infos {
			sum = append(sum, item.Info...)
		}

		sum = jv.Reverse(jv.SortFx(sum, func(a *modelperf.Summary) float64 {
			return float64(a.TotalDuration)
		}))

		for _, qr := range sum {
			if have[qr.Query] {
				continue
			}

			fmt.Fprintln(buf, ellipsis(qr.Query, 200))
			hash := md5.Sum([]byte(qr.Query))
			hashStr := hex.EncodeToString(hash[:])
			fmt.Fprintln(buf, hashStr)
			queries[hashStr] = qr.Query

			for _, item := range sum {
				if item.Query == qr.Query {
					fmt.Fprintln(buf, "avg", qr.AverageDuration.String(), "count", qr.CallCount, "total", qr.TotalDuration.String())
				}
			}

			have[qr.Query] = true
		}
	}

	for hash, item := range queries {
		fmt.Fprintln(buf, "------------------------------------------------------")
		fmt.Fprintln(buf, hash)
		fmt.Fprintln(buf, item)
	}

	os.WriteFile("/tmp/perf.txt", buf.Bytes(), 0644)
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
	er.Check(err)

	wr := tabwriter.NewWriter(os.Stdout, 4, 1, 1, ' ', 0)
	cols := tsv.NewEncoder(wr)
	cols.WriteRow(tbl.Headers)
	for _, row := range tbl.Rows {
		cols.WriteRow(row)
	}

	wr.Flush()
}

// PqArray fixes the nil case with pq.Array where pq.Array interprets it as a sql null.
// PqArray on the other hand treats a nil array as an empty array. (i.e. [])
func PqArray[T any](input []T) interface{} {
	if len(input) == 0 {
		input = []T{}
	}

	return pq.Array(input)
}
