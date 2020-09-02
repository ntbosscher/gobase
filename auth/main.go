package auth

import (
	"context"
	"github.com/dgrijalva/jwt-go"
)

type Minutes int

type UserInfo struct {
	TimeZoneOffset Minutes
	jwt.StandardClaims
	UserID    int
	CompanyID int
	Extra     map[string]interface{}
}

type userKeyType string

var userKey userKeyType = "user-key"

func Current(ctx context.Context) *UserInfo {
	return ctx.Value(userKey).(*UserInfo)
}

func IsAuthenticated(ctx context.Context) bool {
	return ctx.Value(userKey) != nil
}

func Company(ctx context.Context) int {
	return Current(ctx).CompanyID
}

func User(ctx context.Context) int {
	return Current(ctx).UserID
}

func SetUser(ctx context.Context, user *UserInfo) context.Context {
	return context.WithValue(ctx, userKey, user)
}
