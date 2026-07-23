package ratelimit

import (
	"testing"
	"time"
)

func TestKeyedLimiterPerKey(t *testing.T) {
	k := NewKeyed(3, time.Minute)
	defer k.Stop()

	// "a" burns its 3 tokens, then is limited.
	for i := 0; i < 3; i++ {
		if err := k.Take("a"); err != nil {
			t.Fatalf("take %d for a should succeed: %v", i, err)
		}
	}

	if err := k.Take("a"); err == nil {
		t.Fatal("a should be limited after exhausting its bucket")
	}

	if err := k.IsLimited("a"); err == nil {
		t.Fatal("IsLimited should report a as limited")
	}

	// "b" has its own independent bucket.
	if err := k.IsLimited("b"); err != nil {
		t.Fatalf("b should not be limited: %v", err)
	}

	if err := k.Take("b"); err != nil {
		t.Fatalf("b should be able to take: %v", err)
	}
}

func TestKeyedLimiterRefill(t *testing.T) {
	// 60 tokens/minute == 1 token every second.
	k := NewKeyed(60, time.Minute)
	defer k.Stop()

	for i := 0; i < 60; i++ {
		if err := k.Take("a"); err != nil {
			t.Fatalf("take %d should succeed: %v", i, err)
		}
	}

	if err := k.Take("a"); err == nil {
		t.Fatal("a should be limited after 60 takes")
	}

	// After ~1.1s at least one token should have refilled.
	time.Sleep(1100 * time.Millisecond)

	if err := k.Take("a"); err != nil {
		t.Fatalf("a should have refilled a token: %v", err)
	}
}
