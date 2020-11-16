package model

import (
	"context"
	"github.com/ntbosscher/gobase/randomish"
	"log"
	"time"
)

func ScheduleRecurringQuery(interval time.Duration, query string) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Println(r)
			}
		}()

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
