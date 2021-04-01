package er

import (
	"fmt"
	"github.com/pkg/errors"
	"log"
	"net/http"
	"runtime/debug"
)

// HandleErrors deals with panics caused by Check and CheckForDecode
// should call `defer HandleErrors(func(input *HandlerInput) { /* stuff */ })`
func HandleErrors(callback func(input *HandlerInput)) {
	r := recover()
	if r == nil {
		return
	}

	err, ok := r.(error)
	if !ok {
		log.Println("Unknown error: ", r)
		return
	}

	cause, ok := errors.Cause(err).(*HttpError)
	if !ok {
		callback(&HandlerInput{
			Message:           err.Error(),
			SuggestedHttpCode: 500,
			StackTrace:        string(debug.Stack()),
			Error:             err,
		})

		return
	}

	callback(&HandlerInput{
		Message:           cause.Error(),
		SuggestedHttpCode: cause.Code,
		StackTrace:        fmt.Sprintf("%+v", err),
		Error:             err,
	})
}

type HandlerInput struct {
	Message           string
	SuggestedHttpCode int
	StackTrace        string
	Error             error
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
