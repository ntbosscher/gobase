// Worker is a easy way to run background jobs within the same process.
// For multi-processed jobs, see pqworkerqueue
package worker

import (
	"context"
	"errors"
	"github.com/ntbosscher/gobase/er"
	"log"
	"os"
	"sync"
	"time"
)

var Logger = log.New(os.Stdout, "worker", log.Lshortfile)

// Exec is the worker function. input=0 by default
// If TriggerWithInput is called, the input passed there will be used
// If your worker executes in a model.Tx, you may wish so use the dbworker.Middlware
type Exec = func(ctx context.Context, input int) error

// Middleware allows transformation of the context and input before the worker is executed
type Middleware = func(next Exec) Exec

type Worker struct {
	name     string
	exec     Exec
	signal   chan int
	done     chan struct{}
	stopOnce sync.Once
}

// New creates a new worker and starts the worker loop
// if checkInterval > 0, will execute the worker every interval with input=0
func New(name string, exec Exec, checkInterval time.Duration, middleware ...Middleware) *Worker {
	w := &Worker{
		name:   name,
		exec:   exec,
		signal: make(chan int, 10),
		done:   make(chan struct{}),
	}

	go w.loop(checkInterval, middleware)

	return w
}

func (w *Worker) loop(checkInterval time.Duration, middleware []Middleware) {
	run := func(ctx context.Context, input int) (err error) {
		defer er.HandleErrors(func(input *er.HandlerInput) {
			err = errors.New("worker panic: " + input.Message + " " + input.StackTrace)
		})

		exec := w.exec

		for _, md := range middleware {
			exec = md(exec)
		}

		return exec(ctx, input)
	}

	var timer <-chan time.Time

	if checkInterval == 0 {
		timer = make(chan time.Time)
	} else {
		tc := time.NewTicker(checkInterval)
		defer tc.Stop()
		timer = tc.C
	}

	ctx := context.Background()
	ctx = context.WithValue(ctx, workerKey, w)

	for {
		value := 0

		select {
		case <-w.done:
			return
		case value = <-w.signal:
		case <-timer:
			value = 0
		}

		err := run(ctx, value)
		if err != nil {
			Logger.Println("worker "+w.name, err.Error())

			// Back off after an error, but stay responsive to Stop() so
			// shutdown isn't delayed up to 10s.
			select {
			case <-time.After(10 * time.Second):
			case <-w.done:
				return
			}
		}
	}
}

// Stop signals the worker loop to exit. It is safe to call multiple times and
// concurrently with Trigger/TriggerWithInput.
func (w *Worker) Stop() {
	w.stopOnce.Do(func() {
		close(w.done)
	})
}

// Trigger executes the job with input=0
// If the queue is full or the worker has stopped, this does nothing
func (w *Worker) Trigger() {
	select {
	case w.signal <- 0:
	case <-w.done:
	default:
	}
}

// TriggerWithInput executes the job with the given input
// ctx is used to deal with timeouts if the queue is backed up
// Returns immediately (without enqueuing) if the worker has stopped.
func (w *Worker) TriggerWithInput(ctx context.Context, input int) {
	select {
	case w.signal <- input:
	case <-ctx.Done():
	case <-w.done:
	}
}

func (w *Worker) Name() string {
	return w.name
}

func WithTimeLimitMiddleware(limit time.Duration) Middleware {

	return func(next Exec) Exec {
		return func(ctx context.Context, input int) error {
			ctx, cancel := context.WithTimeout(ctx, limit)
			defer cancel()

			return next(ctx, input)
		}
	}
}

type contextKey string

const workerKey contextKey = "worker-key"

func Current(ctx context.Context) *Worker {
	return ctx.Value(workerKey).(*Worker)
}
