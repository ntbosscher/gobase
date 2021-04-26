// Package model handles database connections and transactions
package model

import (
	"context"
	"errors"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx"
	"github.com/jmoiron/sqlx"
	"github.com/ntbosscher/gobase/env"
	"io/ioutil"
	"log"
	"os"
	"time"
)

var db *sqlx.DB

func init() {
	if !env.IsUnitTest {
		var err error

		db, err = createConnection()
		if err != nil {
			log.Fatal(err)
		}
	}
}

var dbType = env.Optional("DB_TYPE", "postgres")

func createConnection() (*sqlx.DB, error) {

	db, err := sqlx.Open(dbType, env.Require("CONNECTION_STRING"))
	if err != nil {
		return nil, errors.New(err.Error() + ": check the environment value for CONNECTION_STRING")
	}

	db.SetConnMaxLifetime(time.Minute)
	db.SetMaxIdleConns(5)
	db.SetMaxOpenConns(10)
	db.MapperFunc(LowerCamelCaseStructNameMapping)

	err = db.Ping()
	if err != nil {
		return nil, errors.New(err.Error() + ": check the environment value for CONNECTION_STRING")
	}

	return db, nil
}

func OnTransactionCommitted(ctx context.Context, callback func()) {
	tx := getInfo(ctx)
	if tx.commitCalled {
		callback()
		return
	}

	tx.callbacks = append(tx.callbacks, callback)
}

// NewConnection creates a new database connection. If you are attempting to use other model functions
// (e.g. model.SelectContext), you don't need this method. A connection will be auto-assigned to your context
//
// You are responsible to clean up this connection when you're done with it
func NewConnection() (*sqlx.DB, error) {
	return createConnection()
}

// EnableVerboseLogging will log all queries, input parameters and errors (even ones returned to the caller).
// This is very useful for debugging, but should be disabled in production.
var EnableVerboseLogging = false

func verboseLog(err error, query string, args ...interface{}) {
	if !EnableVerboseLogging {
		return
	}

	if err != nil {
		log.Println("query failed: " + err.Error())
	}
	log.Println("query input: ", query, args)
}

var _debugLogger = log.New(os.Stdout, "tx-debug: ", log.Ldate|log.Ltime)
var _discardDebugLogger = log.New(ioutil.Discard, "", log.Flags())

func debugLogger() *log.Logger {
	if EnableVerboseLogging {
		return _debugLogger
	}

	return _discardDebugLogger
}

func verboseError(err error) {
	if !EnableVerboseLogging {
		return
	}

	log.Println("gobase/model: " + err.Error())
}
