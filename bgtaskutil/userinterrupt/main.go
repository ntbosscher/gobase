package userinterrupt

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/ntbosscher/gobase/er"
)

type Job struct {
	Key           string
	userInterrupt chan bool
	closed        chan bool
}

var ErrUserInterrupted error = errors.New("user interrupted (retry)")

var interruptJobs []*Job
var muInterruptJobs = &sync.RWMutex{}

func MakeWorkerContext(ctx context.Context, key string) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancelCause(ctx)
	ctx2, cancel2 := context.WithCancel(ctx)

	j := &Job{
		Key:           key,
		userInterrupt: make(chan bool),
		closed:        make(chan bool),
	}

	addJob(j)

	go func() {
		select {
		case <-j.userInterrupt:
			cancel(ErrUserInterrupted)
			// use gc to if user interrupted
		case <-ctx2.Done():
			return
		}
	}()

	return ctx, func() {
		removeJob(j)
		cancel2()
		close(j.closed)
	}
}

func IsUserInterrupted(ctx context.Context) bool {
	err := ctx.Err()
	if err != nil {
		cause := context.Cause(ctx)

		if strings.Contains(cause.Error(), ErrUserInterrupted.Error()) {
			return true
		}
	}

	return false
}

func UserInterrupt(ctx context.Context, key string) {

	job := getJob(key)
	if job == nil {
		return
	}

	select {
	case job.userInterrupt <- true: // may block b/c a racer may also send, so also check job.closed
		select {
		case <-job.closed:
		}
	case <-job.closed:
		// closed
	case <-ctx.Done():
		er.Check(ctx.Err())
	}
}

func getJob(key string) *Job {
	muInterruptJobs.RLock()
	defer muInterruptJobs.RUnlock()

	for _, job := range interruptJobs {
		if job.Key == key {
			return job
		}
	}

	return nil
}

func addJob(j *Job) {
	muInterruptJobs.Lock()
	defer muInterruptJobs.Unlock()

	interruptJobs = append(interruptJobs, j)
}

func removeJob(j *Job) {
	muInterruptJobs.Lock()
	defer muInterruptJobs.Unlock()

	for i, job := range interruptJobs {
		if job == j {
			interruptJobs = append(interruptJobs[:i], interruptJobs[i+1:]...)
			return
		}
	}
}
