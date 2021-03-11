// Package squtil is a place for utils to help with github.com/Masterminds/squirrel
package squtil

import (
	"bytes"
	"errors"
	"fmt"
	sq "github.com/Masterminds/squirrel"
	"github.com/lann/builder"
	"strings"
)

func init() {
	builder.Register(UnionBuilder{}, unionData{})
}

type unionSelect struct {
	op       string // e.g. "UNION"
	selector sq.SelectBuilder
}

type unionData struct {
	Selects           []*unionSelect
	Limit             string
	OrderBy           []string
	PlaceholderFormat sq.PlaceholderFormat
}

// UnionBuilder is a (rather hack) implementation of Unions for squirrel query builder. They
// currently don't offer this feature. When they do, this code should be trashed
type UnionBuilder builder.Builder

func (u UnionBuilder) setProp(key string, value interface{}) UnionBuilder {
	return builder.Set(u, key, value).(UnionBuilder)
}

func (u UnionBuilder) ToSql() (sql string, args []interface{}, err error) {
	data := builder.GetStruct(u).(unionData)

	if len(data.Selects) == 0 {
		err = errors.New("require a minimum of 1 select clause in UnionBuilder")
		return
	}

	sqlBuf := &bytes.Buffer{}
	var selArgs []interface{}
	var selSql string

	for index, selector := range data.Selects {

		selSql, selArgs, err = selector.selector.ToSql()
		if err != nil {
			return
		}

		if index == 0 {
			sqlBuf.WriteString(selSql) // no operator for first select-clause
		} else {
			sqlBuf.WriteString(" " + selector.op + " ( " + selSql + " ) ")
		}

		args = append(args, selArgs...)
	}

	if len(data.OrderBy) > 0 {
		sqlBuf.WriteString(" ORDER BY ")
		sqlBuf.WriteString(strings.Join(data.OrderBy, ","))
	}

	if data.Limit != "" {
		sqlBuf.WriteString(" LIMIT ")
		sqlBuf.WriteString(data.Limit)
	}

	sql = sqlBuf.String()
	return
}

func (u UnionBuilder) Union(selector sq.SelectBuilder) UnionBuilder {
	// use ? in children to prevent numbering issues
	selector = selector.PlaceholderFormat(sq.Question)

	return builder.Append(u, "Selects", &unionSelect{op: "UNION", selector: selector}).(UnionBuilder)
}

func (u UnionBuilder) setFirstSelect(selector sq.SelectBuilder) UnionBuilder {

	// copy the PlaceholderFormat value from children since we don't know what it should be
	value, _ := builder.Get(selector, "PlaceholderFormat")
	bld := u.setProp("PlaceholderFormat", value)

	// use ? in children to prevent numbering issues
	selector = selector.PlaceholderFormat(sq.Question)

	return builder.Append(bld, "Selects", &unionSelect{op: "", selector: selector}).(UnionBuilder)
}

func (u UnionBuilder) Limit(n uint) UnionBuilder {
	return u.setProp("Limit", fmt.Sprintf("%d", n))
}

func (u UnionBuilder) OrderBy(orderBys ...string) UnionBuilder {
	return u.setProp("OrderBy", orderBys)
}

func (u UnionBuilder) PlaceholderFormat(fmt sq.PlaceholderFormat) UnionBuilder {
	return u.setProp("PlaceholderFormat", fmt)
}

func Union(a sq.SelectBuilder, b sq.SelectBuilder) UnionBuilder {
	ub := UnionBuilder{}
	ub = ub.setFirstSelect(a)
	return ub.Union(b)
}

func NewUnion() UnionBuilder {
	return UnionBuilder{}
}

func SelectFromUnion(selectBuilder sq.SelectBuilder, union UnionBuilder, alias string) sq.SelectBuilder {
	// use ? in child to prevent numbering issues
	union = union.PlaceholderFormat(sq.Question)

	return builder.Set(selectBuilder, "From", sq.Alias(union, alias)).(sq.SelectBuilder)
}
