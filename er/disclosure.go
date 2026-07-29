package er

import (
	"fmt"
	"log"
	"os"

	"github.com/ntbosscher/gobase/env"
	"github.com/ntbosscher/gobase/random"
)

// ErrorLog is the destination for the full server-side details of a recovered
// error (raw message + stack trace), tagged with the correlation id that is also
// surfaced to the consumer so a report can be traced back to the log line.
// Defaults to stderr. Reassign to redirect diagnostics elsewhere (e.g. a file or
// structured logger). Shared by res, bgtaskutil, pqworkqueue and parallelize.
var ErrorLog = log.New(os.Stderr, "er: ", log.LstdFlags)

// ReturnErrorMessageToClient, when true, surfaces the raw error message to the
// consumer (alongside the correlation id) instead of GenericErrorMessage. The
// stack trace is still withheld unless in dev mode (env.IsTesting). Defaults to
// false so internal error text (SQL, file paths, etc.) isn't disclosed.
var ReturnErrorMessageToClient = false

// GenericErrorMessage is surfaced to the consumer (with the correlation id) for
// an unexpected error when ReturnErrorMessageToClient is false and not in dev
// mode.
var GenericErrorMessage = "An unexpected error occurred"

// SafeError logs the full details of a recovered error server-side (keyed by a
// fresh correlation id via ErrorLog) and returns a *SafeErrorObj holding only
// what is safe to surface to the consumer: the correlation id, a client-facing
// message, and a stack trace that is empty unless in dev mode.
//
// Policy:
//   - always: log correlationID + raw message + stack trace to ErrorLog
//   - dev mode (env.IsTesting): clientMessage = raw message, clientStack = full stack
//   - else if input.ClientSafeMessage (see ThrowClientSafe) or ReturnErrorMessageToClient:
//     clientMessage = raw message, clientStack = ""
//   - else: clientMessage = GenericErrorMessage, clientStack = ""
func SafeError(input *HandlerInput) *SafeErrorObj {
	correlationID := newCorrelationID()

	// Always log the full error + stack trace server-side.
	ErrorLog.Printf("[%s] %s\n%s", correlationID, input.Message, input.StackTrace)

	if env.IsTesting {
		return &SafeErrorObj{
			CorrelationID: correlationID,
			ClientMessage: input.Message,
			ClientStack:   input.StackTrace,
		}
	}

	if input.ClientSafeMessage || ReturnErrorMessageToClient {
		return &SafeErrorObj{
			CorrelationID: correlationID,
			ClientMessage: input.Message,
		}
	}

	return &SafeErrorObj{
		CorrelationID: correlationID,
		ClientMessage: GenericErrorMessage,
	}
}

type SafeErrorObj struct {
	CorrelationID string
	ClientMessage string
	ClientStack   string
}

func (s *SafeErrorObj) Error() string {
	return fmt.Sprintf("[%s] %s", s.CorrelationID, s.ClientMessage)
}

func (s *SafeErrorObj) ExtError() string {
	return fmt.Sprintf("[%s] %s\n%s", s.CorrelationID, s.ClientMessage, s.ClientStack)
}

func (s *SafeErrorObj) String() string {
	return s.Error()
}

func newCorrelationID() string {
	// crypto/rand backed; on the (extremely unlikely) failure path, avoid
	// panicking inside an error handler.
	id, err := random.GetAlphaNumericChars(12)
	if err != nil {
		return "unknown"
	}

	return id
}
