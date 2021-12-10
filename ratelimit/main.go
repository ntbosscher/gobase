package ratelimit

import (
	"context"
	"errors"
	"time"
)

func New(count int, interval time.Duration) *Limiter {
	l := &Limiter{
		count:  count,
		bucket: make(chan bool, count),
		tc:     time.NewTicker(interval),
	}

	go l.start()

	return l
}

type Limiter struct {
	count  int
	bucket chan bool
	tc     *time.Ticker
}

func (l *Limiter) Stop() {
	l.tc.Stop()
}

func (l *Limiter) start() {
	for {
		for i := 0; i < l.count; i++ {
			select {
			case l.bucket <- true:
			default:
			}
		}

		<-l.tc.C
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

func (l *Limiter) IsLimited() error {
	select {
	case <-l.bucket:
		l.bucket <- true
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
