package httpauth

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/golang-jwt/jwt"
	"github.com/ntbosscher/gobase/auth"
	"github.com/ntbosscher/gobase/auth/httpauth/oauth"
	"github.com/ntbosscher/gobase/env"
	"github.com/ntbosscher/gobase/res"
	"github.com/ntbosscher/gobase/strs"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"time"
)

// jwtKey setup during first call to Setup()
var jwtKey []byte

var IsVerbose bool

type ActiveUserValidator func(ctx context.Context, user *auth.UserInfo) error
type APITokenChecker func(ctx context.Context, token string) (*auth.UserInfo, error)

type Config struct {

	// Domain defines the domain setting for authorization cookies
	// default: "" (current domain only)
	Domain string

	// SecureCookies controls whether or not to set the secure flag on http cookies
	// default: !env.IsTesting
	SecureCookies *bool

	// PartitionedCookies controls whether or not to set the partitioned flag on http cookies (CHIPS)
	PartitionedCookies bool

	// SameSite: wether or not to restrict cookies to SameSite
	// default: http default
	SameSite http.SameSite

	// Optional oauth config
	OAuth *oauth.Config

	// Checks user credentials on login
	CredentialChecker CredentialChecker

	// APITokenChecker checks bearer tokens on requests for API access
	APITokenChecker APITokenChecker

	// ValidateActiveUser allows you to do extra db checks when
	// we do a access token refresh
	// e.g. check if the user has been archived.
	//
	// If ValidateActiveUser returns an error, we'll assume the user is no longer valid
	// and force them to re-login
	//
	// if not set, this check will be ignored
	ValidateActiveUser ActiveUserValidator

	// POST route that will accept login requests
	// default: /api/auth/login
	LoginPath string

	// route that will accept logout requests
	// default: /api/auth/logout
	LogoutPath string

	// route/url to redirect logout requests to after they've been logged out
	// default: /
	LogoutRedirectTo string

	// POST route that will accept register requests
	// default: /api/auth/register
	RegisterPath string

	// Handler for registration requests. If a non-nil auth.UserInfo is returned
	// httpauth will setup the user session
	// if nil, register feature will be disabled
	RegisterHandler func(rq *res.Request) (*auth.UserInfo, res.Responder)

	// POST route that will accept jwt refresh requests
	// default: /api/auth/refresh
	RefreshPath string

	// default: 30 min
	AccessTokenLifeTime time.Duration

	// default: 30 days
	RefreshTokenLifeTime time.Duration

	// default: token
	AccessTokenCookieName string

	// default: refresh-token
	RefreshTokenCookieName string

	// route prefixes that don't require authentication
	PublicRoutePrefixes []string

	// exact match request paths that don't require authentication
	IgnoreRoutes []string

	// filters each request after authentication has been checked
	// default: nil
	PerRequestFilter PerRequestFilter

	NotAuthorizedResponder func(rq *res.Request, err error) res.Responder
}

type PerRequestFilter func(ctx context.Context, r *http.Request, user *auth.UserInfo) error

func (c Config) getRefreshTokenCookieName() string {
	return strs.Coalesce(c.RefreshTokenCookieName, "refresh-token")
}

func (c Config) getAccessTokenCookieName() string {
	return strs.Coalesce(c.AccessTokenCookieName, "token")
}

const defaultLoginEndpoint = "/api/auth/login"
const defaultRefreshEndpoint = "/api/auth/refresh"
const defaultLogoutEndpoint = "/api/auth/logout"
const defaultRegisterEndpoint = "/api/auth/register"

func Setup(router *res.Router, config Config) *AuthRouter {

	if jwtKey == nil {
		var err error
		if jwtKey, err = ioutil.ReadFile("./.jwtkey"); err != nil {
			log.Println("./.jwtkey should contain 2048 random bytes. Run `go run github.com/ntbosscher/gobase/auth/httpauth/jwtgen` to automatically generate one")
			log.Fatal("failed to read required file ./.jwtkey: " + err.Error())
		}
	}

	if config.SecureCookies == nil {
		secureDefault := !env.IsTesting
		config.SecureCookies = &secureDefault
	}

	loginPath := strs.Coalesce(config.LoginPath, defaultLoginEndpoint)
	router.Post(loginPath, loginHandler(&config))
	logoutPath := strs.Coalesce(config.LogoutPath, defaultLogoutEndpoint)
	router.Post(logoutPath, logoutHandler(&config))
	router.Get(logoutPath, logoutHandler(&config))
	refreshPath := strs.Coalesce(config.RefreshPath, defaultRefreshEndpoint)
	router.Post(refreshPath, refreshHandler(&config))

	if env.IsTesting && config.Domain != "" {
		log.Println("httpauth: removing .Domain in testing mode")
		config.Domain = ""
	}

	if config.RegisterHandler != nil {
		router.Post(strs.Coalesce(config.RegisterPath, defaultRegisterEndpoint), registerHandler(&config))
	}

	config.IgnoreRoutes = append(config.IgnoreRoutes, loginPath, logoutPath, refreshPath)

	if config.OAuth != nil {
		config.IgnoreRoutes = append(config.IgnoreRoutes, config.OAuth.CallbackPath)
	}

	sessionSetter := func(rq *res.Request, user *auth.UserInfo) error {
		_, err := setupSession(rq, user, &config)
		return err
	}

	if config.OAuth != nil {
		oauth.Setup(router, config.OAuth, sessionSetter)
	}

	server := newServer(config)

	router.Use(func(handler http.Handler) http.Handler {
		return cloneServer(server, handler)
	})

	return &AuthRouter{
		config: &config,
		auth:   server,
		next:   router,
	}
}

func cloneServer(src *server, next http.Handler) *server {
	clone := &server{}
	*clone = *src
	clone.next = next
	return clone
}

func newServer(config Config) *server {

	if config.CredentialChecker == nil {
		log.Fatal("github.com/ntbosscher/gobase/auth/authhttp.Middleware(config): config requires CredentialChecker")
	}

	return &server{
		perRequestFilter:         config.PerRequestFilter,
		ignoreRoutesWithPrefixes: config.PublicRoutePrefixes,
		ignoreRoutes:             config.IgnoreRoutes,
		authHandler:              authHandler(&config),
	}
}

type server struct {
	next                     http.Handler
	perRequestFilter         PerRequestFilter
	ignoreRoutesWithPrefixes []string
	ignoreRoutes             []string
	authHandler              func(request *res.Request) (res.Responder, context.Context)
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	ignoredRoute := false

	for _, path := range s.ignoreRoutes {
		if r.URL.Path == path {
			ignoredRoute = true
			break
		}
	}

	if !ignoredRoute {
		for _, prefix := range s.ignoreRoutesWithPrefixes {
			if strings.HasPrefix(r.URL.Path, prefix) {
				ignoredRoute = true
				break
			}
		}
	}

	err, ctx := s.authHandler(res.NewRequest(w, r))
	if err != nil {

		// attempt to authenticate, but ignore errors
		if ignoredRoute {
			s.next.ServeHTTP(w, r)
			return
		}

		err.Respond(w, r)
		return
	}

	r = r.WithContext(ctx)

	if !ignoredRoute && s.perRequestFilter != nil {
		if err := s.perRequestFilter(ctx, r, auth.Current(ctx)); err != nil {
			notAuthenticated.Respond(w, r)
			return
		}
	}

	s.next.ServeHTTP(w, r)
}

func logVerbose(err error) {
	if IsVerbose {
		log.Println(err)
	}
}

var notAuthenticated = res.NotAuthorized()

func registerHandler(config *Config) func(rq *res.Request) res.Responder {
	return func(rq *res.Request) res.Responder {
		info, response := config.RegisterHandler(rq)
		if info != nil {
			_, err := setupSession(rq, info, config)
			logVerbose(err)
		}

		return response
	}
}

func authHandler(config *Config) func(rq *res.Request) (res.Responder, context.Context) {
	notAuthenticatedResponder := config.NotAuthorizedResponder
	if notAuthenticatedResponder == nil {
		notAuthenticatedResponder = func(rq *res.Request, err error) res.Responder {
			if err != nil {
				return res.NotAuthorized(err.Error())
			}

			return res.NotAuthorized()
		}
	}

	return func(rq *res.Request) (res.Responder, context.Context) {
		tokenString := cookieOrBearerToken(rq, config.getAccessTokenCookieName())
		if tokenString == "" {
			return notAuthenticatedResponder(rq, nil), nil
		}

		user, err := parseJwt(tokenString)
		if err != nil {
			if config.APITokenChecker != nil {
				user, err = config.APITokenChecker(rq.Context(), tokenString)
			}

			if err != nil {
				return notAuthenticatedResponder(rq, err), nil
			}
		}

		ctx := auth.SetUser(rq.Context(), user)
		return nil, ctx
	}
}

func parseJwt(tokenString string) (*auth.UserInfo, error) {
	user := &auth.UserInfo{}
	token, err := jwt.ParseWithClaims(tokenString, user, func(token *jwt.Token) (interface{}, error) {
		return jwtKey, nil
	})

	if err != nil {
		return nil, err
	}

	if !token.Valid {
		return nil, errors.New("invalid token")
	}

	return user, nil
}

func cookieOrBearerToken(rq *res.Request, name string) string {
	if value := rq.Cookie(name); value != "" {
		return value
	}

	bearerToken := rq.Request().Header.Get("Authorization")
	return strings.TrimPrefix(bearerToken, "Bearer ")
}

func refreshHandler(config *Config) res.HandlerFunc2 {
	return func(rq *res.Request) res.Responder {

		refreshToken := cookieOrBearerToken(rq, config.getRefreshTokenCookieName())
		if refreshToken == "" {
			return res.BadRequest("Invalid refresh token")
		}

		claims, err := parseJwt(refreshToken)
		if err != nil {
			return res.AppError("Access denied: " + err.Error())
		}

		if config.ValidateActiveUser != nil {
			if err := config.ValidateActiveUser(rq.Context(), claims); err != nil {
				// logout the user if using cookie authentication rather than redirecting to the logout page
				// b/c some applications will redirect the user to the landing page when hitting the logout
				// endpoint. This isn't desirable for api-access types.
				removeCookies(rq, config)

				return res.AppError("Access denied: " + err.Error())
			}
		}

		accessToken, accessTokenExpiry, err := createAccessToken(claims, config.AccessTokenLifeTime)
		if err != nil {
			return res.AppError("Failed to create access token: " + err.Error())
		}

		setCookie(rq.Writer(), config.PartitionedCookies, &http.Cookie{
			Secure:   *config.SecureCookies,
			Name:     config.getAccessTokenCookieName(),
			Value:    accessToken,
			Expires:  accessTokenExpiry,
			Path:     "/",
			SameSite: config.SameSite,
			Domain:   config.Domain,
		})

		return res.Ok(map[string]interface{}{
			"accessToken": accessToken,
		})
	}
}

func setCookie(wr http.ResponseWriter, partitioned bool, cookie *http.Cookie) {
	if !partitioned {
		http.SetCookie(wr, cookie)
		return
	}

	// todo: temporary fix for partitioned cookies, waiting for golang1.23 to drop with partitioned attribute officially supported
	// http.SetCookie(rq.Writer(), cookie)

	rawCookie := cookie.String()
	if rawCookie != "" {
		if !strings.HasSuffix(rawCookie, ";") {
			rawCookie += ";"
		}

		rawCookie += " Partitioned"

		wr.Header().Add("Set-Cookie", rawCookie)
	}
}

type SessionInfo struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
}

func setupSession(rq *res.Request, user *auth.UserInfo, config *Config) (info *SessionInfo, err error) {
	accessToken, accessTokenExpiry, err := createAccessToken(user, config.AccessTokenLifeTime)
	if err != nil {
		err = errors.New("Failed to create access token: " + err.Error())
		return
	}

	refreshToken, refreshTokenExpiry, err := createRefreshToken(user, config.RefreshTokenLifeTime)
	if err != nil {
		err = errors.New("Failed to create refresh token: " + err.Error())
		return
	}

	setCookie(rq.Writer(), config.PartitionedCookies, &http.Cookie{
		Secure:   *config.SecureCookies,
		Name:     config.getAccessTokenCookieName(),
		Value:    accessToken,
		Expires:  accessTokenExpiry,
		Path:     "/",
		SameSite: config.SameSite,
		Domain:   config.Domain,
	})

	setCookie(rq.Writer(), config.PartitionedCookies, &http.Cookie{
		Secure:   *config.SecureCookies,
		HttpOnly: true,
		Name:     config.getRefreshTokenCookieName(),
		Value:    refreshToken,
		Expires:  refreshTokenExpiry,
		Path:     "/",
		SameSite: config.SameSite,
		Domain:   config.Domain,
	})

	info = &SessionInfo{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}

	return
}

func loginHandler(config *Config) res.HandlerFunc2 {
	return func(rq *res.Request) res.Responder {

		creds := &Credential{}

		jsonBytes, err := ioutil.ReadAll(rq.Request().Body)
		if err != nil {
			return res.BadRequest(err.Error())
		}

		if err := json.Unmarshal(jsonBytes, creds); err != nil {
			return res.BadRequest(err.Error())
		}

		creds.Raw = jsonBytes

		user, err := config.CredentialChecker(rq.Context(), creds)
		if err != nil {
			return res.AppError(err.Error())
		}

		info, err := setupSession(rq, user, config)
		if err != nil {
			return res.Error(err)
		}

		return res.Ok(info)
	}
}

type Credential struct {
	Username string
	Password string

	Raw json.RawMessage
}

type CredentialChecker = func(context.Context, *Credential) (*auth.UserInfo, error)

func createRefreshToken(user *auth.UserInfo, lifetime time.Duration) (token string, expiry time.Time, err error) {

	if lifetime == 0 {
		expiry = time.Now().AddDate(0, 0, 30)
	} else {
		expiry = time.Now().Add(lifetime)
	}

	user.StandardClaims.ExpiresAt = expiry.Unix()

	tokenObj := jwt.NewWithClaims(jwt.SigningMethodHS256, user)
	token, err = tokenObj.SignedString(jwtKey)
	return
}

type Claims struct {
}

func createAccessToken(user *auth.UserInfo, lifetime time.Duration) (token string, expiry time.Time, err error) {

	if lifetime == 0 {
		expiry = time.Now().Add(30 * time.Minute)
	} else {
		expiry = time.Now().Add(lifetime)
	}

	user.StandardClaims.ExpiresAt = expiry.Unix()

	tokenObj := jwt.NewWithClaims(jwt.SigningMethodHS256, user)
	token, err = tokenObj.SignedString(jwtKey)
	return
}

func removeCookies(rq *res.Request, config *Config) {
	access := &http.Cookie{
		Secure:   *config.SecureCookies,
		Name:     config.getAccessTokenCookieName(),
		MaxAge:   -1,
		Path:     "/",
		SameSite: config.SameSite,
		Domain:   config.Domain,
	}

	// remove both cookie types to prevent collisions during migration
	setCookie(rq.Writer(), true, access)
	setCookie(rq.Writer(), false, access)

	refresh := &http.Cookie{
		Secure:   *config.SecureCookies,
		Name:     config.getRefreshTokenCookieName(),
		MaxAge:   -1,
		Path:     "/",
		SameSite: config.SameSite,
		Domain:   config.Domain,
	}

	setCookie(rq.Writer(), true, refresh)
	setCookie(rq.Writer(), false, refresh)
}

func logoutHandler(config *Config) res.HandlerFunc2 {
	return func(rq *res.Request) res.Responder {

		removeCookies(rq, config)

		if config.LogoutRedirectTo == "" {
			return res.Redirect("/")
		}

		return res.Redirect(config.LogoutRedirectTo)
	}
}
