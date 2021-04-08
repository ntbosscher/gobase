package worker

import (
	"context"
	"testing"
	"time"
)

func TestWorker(t *testing.T) {
	var signal = make(chan int)
	var standard = New("test", func(ctx context.Context, input int) error {
		<-time.After(100 * time.Millisecond)
		signal <- input
		return nil
	}, 0)

	standard.Trigger()

	select {
	case <-signal:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("failed")
	}

	standard.TriggerWithInput(context.Background(), 3)

	select {
	case value := <-signal:
		if value != 3 {
			t.Fatal("incorrect value")
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("failed")
	}
}

func TestInterval(t *testing.T) {
	var signal = make(chan int)
	New("test", func(ctx context.Context, input int) error {
		signal <- input
		return nil
	}, 10*time.Millisecond)

	select {
	case <-signal:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("failed")
	}
}
