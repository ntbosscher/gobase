package res

import (
	"github.com/ntbosscher/gobase/er"
)

// handlePanic builds the client responder for a recovered panic. The disclosure
// policy (log full details server-side under a correlation id, generic message
// to the client by default, full detail in dev mode) is shared across the
// framework and lives in the er package — see er.SafeError, er.ErrorLog,
// er.ReturnErrorMessageToClient and er.GenericErrorMessage.
func handlePanic(input *er.HandlerInput) *responder {
	correlationID, message, stackTrace := er.SafeError(input)

	return &responder{
		status: input.SuggestedHttpCode,
		data: map[string]interface{}{
			// input.Details comes from ErrorWithDetailsForClient and is
			// client-safe by design, so it's always passed through.
			"error":         message,
			"message":       "",
			"stackTrace":    stackTrace,
			"details":       input.Details,
			"correlationId": correlationID,
		},
	}
}
