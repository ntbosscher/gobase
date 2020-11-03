// Package squtil is a place for utils to help with github.com/Masterminds/squirrel
package squtil

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/Masterminds/squirrel"
	"github.com/lann/builder"
	"strings"
)

func init() {
	builder.Register(UnionBuilder{}, unionData{})
}

type unionSelect struct {
	op       string // e.g. "UNION"
	selector squirrel.SelectBuilder
}

type unionData struct {
	Selects []*unionSelect
	Limit   string
	OrderBy []string
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

	value, _ := builder.Get(data.Selects[0].selector, "PlaceholderFormat")
	placeholderFmt := value.(squirrel.PlaceholderFormat)

	sqlBuf := &bytes.Buffer{}
	var selArgs []interface{}
	var selSql string

	for index, selector := range data.Selects {

		// use a no-change formatter to prevent issues with numbering args
		sel := selector.selector.PlaceholderFormat(squirrel.Question)

		selSql, selArgs, err = sel.ToSql()
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

	sql, err = placeholderFmt.ReplacePlaceholders(sqlBuf.String())
	return
}

func (u UnionBuilder) Union(selector squirrel.SelectBuilder) UnionBuilder {
	return builder.Append(u, "Selects", &unionSelect{op: "UNION", selector: selector}).(UnionBuilder)
}

func (u UnionBuilder) setFirstSelect(selector squirrel.SelectBuilder) UnionBuilder {
	return builder.Append(u, "Selects", &unionSelect{op: "", selector: selector}).(UnionBuilder)
}

func (u UnionBuilder) Limit(n uint) UnionBuilder {
	return u.setProp("Limit", fmt.Sprintf("%d", n))
}

func (u UnionBuilder) OrderBy(orderBys ...string) UnionBuilder {
	return u.setProp("OrderBy", orderBys)
}

func Union(a squirrel.SelectBuilder, b squirrel.SelectBuilder) UnionBuilder {
	ub := UnionBuilder{}
	ub = ub.setFirstSelect(a)

	return ub.Union(b)
}
