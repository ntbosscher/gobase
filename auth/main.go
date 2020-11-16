package auth

import (
	"context"
	"github.com/dgrijalva/jwt-go"
	"github.com/gobuffalo/nulls"
	"math"
)

type Minutes int

type UserInfo struct {
	TimeZoneOffset Minutes
	jwt.StandardClaims
	UserID    int
	CompanyID int
	Role      TRole
	Extra     map[string]interface{}
}

// TRoles should set only 1 bit to allow for byte wise comparisons
type TRole uint32

const (
	Public  TRole = iota
	RoleAny TRole = math.MaxUint32
)

type userKeyType string

var userKey userKeyType = "user-key"

func Current(ctx context.Context) *UserInfo {
	value := ctx.Value(userKey)
	if value == nil {
		return nil
	}

	return value.(*UserInfo)
}

func IsAuthenticated(ctx context.Context) bool {
	return ctx.Value(userKey) != nil
}

func Company(ctx context.Context) int {
	if !IsAuthenticated(ctx) {
		return -1
	}

	return Current(ctx).CompanyID
}

func UserNull(ctx context.Context) nulls.Int {
	if !IsAuthenticated(ctx) {
		return nulls.Int{}
	}

	return nulls.Int{
		Valid: true,
		Int:   User(ctx),
	}
}

func User(ctx context.Context) int {
	if !IsAuthenticated(ctx) {
		return -1
	}

	return Current(ctx).UserID
}

func Role(ctx context.Context) TRole {
	if !IsAuthenticated(ctx) {
		return 0
	}

	return Current(ctx).Role
}

func HasRole(ctx context.Context, role TRole) bool {
	if !IsAuthenticated(ctx) {
		return role&Public != 0
	}

	return (Role(ctx) & role) != 0
}

func SetUser(ctx context.Context, user *UserInfo) context.Context {
	return context.WithValue(ctx, userKey, user)
}
