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
		done <- info{Index: index, Error: errors.New(input.Error.Error() + " " + input.StackTrace)}
	})

	err := tsk()
	done <- info{Index: index, Error: err}
}
