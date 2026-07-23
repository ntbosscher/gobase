package ratelimit

import (
	"math"
	"sync"
	"time"
)

// KeyedLimiter applies an independent token-bucket rate limit per key (e.g. one
// bucket per client IP or per account). Unlike Limiter it does not run a refill
// goroutine per bucket: each bucket refills lazily based on elapsed time, and a
// single janitor goroutine evicts idle buckets so memory stays bounded even
// under a flood of distinct keys.
//
// A key that exceeds count events within interval is reported as limited until
// its bucket refills.
func NewKeyed(count int, interval time.Duration) *KeyedLimiter {
	k := &KeyedLimiter{
		count:    count,
		interval: interval,
		buckets:  map[string]*bucket{},
		done:     make(chan struct{}),
	}

	go k.janitor()

	return k
}

type KeyedLimiter struct {
	count    int
	interval time.Duration
	mu       sync.Mutex
	buckets  map[string]*bucket
	done     chan struct{}
	stopOnce sync.Once
}

type bucket struct {
	tokens   float64
	lastFill time.Time
	lastSeen time.Time
}

// refill tops up the key's bucket based on time elapsed since the last touch,
// creating it (full) on first use. Caller must hold k.mu.
func (k *KeyedLimiter) refill(key string, now time.Time) *bucket {
	b, ok := k.buckets[key]
	if !ok {
		b = &bucket{tokens: float64(k.count), lastFill: now}
		k.buckets[key] = b
	}

	if elapsed := now.Sub(b.lastFill); elapsed > 0 {
		added := elapsed.Seconds() / k.interval.Seconds() * float64(k.count)
		b.tokens = math.Min(float64(k.count), b.tokens+added)
		b.lastFill = now
	}

	b.lastSeen = now
	return b
}

// Take consumes one token from the key's bucket, returning ErrRateLimited when
// the bucket is empty.
func (k *KeyedLimiter) Take(key string) error {
	now := time.Now()

	k.mu.Lock()
	defer k.mu.Unlock()

	b := k.refill(key, now)
	if b.tokens >= 1 {
		b.tokens--
		return nil
	}

	return ErrRateLimited
}

// IsLimited reports whether the key would be rate-limited without consuming a
// token. Use it to block a caller and only Take on the events you want to count
// (e.g. failed logins).
func (k *KeyedLimiter) IsLimited(key string) error {
	now := time.Now()

	k.mu.Lock()
	defer k.mu.Unlock()

	b := k.refill(key, now)
	if b.tokens >= 1 {
		return nil
	}

	return ErrRateLimited
}

// Stop halts the janitor goroutine. Safe to call multiple times.
func (k *KeyedLimiter) Stop() {
	k.stopOnce.Do(func() {
		close(k.done)
	})
}

func (k *KeyedLimiter) janitor() {
	// A bucket idle for longer than it takes to fully refill carries no state
	// worth keeping (it would be recreated full anyway), so it's safe to evict.
	ttl := k.interval
	if ttl < time.Minute {
		ttl = time.Minute
	}

	tc := time.NewTicker(ttl)
	defer tc.Stop()

	for {
		select {
		case <-k.done:
			return
		case <-tc.C:
			now := time.Now()
			k.mu.Lock()
			for key, b := range k.buckets {
				if now.Sub(b.lastSeen) > ttl {
					delete(k.buckets, key)
				}
			}
			k.mu.Unlock()
		}
	}
}
