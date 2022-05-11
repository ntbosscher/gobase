package pkglog

import (
	"fmt"
	"testing"
)

// logger is configurable using env TEST_LOG_LEVEL=verbose|info|error|none
var logger = New("test")

func TestLogExample(t *testing.T) {

	logger.Println("hello world (info)")
	logger.Info.Println("hello world (also info)")
	logger.Error.Println("hello world (error)")
	logger.Verbose.Println("hello world (verbose)")

	logger.SetLevel(Error)
	fmt.Println("error-only:")

	logger.Println("hello world (info)")
	logger.Info.Println("hello world (also info)")
	logger.Error.Println("hello world (error)")
	logger.Verbose.Println("hello world (verbose)")

	fmt.Println("\tdone")
}
