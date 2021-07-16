package version

import (
	"context"
	"github.com/ntbosscher/gobase/httpversion"
	"github.com/ntbosscher/gobase/res"
	"github.com/ntbosscher/gobase/res/r"
)

type versionKeyType string

const versionCtxKey versionKeyType = "version"

func Middleware() r.Middleware {
	return func(router *r.Router, method string, path string, handler res.HandlerFunc2) res.HandlerFunc2 {
		return func(rq *res.Request) res.Responder {
			req := rq.Request()
			ctx := context.WithValue(req.Context(), versionCtxKey, httpversion.NewVer(httpversion.FromRequest(req)))
			req = req.WithContext(ctx)

			return handler(rq)
		}
	}
}

func Version(ctx context.Context) *httpversion.Ver {
	value := ctx.Value(versionCtxKey)
	if value == nil {
		return nil
	}

	return value.(*httpversion.Ver)
}
