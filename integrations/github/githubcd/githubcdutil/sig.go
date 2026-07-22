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

func hmacSum(secret string, body []byte) []byte {
	hasher := hmac.New(sha256.New, []byte(secret))
	// hash.Hash.Write never returns an error, so it's safe to ignore.
	hasher.Write(body)
	return hasher.Sum(nil)
}

func calcSignature(secret string, body []byte) (string, error) {
	return hex.EncodeToString(hmacSum(secret, body)), nil
}

func Verify(secret string, r *http.Request) ([]byte, error) {

	sig := r.Header.Get("x-hub-signature-256")
	sig = strings.TrimPrefix(sig, "sha256=")

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}

	// Decode the client-supplied signature to raw bytes and compare with a
	// constant-time HMAC check. A plain `!=` on the hex strings leaks timing
	// that lets an attacker recover a valid signature byte-by-byte, and this
	// signature gates a code-execution/deploy path — so it must be constant
	// time. hmac.Equal also handles length mismatches safely.
	expected := hmacSum(secret, body)
	got, err := hex.DecodeString(sig)
	if err != nil || !hmac.Equal(got, expected) {
		return nil, errors.New("invalid hash")
	}

	return body, nil
}
