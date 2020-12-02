// Package paginate helps with paginated queries and http requests to get paginated queries
package paginate

import (
	"bytes"
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"github.com/Masterminds/squirrel"
	"github.com/gocarina/gocsv"
	"github.com/ntbosscher/gobase/er"
	"github.com/ntbosscher/gobase/model"
	"github.com/ntbosscher/gobase/model/squtil"
	"github.com/ntbosscher/gobase/res"
	"regexp"
	"strings"
	"time"
	"unicode"
)

// MaxPageSize limits the page size that a requester can make
var MaxPageSize = 50

// DefaultPageSize is used when no page size is provided
var DefaultPageSize = MaxPageSize

// Config is a blanket interface for all configuration parameters
type Config interface{}

// ColumnMapping converts an input column name to a database column
// e.g. "First Name" => "first_name", "Full Name" => "concat(first_name, ' ', last_name)"
type ColumnMapping map[string]string

// SearchFields define which fields are checked in a "search" query
func SearchFields(fields ...string) Config {
	return fields
}

type Search string

type OrderBy struct {
	Column    string
	Direction string
}

type Paging struct {
	Page     int
	PageSize int
}

type Filter struct {
	Column   string
	Operator string
	Value    string
}

// ParamsFromRequest parses a request into a Config structure
func ParamsFromRequest(rq *res.Request) Config {
	return []Config{
		Search(rq.Query("search")),
		OrderBy{
			Column:    rq.Query("orderBy"),
			Direction: rq.Query("orderByDirection"),
		},
		Paging{
			Page:     rq.GetQueryInt("page"),
			PageSize: rq.GetQueryInt("pageSize"),
		},
		DownloadFileName(rq.Query("download")), // download triggers a csv file download
		decodeRequestFilter(rq.Query("filter")),
	}
}

var filterRegexp = regexp.MustCompile(`^(.*?)(=|<=|>=|<|>)(.*)$`)

func decodeRequestFilter(value string) []Filter {
	if value == "" {
		return []Filter{}
	}

	parts := strings.Split(value, ",")
	out := []Filter{}

	for _, part := range parts {
		matches := filterRegexp.FindAllStringSubmatch(part, -1)
		if len(matches) == 0 {
			er.Throw("invalid filter part '" + part + "'")
		}

		out = append(out, Filter{
			Column:   matches[0][1],
			Operator: matches[0][2],
			Value:    matches[0][3],
		})
	}

	return out
}

type DownloadFileName string

type queryConfig struct {
	columnMapping    ColumnMapping
	searchFields     []string
	search           string
	orderBy          OrderBy
	page             int
	pageSize         int
	downloadFileName string
	filter           []Filter
}

func (q *queryConfig) decodeConfig(configList []Config) {
	for _, cfg := range configList {
		switch v := cfg.(type) {
		case []string:
			q.searchFields = v
		case ColumnMapping:
			q.columnMapping = v
		case Search:
			q.search = string(v)
		case OrderBy:
			q.orderBy = v
		case Paging:
			q.page = v.Page
			q.pageSize = v.PageSize
		case DownloadFileName:
			q.downloadFileName = string(v)
		case Filter:
			q.filter = append(q.filter, v)
		case []Filter:
			q.filter = append(q.filter, v...)
		case []Config:
			q.decodeConfig(v)
		}
	}
}

func applySearch(query squirrel.SelectBuilder, cfg *queryConfig) squirrel.SelectBuilder {

	if len(cfg.searchFields) == 0 {
		return query
	}

	if cfg.search == "" {
		return query
	}

	search := "%" + cfg.search + "%"
	var or []squirrel.Sqlizer

	for _, field := range cfg.searchFields {
		like := squirrel.ILike{}
		like[field] = search
		or = append(or, like)
	}

	return query.Where(squirrel.Or(or))
}

func applyFilters(query squirrel.SelectBuilder, cfg *queryConfig, listDest interface{}) squirrel.SelectBuilder {

	if len(cfg.filter) == 0 {
		return query
	}

	for _, filter := range cfg.filter {
		dbCol := SanitizeDbColumn(filter.Column, cfg.columnMapping)

		props := map[string]interface{}{}
		props[dbCol] = filter.Value

		var operator interface{}

		switch filter.Operator {
		case "=":
			operator = squirrel.Eq(props)
		case ">=":
			operator = squirrel.GtOrEq(props)
		case "<=":
			operator = squirrel.LtOrEq(props)
		case "<":
			operator = squirrel.Lt(props)
		case ">":
			operator = squirrel.Gt(props)
		default:
			er.Throw("invalid operator '" + filter.Operator + "'")
			return query
		}

		query = query.Where(operator)
	}

	return query
}

// Query applies the configuration options to the query and returns a paginated response
// listDest should contain the type of result to expect (e.g. &[]*Person{})
func Query(ctx context.Context, listDest interface{}, baseQuery squirrel.SelectBuilder, config ...Config) res.Responder {

	query := baseQuery

	cfg := &queryConfig{}
	cfg.decodeConfig(config)

	downloadFileName := cfg.downloadFileName
	isDownload := downloadFileName != ""

	query = applySearch(query, cfg)
	query = applyFilters(query, cfg, listDest)

	totalCount := 0

	// don't need totalCount for downloads
	if !isDownload {
		err := model.Builder.
			Select("count(*)").
			FromSelect(query, "d").
			RunWith(model.Tx(ctx)).
			Scan(&totalCount)

		er.Check(err)
	}

	query = applyOrderBy(query, cfg)

	if !isDownload {
		query = applyPager(query, cfg)
	}

	squtil.MustSelectContext(ctx, listDest, query)

	if isDownload {
		return DownloadResponse(listDest, downloadFileName)
	}

	return Response(listDest, totalCount)
}

func applyPager(query squirrel.SelectBuilder, cfg *queryConfig) squirrel.SelectBuilder {

	pageSize := cfg.pageSize

	if pageSize > MaxPageSize {
		pageSize = MaxPageSize
	}

	if pageSize == 0 {
		pageSize = DefaultPageSize
	}

	return query.Limit(uint64(pageSize)).Offset(uint64(cfg.page * pageSize))
}

func applyOrderBy(query squirrel.SelectBuilder, cfg *queryConfig) squirrel.SelectBuilder {

	orderBy := decodeOrderBy(cfg.orderBy.Column, cfg.columnMapping)
	orderByDirection := SanitizeOrderByDirection(cfg.orderBy.Direction)

	for _, order := range orderBy {
		query = query.OrderBy(order + " " + orderByDirection)
	}

	return query
}

// DownloadResponse converts the list to a csv download response
func DownloadResponse(listDest interface{}, downloadFileName string) res.Responder {

	buf := &bytes.Buffer{}

	if err := gocsv.Marshal(listDest, buf); err != nil {
		return res.Error(err)
	}

	return res.Download(downloadFileName, bytes.NewReader(buf.Bytes()))
}

// Response converts the list and total data count to a paginated http response
func Response(data interface{}, totalCount int) res.Responder {
	return res.Ok(map[string]interface{}{
		"data":       data,
		"totalCount": totalCount,
	})
}

func SanitizeOrderByDirection(value string) string {
	switch value {
	case "asc":
		return value
	case "desc":
		return value
	case "":
		return value
	default:
		er.Throw("invalid order by direction '" + value + "'")
		return ""
	}
}

func camelCaseToSnakeCase(str string) string {
	out := []rune{}
	src := []rune(str)

	for i, c := range src {
		if unicode.IsUpper(c) {
			if i != 0 {
				out = append(out, '_')
			}

			out = append(out, unicode.ToLower(c))
			continue
		}

		out = append(out, c)
	}

	return string(out)
}

var columnNameRegExp = regexp.MustCompile("^[a-z_]+$")

func SanitizeDbColumn(inputCol string, mapping ColumnMapping) string {

	if mapping == nil {
		mapping = ColumnMapping{}
	}

	replace, ok := mapping[inputCol]
	if ok {
		return replace
	}

	replace, ok = mapping[strings.ToLower(inputCol)]
	if ok {
		return replace
	}

	// remove spaces
	inputCol = strings.Replace(inputCol, " ", "", -1)
	name := camelCaseToSnakeCase(inputCol)

	if !columnNameRegExp.MatchString(name) {
		er.Throw("invalid order by column '" + name + "'")
		return ""
	}

	return name
}

func decodeOrderBy(value string, mapping ColumnMapping) []string {
	if value == "" {
		return []string{}
	}

	parts := strings.Split(value, ",")
	var out []string

	for _, part := range parts {
		out = append(out, SanitizeDbColumn(part, mapping))
	}

	return out
}

type Date time.Time

// Convert the internal date as CSV string
func (date Date) MarshalCSV() (string, error) {
	return time.Time(date).Format("2006-01-02"), nil
}

func (date Date) MarshalJSON() ([]byte, error) {
	return time.Time(date).MarshalJSON()
}

func (date *Date) UnmarshalJSON(data []byte) error {
	tm := time.Time{}

	if err := json.Unmarshal(data, &tm); err != nil {
		return err
	}

	*date = Date(tm)
	return nil
}

func (date *Date) Scan(value interface{}) error {
	v, ok := value.(time.Time)
	if !ok {
		return errors.New("expecting time.Time")
	}

	*date = Date(v)
	return nil
}

func (date Date) Value() (driver.Value, error) {
	return time.Time(date), nil
}
