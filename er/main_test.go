package er

import (
	"encoding/json"
	"github.com/pkg/errors"
	"testing"
)

func TestErr(t *testing.T) {

	defer HandleErrors(func(input *HandlerInput) {
		js, _ := json.MarshalIndent(input, "", "\t")
		t.Error(string(js))
	})

	err := errors.New("test")
	Check(err)
}
