package res

import (
	"log"
	"os"

	"github.com/ntbosscher/gobase/env"
	"github.com/ntbosscher/gobase/er"
	"github.com/ntbosscher/gobase/random"
)

// ErrorLog is the destination for full server-side details of a recovered
// panic (raw error message + stack trace), tagged with the correlation id that
// is also returned to the client. Defaults to stderr. Reassign to redirect
// panic diagnostics elsewhere (e.g. a file or structured logger).
var ErrorLog = log.New(os.Stderr, "res: ", log.LstdFlags)

// ReturnErrorMessageToClient, when true, includes the raw error message in the
// client response (alongside the correlation id) instead of GenericErrorMessage.
// The stack trace is still withheld unless env.IsTesting. Defaults to false so
// internal error text (SQL, file paths, etc.) isn't disclosed to callers.
var ReturnErrorMessageToClient = false

// GenericErrorMessage is returned to the client (with the correlation id) for an
// unexpected server error when ReturnErrorMessageToClient is false and not in
// dev mode.
var GenericErrorMessage = "An unexpected error occurred"

// handlePanic builds the client responder for a recovered panic and logs the
// full details server-side, keyed by a correlation id that is echoed to the
// client so a report can be traced back to the log line.
func handlePanic(input *er.HandlerInput) *responder {
	correlationID := newCorrelationID()

	// Always log the full error + stack trace server-side.
	ErrorLog.Printf("[%s] %s\n%s", correlationID, input.Message, input.StackTrace)

	return &responder{
		status: input.SuggestedHttpCode,
		data:   clientErrorData(correlationID, input),
	}
}

// clientErrorData decides how much of a recovered panic is safe to send back to
// the caller. In dev mode everything is exposed; otherwise the stack trace is
// always withheld and the message is generic unless ReturnErrorMessageToClient
// is set. input.Details comes from ErrorWithDetailsForClient and is client-safe
// by design, so it's always included.
func clientErrorData(correlationID string, input *er.HandlerInput) interface{} {
	if env.IsTesting {
		return correlatedErrorData(correlationID, input.Message, input.StackTrace, input.Details)
	}

	message := GenericErrorMessage
	if ReturnErrorMessageToClient {
		message = input.Message
	}

	return correlatedErrorData(correlationID, message, "", input.Details)
}

func correlatedErrorData(correlationID string, errStr string, stackTrace string, details any) interface{} {
	return map[string]interface{}{
		"error":         errStr,
		"message":       "",
		"stackTrace":    stackTrace,
		"details":       details,
		"correlationId": correlationID,
	}
}

func newCorrelationID() string {
	// crypto/rand backed; on the (extremely unlikely) failure path, avoid
	// panicking inside the panic handler.
	id, err := random.GetAlphaNumericChars(12)
	if err != nil {
		return "unknown"
	}

	return id
}
