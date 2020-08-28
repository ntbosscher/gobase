// Package model handles database connections and transactions
package model

import (
	"context"
	_ "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/ntbosscher/gobase/env"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"time"
	"unicode"
)

var db *sqlx.DB

func init() {
	db = createConnection()
}

func createConnection() *sqlx.DB {

	db, err := sqlx.Open(env.Optional("DB_TYPE", "postgres"), env.Require("CONNECTION_STRING"))
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

// Updates the default struct to column name mapper (you can still bypass this using the `db:"col_name"` tag)
// sample:
//   // struct:
//   type Company struct { ID int, ContactPerson int }
//
//   // query:
//   select id, contactPerson from company where id = 1;
//
//   // mapper:
//   mapper("ID") // id
//   mapper("ContactPerson") // contactPerson
//
func SetStructNameMapping(mapper func(structCol string) (colName string)) {
	db.MapperFunc(mapper)
}

func LowerCamelCaseStructNameMapping(structCol string) string {
	if len(structCol) == 0 {
		return structCol
	}

	if structCol == "ID" {
		return "id"
	}

	runes := []rune(structCol)
	runes[0] = unicode.ToLower(runes[0])

	length := len(runes)

	if length > 2 {
		if string(runes[length-2:]) == "ID" {
			runes[length-2] = 'I'
			runes[length-1] = 'd'
		}
	}

	return string(runes)
}

var _debugLogger *log.Logger

func debugLogger() *log.Logger {
	if _debugLogger != nil {
		return _debugLogger
	}

	if EnableVerboseLogging {
		_debugLogger = log.New(os.Stdout, "tx-debug: ", log.Ldate|log.Ltime)
	} else {
		_debugLogger = log.New(ioutil.Discard, "", log.Flags())
	}

	return _debugLogger
}

// AttachTxHandler starts a database transaction so that
// subsequent calls to Tx() will use the same transaction.
//
// By default transactions will be cancelled unless Commit()
// is called.
func AttachTxHandler(withTx http.Handler) http.Handler {
	return &txRouter{withTx: withTx}
}

type transactionContextKeyType = string

var transactionContextKey transactionContextKeyType = "transaction-context-key"

type txRouter struct {
	withTx http.Handler
}

func (router *txRouter) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	ctx := r.Context()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	ctx, cleanup, err := BeginTx(ctx, "tx-router:"+r.Method+r.URL.String())
	if err != nil {
		verboseError(err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "Unable to start transaction"}`))
		return
	}

	defer cleanup()
	r = r.WithContext(ctx)

	router.withTx.ServeHTTP(w, r)

	if !getInfo(ctx).commitCalled {
		if err = Commit(ctx); err != nil {
			verboseError(err)
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error": "Unable to complete transaction"}`))
		}
	}
}

func verboseError(err error) {
	if !EnableVerboseLogging {
		return
	}

	log.Println("gobase/model: " + err.Error())
}
