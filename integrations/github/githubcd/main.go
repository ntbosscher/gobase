package githubcd

import (
	"bytes"
	"github.com/ntbosscher/gobase/integrations/github/githubcd/githubcdutil"
	"log"
	"net/http"
	"os"
	"os/exec"
)

var Verbose = false
var logger = log.New(os.Stderr, "gobase-githubcd: ", log.Ldate|log.Ltime)

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
	_, err := githubcdutil.Verify(h.secret, r)
	return err
}

func (h *Handler) doUpdate() error {

	if Verbose {
		logger.Println("git", "pull", "-f")
	}

	cmd := exec.Command("git", "pull", "-f")
	output, err := cmd.CombinedOutput()

	if err != nil {
		log.Println(string(output))
		return err
	}

	if Verbose {
		logger.Println(string(output))
	}

	if bytes.Contains(output, []byte("Already up to date.")) {
		logger.Println("No changes, skipping deployment script")
		return nil
	}

	// run async so we can return the http request before this process get's killed
	go func() {
		if Verbose {
			logger.Println(h.postPullScript)
		}

		cmd := exec.Command(h.postPullScript)

		output, err := cmd.CombinedOutput()
		if err != nil {
			log.Println("github continuous deployment post-pull script failed")
			log.Println(string(output))
		} else if Verbose {
			logger.Println(h.postPullScript)
			logger.Println(string(output))
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
