package demux

import (
	"context"
	"sync"
)

type Demux struct {
	outputs []chan interface{}
	mutex   sync.Mutex

	// Lossy allows demux to drop values
	// when the output isn't ready to receive
	Lossy bool

	// Buffer creates a buffered output channels
	Buffer int
}

func (d *Demux) Send(value interface{}) {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	for _, pipe := range d.outputs {
		if d.Lossy {
			select {
			case pipe <- value:
			default:
				// drop value b/c pipe isn't ready to receive
			}
		} else {
			pipe <- value
		}
	}
}

func (d *Demux) removeOutput(c chan interface{}) {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	close(c)

	var filtered []chan interface{}
	for _, output := range d.outputs {
		if output != c {
			filtered = append(filtered, output)
		}
	}

	d.outputs = filtered
}

// Receive creates a new output channel
//
// When you're done with the channel, cancelling the context will cleanup
// any resources
func (d *Demux) Receive(ctx context.Context) <-chan interface{} {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	c := make(chan interface{}, d.Buffer)
	d.outputs = append(d.outputs, c)

	go func() {
		<-ctx.Done()
		d.removeOutput(c)
	}()

	return c
}