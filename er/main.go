package er

import (
	"fmt"
	"net/http"
	"runtime/debug"

	"github.com/pkg/errors"
)

var ErrUnknown = errors.New("unknown error (probably thrown by a panic, not er.Check)")

// HandleErrors deals with panics caused by Check and CheckForDecode
// should call `defer HandleErrors(func(input *HandlerInput) { /* stuff */ })`
func HandleErrors(callback func(input *HandlerInput)) {
	r := recover()
	if r == nil {
		return
	}

	err, ok := r.(error)
	if !ok {
		callback(&HandlerInput{
			Message:           "Unknown error: " + fmt.Sprint(err),
			SuggestedHttpCode: 500,
			StackTrace:        string(debug.Stack()),
			Error:             ErrUnknown,
			Details:           getDetails(r),
		})

		return
	}

	clientSafe := isClientSafeMessage(err)

	cause, ok := errors.Cause(err).(*HttpError)
	if !ok {
		callback(&HandlerInput{
			Message:           err.Error(),
			SuggestedHttpCode: 500,
			StackTrace:        string(debug.Stack()),
			Error:             err,
			Details:           getDetails(err),
			ClientSafeMessage: clientSafe,
		})

		return
	}

	callback(&HandlerInput{
		Message:           cause.Error(),
		SuggestedHttpCode: cause.Code,
		StackTrace:        fmt.Sprintf("%+v", err),
		Error:             err,
		Details:           getDetails(cause.Err),
		ClientSafeMessage: clientSafe,
	})
}

type ErrorWithDetailsForClient interface {
	GetClientDetails() any
}

func getDetails(input any) any {
	if input == nil {
		return nil
	}

	if details, ok := input.(ErrorWithDetailsForClient); ok {
		return details.GetClientDetails()
	}

	return nil
}

// ClientSafeErr marks an error whose Error() message is safe to display to the
// consumer verbatim. SafeError surfaces such a message regardless of the
// ReturnErrorMessageToClient setting. The check walks the error chain, so it
// works even when the error is wrapped (e.g. by er.Check's HttpError).
type ClientSafeErr interface {
	IsClientSafeErr() bool
}

func isClientSafeMessage(err error) bool {
	var target ClientSafeErr
	if errors.As(err, &target) {
		return target.IsClientSafeErr()
	}

	return false
}

type HandlerInput struct {
	Message           string
	SuggestedHttpCode int
	StackTrace        string
	Error             error
	Details           any

	// ClientSafeMessage is true when the recovered error opted in to having its
	// Message shown to the consumer verbatim (see ClientSafeMessager /
	// ThrowClientSafe). SafeError surfaces Message instead of the generic
	// message when this is set.
	ClientSafeMessage bool
}

func CheckForDecode(err error) {
	if err == nil {
		return
	}

	panic(errors.WithStack(&HttpError{
		Code: http.StatusBadRequest,
		Err:  err,
	}))
}

type HttpError struct {
	Code int
	Err  error
}

func (h *HttpError) Unwrap() error {
	if h == nil {
		return nil
	}
	return h.Err
}

func (h *HttpError) Error() string {
	// Guard against a HttpError constructed without an underlying Err (e.g.
	// panic(&HttpError{Code: 500})). Error()/Unwrap() run inside the
	// panic-recovery path (HandleErrors); a nil-deref here would panic the
	// recover handler and crash the whole process.
	if h == nil || h.Err == nil {
		return "http error"
	}
	return h.Err.Error()
}

func Check(err error) {
	if err == nil {
		return
	}

	panic(errors.WithStack(&HttpError{
		Code: http.StatusInternalServerError,
		Err:  err,
	}))
}

func Throw(value string) {
	Check(errors.New(value))
}

type errorWithDetailsImpl struct {
	Value   string
	Details any
}

func (e *errorWithDetailsImpl) Error() string {
	return e.Value
}

func (e *errorWithDetailsImpl) GetClientDetails() any {
	return e.Details
}

func ThrowWithClientDetails(value string, input any) {
	Check(&errorWithDetailsImpl{
		Value:   value,
		Details: input,
	})
}

type clientSafeMessageErr struct {
	value string
}

func (e *clientSafeMessageErr) Error() string {
	return e.value
}

func (e *clientSafeMessageErr) IsClientSafeErr() bool {
	return true
}

// ThrowClientSafe panics with an error whose message SafeError always surfaces
// to the consumer, regardless of ReturnErrorMessageToClient. Use it for
// user-facing messages (validation / business-rule failures) that are meant to
// be read by the end user, e.g. er.ThrowClientSafe("Email is already in use").
//
// The full details (message + stack) are still logged server-side under the
// correlation id. Responds with HTTP 400, since a client-displayable message is
// almost always a bad-request condition; use ThrowClientSafeCode for a
// different status.
func ThrowClientSafe(message string) {
	ThrowClientSafeCode(http.StatusBadRequest, message)
}

// ThrowClientSafeCode is ThrowClientSafe with an explicit HTTP status code.
func ThrowClientSafeCode(code int, message string) {
	panic(errors.WithStack(&HttpError{
		Code: code,
		Err:  &clientSafeMessageErr{value: message},
	}))
}

// ThrowClientSafef is ThrowClientSafe with a fmt.Sprintf-style message.
func ThrowClientSafef(format string, args ...any) {
	ThrowClientSafeCode(http.StatusBadRequest, fmt.Sprintf(format, args...))
}
