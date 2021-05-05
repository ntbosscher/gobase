package tsv

import (
	"bytes"
	"testing"
)

func TestT(t *testing.T) {
	buf := &bytes.Buffer{}
	enc := NewEncoder(buf)
	enc.WriteRow([]string{"ehllo\" world", "t\tt", `muliline
sentance`})
	enc.WriteRow([]string{"ehllo world", "t\tt", `muliline
sentance`})

	t.Log(buf.String())
}
