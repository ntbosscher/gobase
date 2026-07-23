package model

import (
	"context"
	"database/sql"
	"net/http"

	"github.com/gorilla/websocket"
)

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

type AttachTxHandlerOpts struct {
	IgnorePaths    []string
	IsolationLevel sql.IsolationLevel
	Readonly       bool
}

func AttachTxHandler2(opts *AttachTxHandlerOpts) func(withTx http.Handler) http.Handler {
	return func(withTx http.Handler) http.Handler {
		return &txRouter{
			withTx:      withTx,
			ignorePaths: opts.IgnorePaths,
			transactionOptions: &BeginTx2Options{
				IsolationLevel: opts.IsolationLevel,
				Readonly:       opts.Readonly,
			},
		}
	}
}

type transactionContextKeyType = string

var transactionContextKey transactionContextKeyType = "transaction-context-key"

type txRouter struct {
	withTx             http.Handler
	ignorePaths        []string
	transactionOptions *BeginTx2Options
}

func (router *txRouter) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	for _, path := range router.ignorePaths {
		if r.URL.Path == path {
			router.withTx.ServeHTTP(w, r)
			return
		}
	}

	// don't open transactions for websocket connections
	if websocket.IsWebSocketUpgrade(r) {
		router.withTx.ServeHTTP(w, r)
		return
	}

	ctx := r.Context()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	opts := router.transactionOptions
	if opts == nil {
		opts = &BeginTx2Options{}
	}

	ctx, cleanup, err := BeginTx2(ctx, &BeginTx2Options{
		TraceID:        "tx-router:" + r.Method + r.URL.String(),
		IsolationLevel: opts.IsolationLevel,
		Readonly:       opts.Readonly,
	})

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

		if writer.StatusCode == 0 {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error": "Unable to complete transaction"}`))
		}
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

// Unwrap exposes the underlying ResponseWriter so that http.ResponseController
// (and therefore SSE flushing, connection hijacking, etc.) can reach the real
// Flusher/Hijacker beneath the tx wrapper.
func (h *httpWriteWrapper) Unwrap() http.ResponseWriter {
	return h.ResponseWriter
}
