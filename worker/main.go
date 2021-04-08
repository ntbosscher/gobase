package worker

import (
	"context"
	"github.com/ntbosscher/gobase/er"
	"log"
	"os"
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
	name   string
	exec   Exec
	signal chan int
}

// New creates a new worker and starts the worker loop
// if checkInterval > 0, will execute the worker every interval with input=0
func New(name string, exec Exec, checkInterval time.Duration, middleware ...Middleware) *Worker {
	w := &Worker{
		name:   name,
		exec:   exec,
		signal: make(chan int, 10),
	}

	go w.loop(checkInterval, middleware)

	return w
}

func (w *Worker) loop(checkInterval time.Duration, middleware []Middleware) {
	run := func(ctx context.Context, input int) error {
		defer er.HandleErrors(func(input *er.HandlerInput) {
			Logger.Println("worker "+w.name, input.Message, input.StackTrace)
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

	for {
		value := 0

		select {
		case input, ok := <-w.signal:
			if !ok {
				return
			}
			value = input
		case <-timer:
			value = 0
		}

		err := run(context.Background(), value)
		if err != nil {
			<-time.After(10 * time.Second)
		}
	}
}

func (w *Worker) Stop() {
	close(w.signal)
}

// Trigger executes the job with input=0
// If the queue is full, this does nothing
func (w *Worker) Trigger() {
	select {
	case w.signal <- 0:
	default:
	}
}

// TriggerWithInput executes the job with the given input
// ctx is used to deal with timeouts if the queue is backed up
func (w *Worker) TriggerWithInput(ctx context.Context, input int) {
	select {
	case w.signal <- input:
	case <-ctx.Done():
	}
}
