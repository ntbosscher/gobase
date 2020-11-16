package requestip

import (
	"context"
	"net"
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

	var source string

	for _, v := range sources {
		if v != "" {
			source = v
			break
		}
	}

	if source != "" {
		// remove port if source contains a port
		host, _, err := net.SplitHostPort(source)
		if err == nil {
			source = host
		}

		ctx = context.WithValue(ctx, _ipKey, source)
		r = r.WithContext(ctx)
	}

	s.next.ServeHTTP(w, r)
}

func IP(ctx context.Context) string {
	value := ctx.Value(_ipKey)
	if value == nil {
		return ""
	}

	return value.(string)
}
