// Package pqshared has a shared connection pool for Postgres-based operations
package pqshared

import (
	"context"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/ntbosscher/gobase/env"
	"log"
)

var Pool *pgxpool.Pool

func init() {
	var err error

	Pool, err = pgxpool.Connect(context.Background(), env.Require("CONNECTION_STRING"))
	if err != nil {
		log.Fatal("failed to connect to postgres using environment variable CONNECTION_STRING", err)
	}
}
