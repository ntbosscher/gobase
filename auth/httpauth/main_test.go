package httpauth

import (
	"testing"
	"time"

	"github.com/ntbosscher/gobase/auth"
)

func TestAccessToken(t *testing.T) {
	user := &auth.UserInfo{
		TimeZoneOffset: 32,
		Extra: map[string]interface{}{
			"id": 1,
		},
	}

	token, _, err := createAccessToken(user, time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	user2, err := parseJwt(token, auth.TokenTypeAccess)
	if err != nil {
		t.Fatal(err)
	}

	if user2.TokenType != auth.TokenTypeAccess {
		t.Fatal("expected access token type")
	}

	if user2.TimeZoneOffset != user.TimeZoneOffset {
		t.Fatal("mismatched timezone offset")
	}

	if user2.Extra["id"].(float64) != float64(user.Extra["id"].(int)) {
		t.Fatal("mismatched extra.id")
	}
}

// TestAccessTokenRejectedAsRefresh guards finding #4: a stolen access token
// must not be replayable at the refresh endpoint (which expects a refresh token).
func TestAccessTokenRejectedAsRefresh(t *testing.T) {
	user := &auth.UserInfo{Extra: map[string]interface{}{"id": 1}}

	accessToken, _, err := createAccessToken(user, time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := parseJwt(accessToken, auth.TokenTypeRefresh); err == nil {
		t.Fatal("access token was accepted as a refresh token")
	}
}

// TestRefreshTokenRejectedAsAccess is the inverse: a refresh token must not
// authenticate an API request (which expects an access token).
func TestRefreshTokenRejectedAsAccess(t *testing.T) {
	user := &auth.UserInfo{Extra: map[string]interface{}{"id": 1}}

	refreshToken, _, err := createRefreshToken(user, time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := parseJwt(refreshToken, auth.TokenTypeAccess); err == nil {
		t.Fatal("refresh token was accepted as an access token")
	}
}
