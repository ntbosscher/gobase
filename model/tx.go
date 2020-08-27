package model

import (
	"context"
	"errors"
	"github.com/jmoiron/sqlx"
	"log"
)

type txInfo struct {
	commitCalled bool
	traceId      string
	tx           *sqlx.Tx
}

func BeginTx(ctx context.Context, traceId string) (context.Context, func(), error) {
	tx, err := startTx(ctx)
	if err != nil {
		return nil, nil, err
	}

	debugLogger().Println("starting", traceId)

	ctx = context.WithValue(ctx, transactionContextKey, &txInfo{
		commitCalled: false,
		tx:           tx,
		traceId:      traceId,
	})

	cleanup := func() {

		info := getInfo(ctx)
		if info.commitCalled {
			return // nothing to do
		}

		debugLogger().Println("rollback", traceId)

		if err := tx.Rollback(); err != nil {
			log.Println(err)
		}
	}

	return ctx, cleanup, nil
}

func Tx(ctx context.Context) *sqlx.Tx {
	return getInfo(ctx).tx
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
	debugLogger().Println("rollback", info.traceId)
	return info.tx.Rollback()
}

var errCommitAlreadyCalled = errors.New("transaction already closed")

func Commit(ctx context.Context) error {
	info := getInfo(ctx)
	if info.commitCalled {
		return errCommitAlreadyCalled
	}

	info.commitCalled = true
	debugLogger().Println("commit", info.traceId)
	return info.tx.Commit()
}

func startTx(ctx context.Context) (*sqlx.Tx, error) {
	tx, err := db.BeginTxx(ctx, nil)
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
