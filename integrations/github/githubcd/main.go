package githubcd

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"github.com/ntbosscher/gobase/er"
	"io/ioutil"
	"log"
	"net/http"
	"os/exec"
	"strings"
)

type Handler struct {
	secret         string
	postPullScript string
}

func New(secret string, postPullScript string) *Handler {
	return &Handler{
		secret:         secret,
		postPullScript: postPullScript,
	}
}

func (h *Handler) validateSignature(r *http.Request) error {

	sig := r.Header.Get("x-hub-signature-256")
	sig = strings.TrimPrefix(sig, "sha256=")

	hasher := hmac.New(sha256.New, []byte(h.secret))

	request, err := ioutil.ReadAll(r.Body)
	er.Check(err)

	_, err = hasher.Write(request)
	er.Check(err)

	value := hex.EncodeToString(hasher.Sum(nil))

	if value != sig {
		return errors.New("invalid hash")
	}

	return nil
}

func (h *Handler) doUpdate() error {
	cmd := exec.Command("git", "pull", "-f")
	if output, err := cmd.CombinedOutput(); err != nil {
		log.Println(string(output))
		return err
	}

	// run async in case postPullScript triggers a process re-start
	go func() {
		cmd = exec.Command(h.postPullScript)
		if output, err := cmd.CombinedOutput(); err != nil {
			log.Println("github continuous deployment post-pull script failed")
			log.Println(string(output))
		}
	}()

	return nil
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := h.validateSignature(r); err != nil {
		w.WriteHeader(403)
		w.Write([]byte(err.Error()))
		return
	}

	if err := h.doUpdate(); err != nil {
		w.WriteHeader(500)
		w.Write([]byte(err.Error()))
		return
	}

	w.WriteHeader(200)
	w.Write([]byte("Accepted"))
}
