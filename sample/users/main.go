package users

import (
	"context"
	"errors"
	"github.com/ntbosscher/gobase/auth"
	"github.com/ntbosscher/gobase/auth/httpauth"
	"github.com/ntbosscher/gobase/res"
)

func CredentialChecker(context.Context, *httpauth.Credential) (*auth.UserInfo, error) {
	return nil, errors.New("todo")
}

func RegisterHandler(rq *res.Request) (*auth.UserInfo, res.Responder) {
	return nil, nil
}
