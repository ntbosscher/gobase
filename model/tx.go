package model

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jmoiron/sqlx"
)

type txInfo struct {
	commitCalled                 bool
	rollbackCalled               bool
	traceId                      string
	tx                           *sqlx.Tx
	commitCallbacks              []func()
	rollbackOrCommitErrCallbacks []func()
}

type BeginTx2Options struct {
	TraceID        string
	IsolationLevel sql.IsolationLevel
	Readonly       bool
}

func BeginTx2(ctx context.Context, opts *BeginTx2Options) (context.Context, func(), error) {
	tx, err := startTx(ctx, &sql.TxOptions{Isolation: opts.IsolationLevel, ReadOnly: opts.Readonly})
	if err != nil {
		return nil, nil, err
	}

	debugLogger().Println("starting", opts.TraceID)

	ctx = context.WithValue(ctx, transactionContextKey, &txInfo{
		commitCalled:                 false,
		rollbackCalled:               false,
		tx:                           tx,
		traceId:                      opts.TraceID,
		commitCallbacks:              []func(){},
		rollbackOrCommitErrCallbacks: []func(){},
	})

	cleanup := func() {

		info := getInfo(ctx)
		if info.commitCalled {
			return // nothing to do
		}

		if info.rollbackCalled {
			return // nothing to do
		}

		_ = Rollback(ctx)
	}

	return ctx, cleanup, nil
}

func BeginTx(ctx context.Context, traceId string) (context.Context, func(), error) {
	return BeginTx2(ctx, &BeginTx2Options{
		TraceID: traceId,
	})
}

func Tx(ctx context.Context) *sqlx.Tx {
	return getInfo(ctx).tx
}

func HasTx(ctx context.Context) bool {
	_, ok := ctx.Value(transactionContextKey).(*txInfo)
	return ok
}

func getInfo(ctx context.Context) *txInfo {
	return ctx.Value(transactionContextKey).(*txInfo)
}

func Rollback(ctx context.Context) error {
	info := getInfo(ctx)
	if info.commitCalled {
		return errCommitAlreadyCalled
	}

	info.commitCalled = true
	info.rollbackCalled = true
	debugLogger().Println("rollback", info.traceId)
	err := info.tx.Rollback()

	for _, callback := range info.rollbackOrCommitErrCallbacks {
		callback()
	}

	return err
}

var errCommitAlreadyCalled = errors.New("transaction already closed")

func Commit(ctx context.Context) error {
	info := getInfo(ctx)
	if info.commitCalled {
		return errCommitAlreadyCalled
	}

	info.commitCalled = true
	debugLogger().Println("commit", info.traceId)
	err := info.tx.Commit()
	if err != nil {
		for _, callback := range info.rollbackOrCommitErrCallbacks {
			callback()
		}

		return err
	}

	for _, callback := range info.commitCallbacks {
		callback()
	}

	return nil
}

func startTx(ctx context.Context, opts *sql.TxOptions) (*sqlx.Tx, error) {
	tx, err := getDb(ctx).BeginTxx(ctx, opts)
	if err != nil {
		return nil, err
	}

	return tx, nil
}

// WithTx runs the callback in a sql transaction. If the callback inTx
// returns an error, the transaction is rolled back
func WithTx(ctx context.Context, inTx func(ctx context.Context, tx *sqlx.Tx) error) error {
	ctx, cancel, err := BeginTx(ctx, "with-tx")
	if err != nil {
		return err
	}

	defer cancel()

	if err := inTx(ctx, Tx(ctx)); err != nil {
		return err
	}

	return Commit(ctx)
}
