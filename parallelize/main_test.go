package parallelize

import (
	"errors"
	"testing"
	"time"
)

func TestT(t *testing.T) {

	start := time.Now()

	result := Go(func() error {
		<-time.After(100 * time.Millisecond)
		return nil
	}, func() error {
		<-time.After(100 * time.Millisecond)
		return errors.New("err")
	}, func() error {
		<-time.After(100 * time.Millisecond)
		panic(errors.New("failed"))
	})

	dur := time.Now().Sub(start)

	if dur > 200*time.Millisecond {
		t.Fatal("didn't run in parallel")
	}

	if len(result) != 3 {
		t.Fatal("unexpected number of results")
	}

	if result[0] != nil {
		t.Fatal("unexpected result[0]", result[0])
	}

	if result[1].Error() != "err" {
		t.Fatal("unexpected result[1]", result[1])
	}

	if result[2] == nil {
		t.Fatal("unexpected result[2]", result[2])
	}
}
