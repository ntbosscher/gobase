// Package model handles database connections and transactions
package model

import (
	"context"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/ntbosscher/gobase/env"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"time"
)

var db *sqlx.DB
var VerboseLogging = false

func init() {
	db = createConnection()
}

func createConnection() *sqlx.DB {
	db, err := sqlx.Open("postgres", env.Require("CONNECTION_STRING"))
	if err != nil {
		log.Fatal(err.Error() + ": check the environment value for CONNECTION_STRING")
	}

	db.SetConnMaxLifetime(time.Minute)
	db.SetMaxIdleConns(5)
	db.SetMaxOpenConns(10)

	err = db.Ping()
	if err != nil {
		log.Fatal(err.Error() + ": check the environment value for CONNECTION_STRING")
	}

	return db
}

var _debugLogger *log.Logger

func debugLogger() *log.Logger {
	if _debugLogger != nil {
		return _debugLogger
	}

	if VerboseLogging {
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
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "Unable to start transaction"}`))
		return
	}

	defer cleanup()
	r = r.WithContext(ctx)

	router.withTx.ServeHTTP(w, r)

	if !getInfo(ctx).commitCalled {
		if err = Commit(ctx); err != nil {
			log.Println(err)
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error": "Unable to complete transaction"}`))
		}
	}
}
