package r

import (
	"github.com/ntbosscher/gobase/auth"
	"github.com/ntbosscher/gobase/auth/httpauth"
	"github.com/ntbosscher/gobase/ratelimit"
	"github.com/ntbosscher/gobase/res"
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

type config func(router *Router, method string, path string, handler res.HandlerFunc2) res.HandlerFunc2

type WithHandler struct {
	callback func(handler res.HandlerFunc2)
}

func (w *WithHandler) WithHandler(handler res.HandlerFunc2) {
	w.callback(handler)
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

type RouteConfig struct {

	// default: Public
	RequireRole auth.TRole

	// default: none
	RateLimit *RateLimitConfig

	Handler res.HandlerFunc2
}

type RateLimitConfig struct {
	Count  int
	Window time.Duration
}

func (r *Router) Add(method string, path string, input RouteConfig) {

	config := r.Route(method, path).
		RequireRole(input.RequireRole)

	if input.RateLimit != nil {
		config = config.RateLimit(input.RateLimit.Count, input.RateLimit.Window)
	}

	config.Handler(input.Handler)
}

func (r *Router) AddPublic(method string, path string, handler res.HandlerFunc2) {
	r.Route(method, path).RequireRole(auth.Public).Handler(handler)
}

type RoleRouter struct {
	role   auth.TRole
	parent *Router
}

func (r *RoleRouter) Add(method string, path string, input RouteConfig) {
	input.RequireRole = r.role
	r.parent.Add(method, path, input)
}

func (r *RoleRouter) AddSimple(method string, path string, handler res.HandlerFunc2) {
	r.parent.Add(method, path, RouteConfig{
		RequireRole: r.role,
		Handler:     handler,
	})
}

func (r *Router) WithRole(role auth.TRole, callback func(r *RoleRouter)) {
	router := &RoleRouter{role: role, parent: r}
	callback(router)
}

func (r *Router) Post(path string) *Configure {
	return r.Route("POST", path)
}

func (r *Router) Put(path string) *Configure {
	return r.Route("PUT", path)
}

func (r *Router) Delete(path string) *Configure {
	return r.Route("DELETE", path)
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
	next     []config
	callback func(c *Configure, handler res.HandlerFunc2)
}

func (c *Configure) Add(next config) *Configure {
	c.next = append(c.next, next)
	return c
}

func (c *Configure) Handler(handler res.HandlerFunc2) {
	c.callback(c, handler)
}

func (c *Configure) RateLimit(count int, window time.Duration) *Configure {
	limiter := ratelimit.New(count, window)

	return c.Add(func(r *Router, method, path string, next res.HandlerFunc2) res.HandlerFunc2 {
		return func(rq *res.Request) res.Responder {

			if err := limiter.Take(); err != nil {
				return res.TooMayRequests()
			}

			return next(rq)
		}
	})
}

func (c *Configure) IsPublic() *Configure {
	return c.Add(func(router *Router, method, path string, next res.HandlerFunc2) res.HandlerFunc2 {
		return router.auth.RequireRole(path, auth.Public, next)
	})
}

func (c *Configure) RequireRole(role auth.TRole) *Configure {
	return c.Add(func(router *Router, method, path string, next res.HandlerFunc2) res.HandlerFunc2 {
		return router.auth.RequireRole(path, role, next)
	})
}
