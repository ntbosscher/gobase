package res

import (
	"fmt"
	"github.com/ntbosscher/gobase/env"
	"io"
	"log"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"strings"
)

func ReactApp(dir string) http.Handler {
	return &reactRouter{
		fileServer: http.FileServer(http.Dir(dir)),
		staticDir:  dir,
	}
}

type reactRouter struct {
	fileServer http.Handler
	staticDir  string
}

func (router *reactRouter) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	if env.IsTesting {
		router.serveCreateReactApp(w, r)
		return
	}

	// if it isn't a static file, serve up index.html
	if path.Ext(r.URL.Path) == "" {
		defaultFile := filepath.Join(router.staticDir, "index.html")
		http.ServeFile(w, r, defaultFile)
		return
	}

	// based on recommendation from https://facebook.github.io/create-react-app/docs/production-build
	if strings.HasPrefix(r.URL.Path, "/static") {
		w.Header().Set("Cache-Control", "max-age=31536000")
	}

	router.fileServer.ServeHTTP(w, r)
}

func (router *reactRouter) serveCreateReactApp(w http.ResponseWriter, r *http.Request) {
	u := &url.URL{}
	*u = *r.URL
	u.Host = "localhost:3000"
	u.Scheme = "http"

	router.reverseProxy(w, r, u)
}

func (router *reactRouter) reverseProxy(w http.ResponseWriter, r *http.Request, u *url.URL) {

	req, err := http.NewRequest(r.Method, u.String(), r.Body)
	if err != nil {
		log.Println(err)
		w.WriteHeader(400)
		return
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Println(err)
		w.WriteHeader(400)
		return
	}

	defer res.Body.Close()

	if res.StatusCode == 404 && u.Path != "/" {
		fmt.Println("retrying with /")
		u.Path = "/"
		router.reverseProxy(w, r, u)
		return
	}

	// copy headers
	wHeader := w.Header()
	for k, values := range res.Header {
		wHeader.Del(k)

		for _, item := range values {
			wHeader.Add(k, item)
		}
	}

	w.WriteHeader(res.StatusCode)
	_, err = io.Copy(w, res.Body)
	if err != nil {
		log.Println(err)
	}
}
