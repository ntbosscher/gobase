// Package pqchan is a utility to broadcast messages between applications
// connected to the same postgres database (using postgres message channels)
package pqchan

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/ntbosscher/gobase/demux"
	"github.com/ntbosscher/gobase/er"
	"github.com/ntbosscher/gobase/pqshared"
	"log"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

var muOutputs sync.RWMutex
var outputs = map[string]*demux.Demux{}
var outputNamesChangedC chan bool

func init() {
	outputNamesChangedC = make(chan bool)

	go manager()
}

func manager() {

	// run dbListener, but re-start it every time the outputNames are updated
	for {
		ctx, cancel := context.WithCancel(context.Background())

		go func() {
			for {
				dbListener(ctx)
				if ctx.Err() == context.Canceled {
					return
				}

				// delay next execution b/c we're probably here b/c of an error
				<-time.After(1 * time.Second)
			}
		}()

		<-outputNamesChangedC
		cancel()
	}
}

var nameRegexpStr = `^[A-z\_0-9]+$`
var nameValidation = regexp.MustCompile(nameRegexpStr)

func MustSend(ctx context.Context, name string, value interface{}) {
	er.Check(Send(ctx, name, value))
}

func Send(ctx context.Context, name string, value interface{}) error {

	if !nameValidation.MatchString(name) {
		return errors.New("invalid channel name, must match " + nameRegexpStr)
	}

	js, err := json.Marshal(value)
	if err != nil {
		return err
	}

	_, err = pqshared.Pool.Exec(ctx, `select pg_notify($1, $2)`, channelPrefix+name, js)
	return err
}

func dbListener(ctx context.Context) {
	defer er.HandleErrors(func(input *er.HandlerInput) {
		Logger.Println(input.Error, input.StackTrace)
	})

	conn, err := pqshared.Pool.Acquire(ctx)
	if err != nil {
		if err == context.Canceled {
			return
		}

		Logger.Println("unable to get connection", err)
		return
	}

	defer conn.Release()

	if err := setupListens(ctx, conn); err != nil {
		if err == context.Canceled {
			return
		}

		Logger.Println("unable to setup listeners", err)
		return
	}

	for {
		notif, err := conn.Conn().WaitForNotification(ctx)
		if err != nil {
			if ctx.Err() == context.Canceled {
				return
			}

			Logger.Println(err)
			return
		}

		broadcastNotification(notif.Channel, notif.Payload)
	}
}

func broadcastNotification(channel string, payload string) {
	muOutputs.RLock()
	defer muOutputs.RUnlock()

	name := strings.TrimPrefix(channel, channelPrefix)

	mux := outputs[name]
	if mux == nil {
		return
	}

	mux.Send(json.RawMessage(payload))
}

func setupListens(ctx context.Context, conn *pgxpool.Conn) error {

	muOutputs.RLock()
	defer muOutputs.RUnlock()

	for name, _ := range outputs {
		if _, err := conn.Exec(ctx, `listen `+channelPrefix+name); err != nil {
			if ctx.Err() == context.Canceled {
				return ctx.Err()
			}

			return err
		}
	}

	return nil
}

func getMuxForName(name string) *demux.Demux {
	muOutputs.Lock()
	defer muOutputs.Unlock()

	mux := outputs[name]

	if mux == nil {
		mux = &demux.Demux{
			Lossy:  false,
			Buffer: 5,
		}

		outputs[name] = mux
		outputNamesChangedC <- true
	}

	return mux
}

var channelPrefix = "pqchan_"

var Logger = log.New(os.Stderr, "pqchan: ", log.Llongfile)

func MustReceive(ctx context.Context, name string) chan json.RawMessage {
	value, err := Receive(ctx, name)
	er.Check(err)
	return value
}

func Receive(ctx context.Context, name string) (chan json.RawMessage, error) {
	if !nameValidation.MatchString(name) {
		return nil, errors.New("invalid channel name, must match " + nameRegexpStr)
	}

	mux := getMuxForName(name)

	channel := make(chan json.RawMessage)

	go func() {
		defer er.HandleErrors(func(input *er.HandlerInput) {
			Logger.Println(input.Error, input.StackTrace)
		})

		defer close(channel)

		for input := range mux.Receive(ctx) {
			select {
			case channel <- input.(json.RawMessage):
			case <-ctx.Done():
				return
			}
		}
	}()

	return channel, nil
}
