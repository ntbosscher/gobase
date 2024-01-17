package cors

import (
	"github.com/ntbosscher/gobase/jv"
	"net/http"
	"strings"
)

type corsMiddleware struct {
	allowedOrigins   []string
	next             http.Handler
	allowHeaders     []string
	allowCredentials string
	allowMethods     string
}

func (c *corsMiddleware) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	origin := strings.ToLower(request.Header.Get("Origin"))

	match := jv.First(c.allowedOrigins, func(input string) bool {
		return strings.HasPrefix(origin, input)
	})

	if match != "" {
		writer.Header().Set("Access-Control-Allow-Origin", origin)
		writer.Header().Set("Access-Control-Expose-Headers", "*")
		writer.Header().Set("Access-Control-Allow-Methods", c.allowMethods)
		writer.Header().Set("Access-Control-Allow-Headers", strings.Join(c.allowHeaders, ", "))
		writer.Header().Set("Access-Control-Allow-Credentials", c.allowCredentials)

		if request.Method == "OPTIONS" {
			writer.WriteHeader(http.StatusNoContent)
			return
		}
	}

	c.next.ServeHTTP(writer, request)
}

type WrapOpts struct {
	AllowOrigins []string

	// AllowHeaders response value
	// default: Authorization, Accept, Origin, X-Apiversion, X-Browserwindowid, X-Timezonename, X-Timezoneoffsetmins, Cookie, Content-Type, Cache-Control
	AllowHeaders []string

	// AllowCredentials response value
	// default: true
	AllowCredentials string

	// AllowMethods response value
	// default: *
	AllowMethods string
}

func Wrap(input http.Handler, opts WrapOpts) http.Handler {

	if opts.AllowMethods == "" {
		opts.AllowMethods = "*"
	}

	if opts.AllowCredentials == "" {
		opts.AllowCredentials = "true"
	}

	if opts.AllowHeaders == nil {
		opts.AllowHeaders = []string{
			"Authorization",
			"Accept",
			"Origin",
			"X-Apiversion",
			"X-Browserwindowid",
			"X-Timezonename",
			"X-Timezoneoffsetmins",
			"Cookie",
			"Content-Type",
			"Cache-Control",
		}
	}

	return &corsMiddleware{
		allowedOrigins:   opts.AllowOrigins,
		allowHeaders:     opts.AllowHeaders,
		allowCredentials: opts.AllowCredentials,
		allowMethods:     opts.AllowMethods,
		next:             input,
	}
}
