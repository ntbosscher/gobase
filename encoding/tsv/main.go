package tsv

import (
	"io"
	"strings"
)

type Encoder interface {
	WriteRow(values []string) error
}

type encoder struct {
	wr io.Writer
}

func NewEncoder(wr io.Writer) Encoder {
	return &encoder{
		wr: wr,
	}
}

func (e *encoder) WriteRow(values []string) error {
	encodeValues(values)
	tsv := strings.Join(values, "\t") + "\n"

	_, err := e.wr.Write([]byte(tsv))
	return err
}

func encodeValues(values []string) {
	for i, v := range values {
		needsQuote := strings.ContainsAny(v, "\n\t\r\\\"")
		if !needsQuote {
			continue
		}

		v = strings.ReplaceAll(v, `"`, `""`)

		values[i] = `"` + v + `"`
	}
}
