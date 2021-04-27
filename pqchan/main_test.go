package pqchan

import (
	"context"
	"encoding/json"
	"github.com/ntbosscher/gobase/er"
	"testing"
	"time"
)

func TestT(t *testing.T) {
	info := MustReceive(context.Background(), "test")
	MustSend(context.Background(), "test", "my-value")

	select {
	case value := <-info:
		str := ""
		er.Check(json.Unmarshal(value, &str))

		if str != "my-value" {
			t.Error("unexpected value")
		}
	case <-time.After(1 * time.Second):
		t.Fatal("took too long")
	}
}
