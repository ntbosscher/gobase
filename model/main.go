// Package model handles database connections and transactions
package model

import (
	_ "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/ntbosscher/gobase/env"
	"io/ioutil"
	"log"
	"os"
	"time"
)

var db *sqlx.DB

func init() {
	if !env.IsUnitTest {
		db = createConnection()
	}
}

var dbType = env.Optional("DB_TYPE", "postgres")

func createConnection() *sqlx.DB {

	db, err := sqlx.Open(dbType, env.Require("CONNECTION_STRING"))
	if err != nil {
		log.Fatal(err.Error() + ": check the environment value for CONNECTION_STRING")
	}

	db.SetConnMaxLifetime(time.Minute)
	db.SetMaxIdleConns(5)
	db.SetMaxOpenConns(10)
	db.MapperFunc(LowerCamelCaseStructNameMapping)

	err = db.Ping()
	if err != nil {
		log.Fatal(err.Error() + ": check the environment value for CONNECTION_STRING")
	}

	return db
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
