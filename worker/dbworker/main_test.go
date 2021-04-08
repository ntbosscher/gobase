package dbworker

import (
	"context"
	"github.com/ntbosscher/gobase/worker"
	"testing"
)

func TestDbWorker(t *testing.T) {

	worker.New("test", func(ctx context.Context, input int) error {
		return nil
	}, 0, Middleware)

}
