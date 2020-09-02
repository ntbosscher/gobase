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
	writer := &httpWriteWrapper{next: w}

	router.withTx.ServeHTTP(writer, r)

	if getInfo(ctx).commitCalled {
		return
	}

	if writer.StatusCode >= 400 {
		if err = Rollback(ctx); err != nil {
			verboseError(err)
		}

		return
	}

	if err = Commit(ctx); err != nil {
		verboseError(err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "Unable to complete transaction"}`))
	}

}

type httpWriteWrapper struct {
	StatusCode int
	next       http.ResponseWriter
}

func (h *httpWriteWrapper) Write(data []byte) (int, error) {
	if h.StatusCode == 0 {
		h.StatusCode = http.StatusOK
		h.next.WriteHeader(http.StatusOK)
	}

	return h.next.Write(data)
}

func (h *httpWriteWrapper) WriteHeader(statusCode int) {
	h.StatusCode = statusCode
	h.next.WriteHeader(statusCode)
}

func (h *httpWriteWrapper) Header() http.Header {
	return h.next.Header()
}

func verboseError(err error) {
	if !EnableVerboseLogging {
		return
	}

	log.Println("gobase/model: " + err.Error())
}
