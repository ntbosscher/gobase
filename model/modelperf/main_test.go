package modelperf

import (
	"context"
	"testing"
)

func TestNewPerf(t *testing.T) {

	_, cancel, perf := New(context.Background(), &PerfInput{})
	cancel()

	perf.GetSummaries()
}
