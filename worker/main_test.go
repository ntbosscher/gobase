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

func TestStopIsSafe(t *testing.T) {
	w := New("test", func(ctx context.Context, input int) error { return nil }, 0)

	// Previously these panicked with "send on closed channel" / "close of
	// closed channel". They must now be no-ops.
	w.Stop()
	w.Stop() // double stop
	w.Trigger()
	w.TriggerWithInput(context.Background(), 1)
}

func TestStopConcurrentWithTrigger(t *testing.T) {
	w := New("test", func(ctx context.Context, input int) error { return nil }, 0)

	done := make(chan struct{})
	go func() {
		for i := 0; i < 1000; i++ {
			w.Trigger()
			w.TriggerWithInput(context.Background(), i)
		}
		close(done)
	}()

	w.Stop()
	<-done // must complete without a panic taking down the process
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

func TestTimeLimit(t *testing.T) {
	var signal = make(chan int)
	New("test", func(ctx context.Context, input int) error {
		select {
		case <-time.After(time.Second):
			t.Fatal("should have timed out in the context")
		case <-ctx.Done():
		}

		signal <- input
		return nil
	}, 10*time.Millisecond, WithTimeLimitMiddleware(100*time.Millisecond))

	select {
	case <-signal:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("failed")
	}
}
