package modelperf

import (
	"context"
	"testing"
)

func TestNewPerf(t *testing.T) {

	_, cancel, perf := NewPerf(context.Background(), &PerfInput{})
	cancel()

	perf.GetSummaries()
}
