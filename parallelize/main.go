package parallelize

import (
	"errors"
	"github.com/ntbosscher/gobase/er"
)

type info struct {
	Index int
	Error error
}

func Run(tasks ...func() error) []error {

	done := make(chan info)

	for i, task := range tasks {
		go handler(done, i, task)
	}

	list := make([]error, len(tasks))

	for i := 0; i < len(tasks); i++ {
		err := <-done
		list[err.Index] = err.Error
	}

	return list
}

func handler(done chan info, index int, tsk func() error) {
	defer er.HandleErrors(func(input *er.HandlerInput) {
		// Full details are logged server-side under a correlation id; the
		// returned error carries only the safe view (no stack trace outside dev
		// mode). See er.SafeError.
		safe := er.SafeError(input)

		done <- info{Index: index, Error: safe}
	})

	err := tsk()
	done <- info{Index: index, Error: err}
}
