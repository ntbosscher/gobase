package httpauth

import (
	"github.com/ntbosscher/gobase/auth"
	"github.com/ntbosscher/gobase/res"
)

// AuthRouter wraps res.Router, and adds a role-based authentication layer
type AuthRouter struct {
	config *Config
	auth   *server
	next   *res.Router
}

func (a *AuthRouter) AddIgnoreRoute(path string) {
	a.auth.ignoreRoutes = append(a.auth.ignoreRoutes, path)
}

func (a *AuthRouter) ManuallySetSession(rq *res.Request, user *auth.UserInfo) error {
	_, _, err := setupSession(rq, user, a.config)
	return err
}

func (a *AuthRouter) Get(path string, role auth.TRole, handler res.HandlerFunc2) {
	a.next.Get(path, a.RequireRole(path, role, handler))
}

func (a *AuthRouter) Put(path string, role auth.TRole, handler res.HandlerFunc2) {
	a.next.Put(path, a.RequireRole(path, role, handler))
}

func (a *AuthRouter) Post(path string, role auth.TRole, handler res.HandlerFunc2) {
	a.next.Post(path, a.RequireRole(path, role, handler))
}

func (a *AuthRouter) Delete(path string, role auth.TRole, handler res.HandlerFunc2) {
	a.next.Delete(path, a.RequireRole(path, role, handler))
}

func (a *AuthRouter) RequireRole(path string, role auth.TRole, next res.HandlerFunc2) res.HandlerFunc2 {
	if role == auth.Public {
		a.auth.ignoreRoutes = append(a.auth.ignoreRoutes, path)
		return next
	}

	return func(rq *res.Request) res.Responder {
		if auth.HasRole(rq.Context(), role) {
			return next(rq)
		}

		return res.NotAuthorized("Doesn't have required role")
	}
}
