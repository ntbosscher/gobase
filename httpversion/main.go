package httpversion

import (
	"net/http"
	"strconv"
	"strings"
)

var VersionHeaderName = "X-APIVersion"

func Parse(r *http.Request) string {
	return r.Header.Get(VersionHeaderName)
}

func FromRequest(r *http.Request) string {
	return Parse(r)
}

type Ver struct {
	Value string
	Parts []int
}

func NewVer(value string) *Ver {
	ver := &Ver{Value: value}
	parts := strings.Split(value, ".")

	for _, part := range parts {
		i, err := strconv.Atoi(part)
		if err != nil {
			return ver
		}

		ver.Parts = append(ver.Parts, i)
	}

	return ver
}

func (v *Ver) GtOrEq(other *Ver) bool {
	for i := 0; i < len(v.Parts); i++ {
		if len(other.Parts) <= i {
			return true
		}

		if v.Parts[i] > other.Parts[i] {
			return true
		}

		if v.Parts[i] < other.Parts[i] {
			return false
		}
	}

	return true
}
