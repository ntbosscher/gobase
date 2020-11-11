package demux

import (
	"context"
	"testing"
	"time"
)

func TestDemux(t *testing.T) {
	d := &Demux{}

	ctx, cancel := context.WithCancel(context.Background())
	gotResult := make(chan bool)
	ready := make(chan bool)

	go func () {
		c := d.Receive(ctx)
		ready <- true
		<-c
		gotResult <- true
	}()

	<-ready
	d.Send(true)
	<-gotResult

	cancel()

	// wait for cleanup
	<-time.After(1 * time.Millisecond)

	if len(d.outputs) != 0 {
		t.Fatal("should have cleaned up output")
	}

}
