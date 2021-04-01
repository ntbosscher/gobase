package r

import (
	"errors"
	"github.com/ntbosscher/gobase/auth"
	"github.com/ntbosscher/gobase/auth/httpauth"
	"github.com/ntbosscher/gobase/er"
	"github.com/ntbosscher/gobase/ratelimit"
	"github.com/ntbosscher/gobase/res"
	"github.com/ntbosscher/gobase/strs"
	errors2 "github.com/pkg/errors"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"reflect"
	"strings"
	"time"
)

var logger = log.New(os.Stderr, "gobase/res/r: ", log.Llongfile)

func DisableLogging() {
	logger = log.New(ioutil.Discard, "", log.Llongfile)
}

type Router struct {
	*res.Router
	auth     *httpauth.AuthRouter
	hasRoute bool
}

func NewRouter() *Router {
	return &Router{
		Router: res.NewRouter(),
	}
}

func (r *Router) WithAuth(config httpauth.Config) *httpauth.AuthRouter {
	if r.hasRoute {
		log.Fatal("must call .WithAuth() before any routing setup (e.g. Get('/...')")
	}

	r.auth = httpauth.Setup(r.Router, config)
	return r.auth
}

func (r *Router) Get(path string) *Configure {
	return r.Route("GET", path)
}

type Middleware func(router *Router, method string, path string, handler res.HandlerFunc2) res.HandlerFunc2

type RouteConfig interface{}

// Add adds a route to the router.
// This route will be publicly accessible unless otherwise specified in 'Middleware' parameter
// or through WithRole()
func (r *Router) Add(method string, path string, config ...RouteConfig) {

	cfg := r.Route(method, path)

	requiredRole := auth.Public
	var handler res.HandlerFunc2

	configs := [][]RouteConfig{config}

	for i := 0; i < len(configs); i++ {
		for _, item := range configs[i] {
			switch v := item.(type) {
			case []RouteConfig:
				configs = append(configs, v)
			case res.HandlerFunc2:
				handler = v
			case auth.TRole:
				requiredRole = requiredRole | v
				// add at the end so we can OR all the roles that come along
			case Middleware:
				cfg.next = append(cfg.next, v)
			default:
				err := errors.New(strings.Join([]string{"warning: route", method, path, "unrecognized route option type", reflect.TypeOf(item).String()}, " "))
				err = errors2.WithStack(err)
				logger.Printf("%+v", err)
				logger.Println()
			}
		}
	}

	if handler == nil {
		er.Throw("missing type(res.HandlerFunc2) parameter for route config")
	}

	cfg.Add(RequireRole(requiredRole))
	cfg.Handler(handler)
}

func (r *Router) ignoreAuthForRoute(path string) {
	r.auth.AddIgnoreRoute(path)
}

func (r *Router) GithubContinuousDeployment(input res.GithubCDInput) {
	input.Path = strs.Coalesce(input.Path, res.DefaultGithubCdPath)
	r.ignoreAuthForRoute(input.Path)
	r.Router.GithubContinuousDeployment(input)
}

type VersionedHandler struct {
	isDefault bool
	version   string
	value     res.HandlerFunc2
}

func (r *Router) Route(method string, path string) *Configure {
	r.hasRoute = true

	return &Configure{
		callback: func(c *Configure, handler res.HandlerFunc2) {

			for _, cfg := range c.next {
				handler = cfg(r, method, path, handler)
			}

			r.Router.Route(method, path, handler)
		},
	}
}

type Configure struct {
	next     []Middleware
	callback func(c *Configure, handler res.HandlerFunc2)
}

func (c *Configure) Add(next Middleware) *Configure {
	c.next = append(c.next, next)
	return c
}

func (c *Configure) Handler(handler res.HandlerFunc2) {
	c.callback(c, handler)
}

func RequireRole(role auth.TRole) Middleware {
	return func(router *Router, method, path string, next res.HandlerFunc2) res.HandlerFunc2 {
		return router.auth.RequireRole(path, role, next)
	}
}

func DefaultVersion(handler res.HandlerFunc2) VersionedHandler {
	return VersionedHandler{
		version:   "",
		isDefault: true,
		value:     handler,
	}
}

func Version(n string, handler res.HandlerFunc2) VersionedHandler {
	return VersionedHandler{
		version: n,
		value:   handler,
	}
}

func Versioned(versionedHandlers ...VersionedHandler) Middleware {

	return func(router *Router, method string, path string, next res.HandlerFunc2) res.HandlerFunc2 {
		uniq := map[string]bool{}
		var defaultHandler *VersionedHandler

		for _, handler := range versionedHandlers {
			if uniq[handler.version] {
				log.Panicf("Version '%s' already exists for route %s %s", handler.version, method, path)
			}

			uniq[handler.version] = true
			if handler.isDefault {
				defaultHandler = &handler
			}
		}

		return func(rq *res.Request) res.Responder {
			version := rq.APIVersion()
			for _, handler := range versionedHandlers {
				if handler.version == version {
					return handler.value(rq)
				}
			}

			if defaultHandler != nil {
				return defaultHandler.value(rq)
			}

			return res.NotFound("No handler for that api-version")
		}
	}
}

func RateLimit(n int, window time.Duration) Middleware {
	return func(r *Router, method, path string, next res.HandlerFunc2) res.HandlerFunc2 {
		limiter := ratelimit.New(n, window)

		return func(rq *res.Request) res.Responder {

			if err := limiter.Take(); err != nil {
				return res.TooMayRequests()
			}

			return next(rq)
		}
	}
}

// RateLimitErr only uses error responses to take from the rate limit bucket.
// When the rate limit bucket is empty, all requests are blocked with response TooManyRequests
func RateLimitErr(n int, window time.Duration) Middleware {
	return func(r *Router, method, path string, next res.HandlerFunc2) res.HandlerFunc2 {
		limiter := ratelimit.New(n, window)

		return func(rq *res.Request) res.Responder {

			if err := limiter.IsLimited(); err != nil {
				return res.TooMayRequests()
			}

			return &statusCodeSpyResponder{
				next: next(rq),
				onStatusCode: func(value int) {
					if value >= 400 {
						_ = limiter.Take()
					}
				},
			}
		}
	}
}

type statusCodeSpyResponder struct {
	next         res.Responder
	onStatusCode func(value int)
}

func (s *statusCodeSpyResponder) Respond(w http.ResponseWriter, r *http.Request) {
	written := false

	w = &interceptWriter{
		next: w,
		onWrite: func(value []byte) {
			if !written {
				s.onStatusCode(http.StatusOK)
				written = true
			}
		},
		onWriteHeader: func(status int) {
			written = true
			s.onStatusCode(status)
		},
	}

	s.next.Respond(w, r)
}

type interceptWriter struct {
	next          http.ResponseWriter
	onWrite       func(value []byte)
	onWriteHeader func(status int)
}

func (i *interceptWriter) Write(value []byte) (int, error) {
	i.onWrite(value)
	return i.next.Write(value)
}

func (i *interceptWriter) WriteHeader(status int) {
	i.onWriteHeader(status)
	i.next.WriteHeader(status)
}

func (i *interceptWriter) Header() http.Header {
	return i.next.Header()
}
