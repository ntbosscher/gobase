package ratelimit

import (
	"context"
	"errors"
	"sync"
	"time"
)

func New(count int, interval time.Duration) *Limiter {
	l := &Limiter{
		count:  count,
		bucket: make(chan bool, count),
		tc:     time.NewTicker(interval),
		done:   make(chan struct{}),
	}

	go l.start()

	return l
}

type Limiter struct {
	count    int
	bucket   chan bool
	tc       *time.Ticker
	done     chan struct{}
	stopOnce sync.Once
}

// Stop halts the refill ticker and the start() goroutine. Safe to call multiple times.
func (l *Limiter) Stop() {
	l.stopOnce.Do(func() {
		l.tc.Stop()
		close(l.done)
	})
}

func (l *Limiter) start() {
	for {
		for i := 0; i < l.count; i++ {
			select {
			case l.bucket <- true:
			default:
			}
		}

		select {
		case <-l.tc.C:
		case <-l.done:
			return
		}
	}
}

func (l *Limiter) Take() error {
	select {
	case <-l.bucket:
		return nil
	default:
		return ErrRateLimited
	}
}

func (l *Limiter) TakeContext(ctx context.Context) error {
	select {
	case <-l.bucket:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// IsLimited reports whether the caller would be rate-limited, without consuming a token.
// The put-back is non-blocking: between the take and the put-back the start() goroutine may
// have refilled the bucket to capacity, so a blocking send here could hang forever.
func (l *Limiter) IsLimited() error {
	select {
	case <-l.bucket:
		select {
		case l.bucket <- true:
		default:
		}
		return nil
	default:
		return ErrRateLimited
	}
}

var ErrRateLimited = errors.New("rate limited")

func (l *Limiter) WaitTake(timeout time.Duration) error {
	select {
	case <-l.bucket:
		return nil
	case <-time.After(timeout):
		return ErrRateLimited
	}
}
