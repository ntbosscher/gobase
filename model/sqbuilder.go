package model

import (
	sq "github.com/Masterminds/squirrel"
)

// Builder is a pre-configured squirrel sql-builder instance.
// Build queries (e.g. Builder.Select("col").From("table")...) and pass the builder to SelectContext() to get the
// results.
var Builder = sq.StatementBuilder.PlaceholderFormat(sq.Dollar)
