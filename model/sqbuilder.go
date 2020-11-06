package model

import (
	"context"
	sq "github.com/Masterminds/squirrel"
	"github.com/ntbosscher/gobase/model/squtil"
)

// SelectContext runs a query created with model.Builder.Select and scans the resulting
// rows into dest.
// Dest must be a pointer to a array-type (e.g. *[]*Person)
func SelectContext(ctx context.Context, dest interface{}, qr sq.Sqlizer) error {
	sqlStr, args, err := qr.ToSql()
	if err != nil {
		verboseLog(err, sqlStr, args...)
		return err
	}

	verboseLog(err, sqlStr, args...)
	return SelectContextString(ctx, dest, sqlStr, args...)
}

// Builder is a pre-configured squirrel sql-builder instance.
// Build queries (e.g. Builder.Select("col").From("table")...) and pass the builder to SelectContext() to get the
// results.
var Builder = sq.StatementBuilder.PlaceholderFormat(sq.Dollar)

// UnionBuilder is a (rather hack) implementation of Unions for squirrel query builder. They
// currently don't offer this feature. When they do, this code should be trashed
func UnionBuilder(a sq.SelectBuilder, b sq.SelectBuilder) squtil.UnionBuilder {
	return squtil.Union(a, b)
}
