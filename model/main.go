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
	"sync"
	"time"
)

var defaultDb *sqlx.DB
var defaultDbType string
var mappingFunc = LowerCamelCaseStructNameMapping
var muAll = sync.RWMutex{}
var otherDbs = map[string]*sqlx.DB{}

func init() {
	defaultDbType = env.Optional("DB_TYPE", "postgres")
	skipInitConnect := env.OptionalBool("DB_NO_CONNECT_ON_INIT", false)

	if env.IsUnitTest {
		skipInitConnect = true
	}

	if skipInitConnect {
		return
	}

	var err error

	defaultDb, err = createConnection(defaultDbType, env.Require("CONNECTION_STRING"))
	if err != nil {
		err = errors.New(err.Error() + ": check the environment value for CONNECTION_STRING")
		log.Fatal(err)
	}

	otherDbs[DefaultConnectionKey] = defaultDb
}

// AddConnection adds additional database connections that can be used by this package.
// Toggle between connections with UseConnection()
func AddConnection(key string, dbType string, connectionString string) error {
	muAll.Lock()
	defer muAll.Unlock()

	if otherDbs[key] != nil {
		return errors.New("key '" + key + "' is already assigned to another database connection")
	}

	db, err := createConnection(dbType, connectionString)
	if err != nil {
		return err
	}

	if defaultDb == nil || key == DefaultConnectionKey {
		defaultDb = db
	}

	otherDbs[key] = db
	return nil
}

func createConnection(dbType string, connectionString string) (*sqlx.DB, error) {
	// caller must lock muAll before calling

	db, err := sqlx.Open(dbType, connectionString)
	if err != nil {
		return nil, err
	}

	db.SetConnMaxLifetime(time.Minute)
	db.SetMaxIdleConns(5)
	db.SetMaxOpenConns(10)
	db.MapperFunc(mappingFunc)

	err = db.Ping()
	if err != nil {
		return nil, err
	}

	return db, nil
}

func getDb(ctx context.Context) *sqlx.DB {
	key, ok := ctx.Value(contextKey).(string)
	if !ok {
		return defaultDb
	}

	muAll.RLock()
	defer muAll.RUnlock()

	db := otherDbs[key]
	if db == nil {
		log.Println("gobase/model: unable to find database for key '" + key + "', falling back to defaultDb")
		return defaultDb
	}

	return db
}

type dbInstanceContextKey string

const contextKey dbInstanceContextKey = "db-instance"

// DefaultConnectionKey is the key that will select the default database connection
const DefaultConnectionKey = "default"

// UseConnection toggles between database connections used by this package.
// The returned context will cause any method in this package to use the connection selected.
// key corresponds to the connection created by AddConnection.
func UseConnection(ctx context.Context, key string) context.Context {
	return context.WithValue(ctx, contextKey, key)
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
	return createConnection(defaultDbType, env.Require("CONNECTION_STRING"))
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
