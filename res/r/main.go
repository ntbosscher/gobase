package r

import (
	"github.com/ntbosscher/gobase/auth"
	"github.com/ntbosscher/gobase/auth/httpauth"
	"github.com/ntbosscher/gobase/er"
	"github.com/ntbosscher/gobase/ratelimit"
	"github.com/ntbosscher/gobase/res"
	"github.com/ntbosscher/gobase/strs"
	"log"
	"time"
)

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
		for _, item := range config {
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
