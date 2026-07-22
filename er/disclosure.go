package er

import (
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
// fresh correlation id via ErrorLog) and returns only what is safe to surface to
// the consumer: the correlation id, a client-facing message, and a stack trace
// that is empty unless in dev mode.
//
// Policy:
//   - always: log correlationID + raw message + stack trace to ErrorLog
//   - dev mode (env.IsTesting): clientMessage = raw message, clientStack = full stack
//   - else if ReturnErrorMessageToClient: clientMessage = raw message, clientStack = ""
//   - else: clientMessage = GenericErrorMessage, clientStack = ""
func SafeError(input *HandlerInput) (correlationID string, clientMessage string, clientStack string) {
	correlationID = newCorrelationID()

	// Always log the full error + stack trace server-side.
	ErrorLog.Printf("[%s] %s\n%s", correlationID, input.Message, input.StackTrace)

	if env.IsTesting {
		return correlationID, input.Message, input.StackTrace
	}

	if ReturnErrorMessageToClient {
		return correlationID, input.Message, ""
	}

	return correlationID, GenericErrorMessage, ""
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
