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

	cause, ok := errors.Cause(err).(*HttpError)
	if !ok {
		callback(&HandlerInput{
			Message:           err.Error(),
			SuggestedHttpCode: 500,
			StackTrace:        string(debug.Stack()),
			Error:             err,
			Details:           getDetails(r),
		})

		return
	}

	callback(&HandlerInput{
		Message:           cause.Error(),
		SuggestedHttpCode: cause.Code,
		StackTrace:        fmt.Sprintf("%+v", err),
		Error:             err,
		Details:           getDetails(r),
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

type HandlerInput struct {
	Message           string
	SuggestedHttpCode int
	StackTrace        string
	Error             error
	Details           any
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
	return h.Err
}

func (h *HttpError) Error() string {
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
