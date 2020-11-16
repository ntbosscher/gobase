package model

import (
	"context"
	"github.com/jmoiron/sqlx"
	"github.com/ntbosscher/gobase/randomish"
	"log"
	"time"
)

// ScheduleFunc calls callbackWithTx every interval within a new db transaction.
// While callbackWithTx returns moreToProcess=true, the process will call callbackWithTx again without the interval delay.
// If moreToProcess=false, the process waits for the interval to elapse before calling callbackWithTx again.
//
// Panicking from within callbackWithTx is treated as moreToProcess=false, err!=nil and will not break the process loop
func ScheduleFunc(interval time.Duration, callbackWithTx func(ctx context.Context) (moreToProcess bool, err error)) {
	go func() {
		defer recoverWithLog()

		// delay initial query somewhat randomly to ensure
		// we don't block startup and queries with the same interval don't run at exactly the same time
		<-time.After(interval / time.Duration(randomish.Int(1, 5)))

		tc := time.NewTicker(interval)
		defer tc.Stop()

		for {
			runQueryFunc(context.Background(), callbackWithTx)
			<-tc.C
		}
	}()
}

func runQueryFunc(ctx context.Context, callbackWithTx func(ctx context.Context) (moreToProcess bool, err error)) {
	defer recoverWithLog()

	moreToProcess := true
	var err error

	for moreToProcess {
		err = WithTx(ctx, func(ctx context.Context, tx *sqlx.Tx) error {
			moreToProcess, err = callbackWithTx(ctx)
			return err
		})

		if err != nil {
			log.Println(err)
		}
	}
}

func recoverWithLog() {
	if r := recover(); r != nil {
		log.Println(r)
	}
}

func ScheduleRecurringQuery(interval time.Duration, query string) {
	go func() {
		defer recoverWithLog()

		// delay initial query somewhat randomly to ensure
		// we don't block startup and queries with the same interval don't run at exactly the same time
		<-time.After(interval / time.Duration(randomish.Int(1, 5)))

		tc := time.NewTicker(interval)
		defer tc.Stop()

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
