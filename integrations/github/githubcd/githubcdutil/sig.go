package githubcdutil

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/ioutil"
	"net/http"
	"strings"
)

func SignAndSetHeader(rq *http.Request, secret string, body []byte) error {
	sig, err := calcSignature(secret, body)
	if err != nil {
		return err
	}

	rq.Header.Set("x-hub-signature-256", "sha256="+sig)
	return err
}

func calcSignature(secret string, body []byte) (string, error) {
	hasher := hmac.New(sha256.New, []byte(secret))

	if _, err := hasher.Write(body); err != nil {
		return "", err
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func Verify(secret string, r *http.Request) ([]byte, error) {

	sig := r.Header.Get("x-hub-signature-256")
	sig = strings.TrimPrefix(sig, "sha256=")

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}

	value, err := calcSignature(secret, body)
	if err != nil {
		return nil, err
	}

	if value != sig {
		return nil, errors.New("invalid hash")
	}

	return body, nil
}
