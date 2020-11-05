// Package model handles database connections and transactions
package model

import (
	"context"
	_ "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/ntbosscher/gobase/env"
	"github.com/ntbosscher/gobase/randomish"
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
func AttachTxHandler(ignorePaths ...string) func(withTx http.Handler) http.Handler {
	return func(withTx http.Handler) http.Handler {
		return &txRouter{withTx: withTx, ignorePaths: ignorePaths}
	}
}

type transactionContextKeyType = string

var transactionContextKey transactionContextKeyType = "transaction-context-key"

type txRouter struct {
	withTx      http.Handler
	ignorePaths []string
}

func (router *txRouter) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	for _, path := range router.ignorePaths {
		if r.URL.Path == path {
			router.withTx.ServeHTTP(w, r)
			return
		}
	}

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
	writer := &httpWriteWrapper{ResponseWriter: w}

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
	http.ResponseWriter
}

func (h *httpWriteWrapper) Write(data []byte) (int, error) {
	if h.StatusCode == 0 {
		h.WriteHeader(http.StatusOK)
	}

	return h.ResponseWriter.Write(data)
}

func (h *httpWriteWrapper) WriteHeader(statusCode int) {
	h.StatusCode = statusCode
	h.ResponseWriter.WriteHeader(statusCode)
}

func (h *httpWriteWrapper) Header() http.Header {
	return h.ResponseWriter.Header()
}

func verboseError(err error) {
	if !EnableVerboseLogging {
		return
	}

	log.Println("gobase/model: " + err.Error())
}

func ScheduleRecurringQuery(interval time.Duration, query string) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Println(r)
			}
		}()

		tc := time.NewTicker(interval)
		defer tc.Stop()

		// delay initial query somewhat randomly to ensure
		// we don't block startup
		<-time.After(time.Duration(5+randomish.Int(0, 10)) * time.Minute)

		for {
			if err := runQuery(context.Background(), query); err != nil {
				log.Println(err)
			}

			<-tc.C
		}
	}()
}

func runQuery(ctx context.Context, query string) error {
	ctx, cleanup, err := BeginTx(ctx, "scheduled-query")
	if err != nil {
		return err
	}

	defer cleanup()

	_, err = Tx(ctx).ExecContext(ctx, query)
	if err != nil {
		return err
	}

	return Commit(ctx)
}
