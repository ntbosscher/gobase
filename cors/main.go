package cors

import (
	"github.com/ntbosscher/gobase/jv"
	"net/http"
	"strings"
)

type corsMiddleware struct {
	allowedOrigins []string
	next           http.Handler
}

func (c *corsMiddleware) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	origin := strings.ToLower(request.Header.Get("Origin"))

	match := jv.First(c.allowedOrigins, func(input string) bool {
		return strings.HasPrefix(origin, input)
	})

	if match != "" {
		writer.Header().Set("Access-Control-Allow-Origin", origin)
		writer.Header().Set("Access-Control-Expose-Headers", "*")
		writer.Header().Set("Access-Control-Allow-Methods", "*")
		writer.Header().Set("Access-Control-Allow-Headers", "Authorization, Accept, Origin, X-Apiversion, X-Browserwindowid, X-Timezonename, X-Timezoneoffsetmins, Cookie, Content-Type, Cache-Control")
		writer.Header().Set("Access-Control-Allow-Credentials", "true")

		if request.Method == "OPTIONS" {
			writer.WriteHeader(http.StatusNoContent)
			return
		}
	}

	c.next.ServeHTTP(writer, request)
}

func Wrap(input http.Handler, allowedOrigins []string) http.Handler {

	return &corsMiddleware{
		allowedOrigins: allowedOrigins,
		next:           input,
	}
}
