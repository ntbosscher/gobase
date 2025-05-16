package httpauth

import (
	"testing"
	"time"

	jwt "github.com/golang-jwt/jwt/v5"
	"github.com/ntbosscher/gobase/auth"
)

func TestAccessToken(t *testing.T) {
	user := &auth.UserInfo{
		TimeZoneOffset: 32,
		StandardClaims: jwt.StandardClaims{},
		Extra: map[string]interface{}{
			"id": 1,
		},
	}

	token, _, err := createAccessToken(user, time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	user2, err := parseJwt(token)
	if err != nil {
		t.Fatal(err)
	}

	if user2.TimeZoneOffset != user.TimeZoneOffset {
		t.Fatal("mismatched timezone offset")
	}

	if user2.Extra["id"].(float64) != float64(user.Extra["id"].(int)) {
		t.Fatal("mismatched extra.id")
	}
}
