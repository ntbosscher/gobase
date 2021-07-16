package apiversion

import (
	"context"
	"github.com/gorilla/mux"
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

func (v *Ver) String() string {
	return v.Value
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

	if len(v.Parts) == 0 && len(other.Parts) > 0 {
		return false
	}

	return true
}

type versionKeyType string

const versionCtxKey versionKeyType = "version"

func Middleware() mux.MiddlewareFunc {
	return func(handler http.Handler) http.Handler {
		return &versionRouter{
			next: handler,
		}
	}
}

type versionRouter struct {
	next http.Handler
}

func (v *versionRouter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := context.WithValue(r.Context(), versionCtxKey, NewVer(FromRequest(r)))
	r = r.WithContext(ctx)
	v.next.ServeHTTP(w, r)
}

func Current(ctx context.Context) *Ver {
	value := ctx.Value(versionCtxKey)
	if value == nil {
		return nil
	}

	return value.(*Ver)
}
