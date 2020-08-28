package httpauth

import (
	"context"
	"errors"
	"fmt"
	"github.com/dgrijalva/jwt-go"
	"github.com/ntbosscher/gobase/auth"
	"github.com/ntbosscher/gobase/env"
	"github.com/ntbosscher/gobase/res"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"time"
)

var jwtKey []byte

func init() {
	var err error
	jwtKey, err = ioutil.ReadFile("./.jwtkey")
	if err != nil {
		log.Println("./.jwtkey should contain 2048 random bytes. Run `go run github.com/ntbosscher/gobase/auth/httpauth/jwtgen` to automatically generate one")
		log.Fatal("failed to read required file ./.jwtkey: " + err.Error())
	}
}

type ActiveUserValidator func(ctx context.Context, user *auth.UserInfo) error

type Config struct {
	// Checks user credentials on login
	CredentialChecker CredentialChecker

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

	// filters each request after authentication has been checked
	// default: nil
	PerRequestFilter PerRequestFilter
}

type PerRequestFilter func(ctx context.Context, r *http.Request, user *auth.UserInfo) error

func (c Config) getRefreshTokenCookieName() string {
	return useDefault(c.RefreshTokenCookieName, "refresh-token")
}

func (c Config) getAccessTokenCookieName() string {
	return useDefault(c.AccessTokenCookieName, "token")
}

func Middleware(config Config) func(http.Handler) http.Handler {

	if config.CredentialChecker == nil {
		log.Fatal("github.com/ntbosscher/gobase/auth/authhttp.Middleware(config): config requires CredentialChecker")
	}

	return func(h http.Handler) http.Handler {
		return &server{
			perRequestFilter:         config.PerRequestFilter,
			next:                     h,
			loginEndpoint:            useDefault(config.LoginPath, defaultLoginEndpoint),
			loginHandler:             res.WrapHTTPFunc(loginHandler(&config)),
			refreshEndpoint:          useDefault(config.RefreshPath, defaultRefreshEndpoint),
			refreshHandler:           res.WrapHTTPFunc(refreshHandler(&config)),
			logoutEndpoint:           useDefault(config.LogoutPath, defaultLogoutEndpoint),
			logoutHandler:            res.WrapHTTPFunc(logoutHandler(&config)),
			ignoreRoutesWithPrefixes: config.PublicRoutePrefixes,
			authHandler:              authHandler(&config),
		}
	}
}

func useDefault(str string, defaultStr string) string {
	if str == "" {
		return defaultStr
	}

	return str
}

type server struct {
	next                     http.Handler
	perRequestFilter         PerRequestFilter
	loginEndpoint            string
	loginHandler             http.HandlerFunc
	refreshEndpoint          string
	refreshHandler           http.HandlerFunc
	logoutEndpoint           string
	logoutHandler            http.HandlerFunc
	ignoreRoutesWithPrefixes []string
	authHandler              func(request *res.Request) (res.Responder, context.Context)
}

var defaultLoginEndpoint = "/api/auth/login"
var defaultRefreshEndpoint = "/api/auth/refresh"
var defaultLogoutEndpoint = "/api/auth/logout"

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" && r.URL.Path == s.loginEndpoint {
		s.loginHandler.ServeHTTP(w, r)
		return
	}

	if r.URL.Path == s.logoutEndpoint {
		s.logoutHandler.ServeHTTP(w, r)
		return
	}

	for _, prefix := range s.ignoreRoutesWithPrefixes {
		if strings.HasPrefix(r.URL.Path, prefix) {
			s.next.ServeHTTP(w, r)
			return
		}
	}

	err, ctx := s.authHandler(res.NewRequest(w, r))
	if err != nil {
		err.Respond(w, r)
		return
	}

	r = r.WithContext(ctx)

	if s.perRequestFilter != nil {
		if err := s.perRequestFilter(ctx, r, auth.Current(ctx)); err != nil {
			notAuthenticated.Respond(w, r)
			return
		}
	}

	s.next.ServeHTTP(w, r)
}

var notAuthenticated = res.AppError("not authenticated")

func authHandler(config *Config) func(rq *res.Request) (res.Responder, context.Context) {
	return func(rq *res.Request) (res.Responder, context.Context) {
		tokenString := cookieOrBearerToken(rq, config.getAccessTokenCookieName())
		if tokenString == "" {
			return notAuthenticated, nil
		}

		user, err := parseJwt(tokenString)
		if err != nil {
			return res.AppError(fmt.Sprint("not authenticated: ", err)), nil
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
				return res.Redirect(config.LogoutPath)
			}
		}

		accessToken, accessTokenExpiry, err := createAccessToken(claims, config.AccessTokenLifeTime)
		if err != nil {
			return res.AppError("Failed to create access token: " + err.Error())
		}

		http.SetCookie(rq.Writer(), &http.Cookie{
			Secure:  !env.IsTesting,
			Name:    config.getAccessTokenCookieName(),
			Value:   accessToken,
			Expires: accessTokenExpiry,
			Path:    "/",
		})

		return res.Ok(map[string]interface{}{
			"accessToken": accessToken,
		})
	}
}

func loginHandler(config *Config) res.HandlerFunc2 {
	return func(rq *res.Request) res.Responder {
		creds := &Credential{}
		if err := rq.ParseJSON(creds); err != nil {
			return res.BadRequest(err.Error())
		}

		user, err := config.CredentialChecker(rq.Context(), creds)
		if err != nil {
			return res.AppError(err.Error())
		}

		accessToken, accessTokenExpiry, err := createAccessToken(user, config.AccessTokenLifeTime)
		if err != nil {
			return res.AppError("Failed to create access token: " + err.Error())
		}

		refreshToken, refreshTokenExpiry, err := createRefreshToken(user, config.RefreshTokenLifeTime)
		if err != nil {
			return res.AppError("Failed to create refresh token: " + err.Error())
		}

		http.SetCookie(rq.Writer(), &http.Cookie{
			Secure:  !env.IsTesting,
			Name:    config.getAccessTokenCookieName(),
			Value:   accessToken,
			Expires: accessTokenExpiry,
			Path:    "/",
		})

		http.SetCookie(rq.Writer(), &http.Cookie{
			Secure:   !env.IsTesting,
			HttpOnly: true,
			Name:     config.getRefreshTokenCookieName(),
			Value:    refreshToken,
			Expires:  refreshTokenExpiry,
			Path:     "/",
		})

		return res.Ok(map[string]interface{}{
			"accessToken":  accessToken,
			"refreshToken": refreshToken,
		})
	}
}

type Credential struct {
	Username string
	Password string
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

func logoutHandler(config *Config) res.HandlerFunc2 {
	return func(rq *res.Request) res.Responder {

		http.SetCookie(rq.Writer(), &http.Cookie{
			Secure: !env.IsTesting,
			Name:   config.getAccessTokenCookieName(),
			MaxAge: -1,
			Path:   "/",
		})

		http.SetCookie(rq.Writer(), &http.Cookie{
			Secure: !env.IsTesting,
			Name:   config.getRefreshTokenCookieName(),
			MaxAge: -1,
			Path:   "/",
		})

		return res.Redirect("/")
	}
}
