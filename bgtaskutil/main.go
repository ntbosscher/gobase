package bgtaskutil

import (
	"context"
	"log"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/ntbosscher/gobase/bgtaskutil/userinterrupt"
	"github.com/ntbosscher/gobase/er"
	"github.com/ntbosscher/gobase/jv"
	"github.com/ntbosscher/gobase/model"
	"github.com/ntbosscher/gobase/pqworkqueue"
	"github.com/ntbosscher/gobase/res"
)

func init() {
	hasLockingErrorC = make(chan bool)
	go managePQLockingDelay()
}

var hasLockingErrorC chan bool
var lockingDelay time.Duration
var lockingDelayMu = &sync.RWMutex{}

func getLockingDelay() time.Duration {
	lockingDelayMu.RLock()
	defer lockingDelayMu.RUnlock()

	return lockingDelay + time.Duration(rand.Float64()*float64(lockingDelay)*0.25) // add some jitter to prevent recreating locking situation
}

func managePQLockingDelay() {
	defer er.HandleErrors(func(input *er.HandlerInput) {
		log.Println(input)
	})

	tc := time.NewTicker(10 * time.Second)
	noErrorsSince := time.Time{}
	recentErrCount := 0

	setDelay := func(v time.Duration) {
		lockingDelayMu.Lock()
		defer lockingDelayMu.Unlock()

		lockingDelay = v
	}

	calcRequeueDelay := func() {
		switch {
		case recentErrCount > 4:
			setDelay(20 * time.Second)
		case recentErrCount >= 2:
			setDelay(10 * time.Second)
		case recentErrCount < 2:
			setDelay(time.Second)
		default:
			setDelay(30 * time.Second)
		}
	}

	for {
		select {
		case <-hasLockingErrorC:
			recentErrCount++
			noErrorsSince = time.Now()
			calcRequeueDelay()
		case <-tc.C:
			if recentErrCount <= 0 {
				recentErrCount = 0
				continue
			}

			recentErrCount = recentErrCount - max(recentErrCount/4, 1)
			if time.Since(noErrorsSince) > 60*time.Second {
				recentErrCount = 0
			}

			calcRequeueDelay()
		}
	}
}

func Retry[T any](ctx context.Context, job T, queue *pqworkqueue.Queue2[T], delay time.Duration) error {
	_, err := queue.AddOpt(ctx, job, &pqworkqueue.AddOption2[T]{
		StartAfter: time.Now().Add(delay),
	})

	return err
}

func RetryPQLockingErrors[T any](reason *er.HandlerInput, job T, queue *pqworkqueue.Queue2[T]) bool {
	if reason == nil || reason.Error == nil {
		return false
	}

	phrases := []string{
		"could not serialize access due to concurrent update",
		"deadlock detected",
	}

	if !jv.Any(phrases, func(search string) bool {
		return strings.Contains(reason.Error.Error(), search)
	}) {
		return false
	}

	ctxNew := context.Background()
	ctxNew, cancel := context.WithTimeout(ctxNew, time.Minute)
	defer cancel()

	select {
	case hasLockingErrorC <- true:
	default:
		log.Println("hasLockingErrorC not accepting, weird")
	}

	err := model.WithTx(ctxNew, func(ctx context.Context, tx *sqlx.Tx) error {
		_, err := queue.AddOpt(ctx, job, &pqworkqueue.AddOption2[T]{
			StartAfter: time.Now().Add(getLockingDelay()),
		})

		return err
	})

	if err == nil {
		return true
	}

	log.Println(err)
	return false
}

type UserInterrupter[T any] struct {
	keyMaker func(input T) string
}

func NewUserInterrupt[T any](keyMaker func(input T) string) *UserInterrupter[T] {
	return &UserInterrupter[T]{
		keyMaker: keyMaker,
	}
}

func IsUserInterupt(ctx context.Context, reason *er.HandlerInput) bool {
	// need context check for psql errors
	hasInterruptText := reason != nil && reason.Error != nil &&
		strings.Contains(reason.Error.Error(), userinterrupt.ErrUserInterrupted.Error())

	if !hasInterruptText && !userinterrupt.IsUserInterrupted(ctx) {
		return false
	}

	return true
}

func RetryUserInterrupted[T any](ctx context.Context, reason *er.HandlerInput, job T, queue *pqworkqueue.Queue2[T]) bool {
	// need context check for psql errors
	if !IsUserInterupt(ctx, reason) {
		return false
	}

	ctxNew := context.Background()
	ctxNew, cancel := context.WithTimeout(ctxNew, time.Minute)
	defer cancel()

	err := model.WithTx(ctxNew, func(ctx context.Context, tx *sqlx.Tx) error {
		_, err := queue.AddOpt(ctx, job, &pqworkqueue.AddOption2[T]{
			StartAfter: time.Now().Add(15 * time.Second),
		})

		return err
	})

	if err == nil {
		return true
	}

	log.Println(err)
	return false
}

func WithInterrupt[T any](ctx context.Context, interrupt *UserInterrupter[T], arg T) (context.Context, context.CancelFunc) {
	key := interrupt.keyMaker(arg)
	return userinterrupt.MakeWorkerContext(ctx, key)
}

func Interrupt[T any](ctx context.Context, interrupt *UserInterrupter[T], arg T) {
	key := interrupt.keyMaker(arg)
	userinterrupt.UserInterrupt(ctx, key)
}

func JsonErrorResult(input *er.HandlerInput) []byte {

	// Disclosure policy (log full details server-side under a correlation id,
	// generic message + empty stack to the consumer by default, full detail in
	// dev mode) is shared across the framework — see er.SafeError.
	correlationID, message, stackTrace := er.SafeError(input)

	content, _ := res.GetJSONInstance().Marshal(map[string]interface{}{
		"error":         message,
		"detail":        stackTrace,
		"correlationId": correlationID,
	})

	return content
}

func JsonErrorResult2(err error) []byte {
	content, _ := res.GetJSONInstance().Marshal(map[string]interface{}{
		"error": err.Error(),
	})

	return content
}

func BlankResult() []byte {
	return []byte("{}")
}

func JsonResult(input any) []byte {
	content, _ := res.GetJSONInstance().Marshal(input)
	return content
}

func TaskResult(id string) res.Responder {
	return res.Ok(map[string]interface{}{
		"task": id,
	})
}
