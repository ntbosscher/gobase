package model

import (
	"context"
	"github.com/jmoiron/sqlx"
	"github.com/ntbosscher/gobase/timeutil"
	"log"
	"time"
)

// ScheduleFunc calls callbackWithTx every interval within a new db transaction.
// While callbackWithTx returns moreToProcess=true, the process will call callbackWithTx again without the interval delay.
// If moreToProcess=false, the process waits for the interval to elapse before calling callbackWithTx again.
//
// Panicking from within callbackWithTx is treated as moreToProcess=false, err!=nil and will not break the process loop
func ScheduleFunc(interval time.Duration, callbackWithTx func(ctx context.Context) (moreToProcess bool, err error)) {
	timeutil.ScheduleJob(interval, func() {

		ctx := context.Background()

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
	})
}

func ScheduleRecurringQuery(interval time.Duration, query string) {
	timeutil.ScheduleJob(interval, func() {
		ctx := context.Background()

		ctx, cleanup, err := BeginTx(ctx, "scheduled-query")
		if err != nil {
			log.Println(err)
			return
		}

		defer cleanup()

		_, err = Tx(ctx).ExecContext(ctx, query)
		if err != nil {
			log.Println(err)
			return
		}

		if err := Commit(ctx); err != nil {
			log.Println(err)
			return
		}
	})
}