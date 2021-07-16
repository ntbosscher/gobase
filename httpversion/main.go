package httpversion

import (
	"context"
	"github.com/ntbosscher/gobase/res"
	"github.com/ntbosscher/gobase/res/r"
	"net/http"
	"strconv"
	"strings"
)

var VersionHeaderName = "X-APIVersion"

func Parse(r *http.Request) string {
	return r.Header.Get(VersionHeaderName)
}

type versionKeyType string

const versionCtxKey versionKeyType = "version"

func Middleware() r.Middleware {
	return func(router *r.Router, method string, path string, handler res.HandlerFunc2) res.HandlerFunc2 {
		return func(rq *res.Request) res.Responder {
			req := rq.Request()
			ctx := context.WithValue(req.Context(), versionCtxKey, NewVer(Parse(req)))
			req = req.WithContext(ctx)

			return handler(rq)
		}
	}
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

func Version(ctx context.Context) *Ver {
	value := ctx.Value(versionCtxKey)
	if value == nil {
		return nil
	}

	return value.(*Ver)
}
