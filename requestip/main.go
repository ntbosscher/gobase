package requestip

import (
	"context"
	"net/http"
)

func Middleware() func(withIp http.Handler) http.Handler {
	return func(withIP http.Handler) http.Handler {
		return &server{
			next: withIP,
		}
	}
}

type server struct {
	next http.Handler
}

type ipKey string

var _ipKey ipKey = "ip"

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	sources := []string{
		r.Header.Get("X-Forwarded-For"),
		r.Header.Get("X-Real-IP"),
		r.RemoteAddr,
	}

	for _, v := range sources {
		if v != "" {
			ctx = context.WithValue(ctx, _ipKey, v)
			r = r.WithContext(ctx)
			break
		}
	}

	s.next.ServeHTTP(w, r)
}

func IP(ctx context.Context) string {
	return ctx.Value(_ipKey).(string)
}
