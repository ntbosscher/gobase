package oauth

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/sessions"
	"github.com/markbates/goth"
	"github.com/markbates/goth/gothic"
	"github.com/ntbosscher/gobase/auth"
	"github.com/ntbosscher/gobase/env"
	"github.com/ntbosscher/gobase/res"
)

type User = goth.User

type Config struct {
	// The https://{domain}/{callbackPath} to send oauth successes to to setup user sessions
	Domain string

	// Path where oauth process can be initiated
	// default: /api/auth
	//
	// redirects to this endpoint should include the ?provider={provider_name} parameter to determine
	// which type of oauth to initiate
	SetupPath string

	// Path where successful oauth results redirect to setup the user session
	// default: /api/auth/callback
	CallbackPath string

	OnAuthenticationFailure func(err error) res.Responder

	// Called after oauth is successful to validate user info before setting up user session
	// If *auth.UserInfo is nil, will assume user is invalid and not setup user session
	// Regardless or auth.UserInfo, response will be returned to the http request
	OnUserAuthenticated func(ctx context.Context, user *User) (info *auth.UserInfo, response res.Responder)

	// OAuth providers to use.
	//
	// CallbackURL parameter to setup a provider should be CallbackPath + "?provider={provider_name}"
	// e.g. google.New("key", "secret", "https://my-app.com/api/auth/callback?provider=google", "scopes..")
	Providers []goth.Provider
}

type setupSessionFunc func(rq *res.Request, user *auth.UserInfo) error

func Setup(router *res.Router, config *Config, setupSession setupSessionFunc) {

	goth.UseProviders(config.Providers...)

	key := env.Optional("OAUTH_SESSION_SECRET", "")
	if key == "" {
		log.Fatal("OAUTH_SESSION_SECRET must be set. If you don't have one, you can generate one with `go run github.com/ntbosscher/gobase/auth/httpauth/oauthgen -len 64`")
	}

	maxAge := 24 * time.Hour

	store := sessions.NewCookieStore([]byte(key))
	store.MaxAge(int(maxAge.Seconds()))
	store.Options.Path = "/"
	store.Options.HttpOnly = true
	store.Options.Secure = !env.IsTesting

	gothic.Store = store

	router.Get(d(config.CallbackPath, "/api/auth/callback"), onCallback(config, setupSession))
	router.Get(d(config.SetupPath, "/api/auth"), onSetup())
}

func d(value string, defaultValue string) string {
	if value != "" {
		return value
	}

	return defaultValue
}

func onSetup() res.HandlerFunc2 {
	return func(rq *res.Request) res.Responder {
		return res.Func(func(w http.ResponseWriter) {
			gothic.BeginAuthHandler(w, rq.Request())
		})
	}
}

func onCallback(config *Config, setupSession setupSessionFunc) res.HandlerFunc2 {
	return func(rq *res.Request) res.Responder {
		user, err := gothic.CompleteUserAuth(rq.Writer(), rq.Request())
		if err != nil {
			return config.OnAuthenticationFailure(err)
		}

		info, responder := config.OnUserAuthenticated(rq.Context(), &user)
		if info == nil {
			return responder
		}

		if err := setupSession(rq, info); err != nil {
			return config.OnAuthenticationFailure(err)
		}

		return responder
	}
}
