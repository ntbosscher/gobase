package res

import (
	"github.com/ntbosscher/gobase/env"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"strings"
)

var sourceMapToken = ""

func init() {
	sourceMapToken = env.Optional("REACT_SOURCE_MAP_TOKEN", "")
}

type ReactConfig interface{}

type reactIndexFile func(r *http.Request) string

type reactSmoothTransitionBuildFolder string

func ReactCustomIndexFile(fx func(r *http.Request) string) ReactConfig {
	return reactIndexFile(fx)
}

// ReactSmoothTransitionBuildFolder sets a folder to use as a fallback for
// react's smooth transition. This is useful when you have a react app that
// is actively in-use, but you want to deploy a new version of the app.
//
// When an old file is requested by an active session, the old file can
// still be served.
func ReactSmoothTransitionBuildFolder(value string) ReactConfig {
	return reactSmoothTransitionBuildFolder(value)
}

func ReactApp(dir string, testNodeServerAddr string, cfg ...ReactConfig) http.Handler {

	rr := &reactRouter{
		fileServer:         http.FileServer(http.Dir(dir)),
		staticDir:          dir,
		testNodeServerAddr: testNodeServerAddr,
		indexFile: func(r *http.Request) string {
			return "index.html"
		},
	}

	for _, item := range cfg {
		switch value := item.(type) {
		case reactIndexFile:
			rr.indexFile = value
		case reactSmoothTransitionBuildFolder:
			rr.fallbackStaticDir = string(value)
			rr.fallbackFileServer = http.FileServer(http.Dir(value))
		default:
			log.Println("unknown ReactApp option with type " + reflect.TypeOf(item).String())
		}
	}

	return rr
}

type reactRouter struct {
	fileServer http.Handler
	staticDir  string

	fallbackFileServer http.Handler
	fallbackStaticDir  string

	testNodeServerAddr string
	indexFile          reactIndexFile
}

func (router *reactRouter) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	if env.IsTesting {
		router.serveCreateReactApp(w, r)
		return
	}

	fpath := filepath.Join(router.staticDir, path.Clean(r.URL.Path))
	_, err := os.Stat(fpath)
	is404 := os.IsNotExist(err)
	fileServer := router.fileServer

	// if the file doesn't exist, check the fallback folder
	//
	// This may be used by active sessions that want access to files from
	// the previous release.
	if is404 && router.fallbackStaticDir != "" {
		fpath2 := filepath.Join(router.fallbackStaticDir, path.Clean(r.URL.Path))
		_, err2 := os.Stat(fpath2)
		is404 = os.IsNotExist(err2)

		if !is404 {
			fileServer = router.fallbackFileServer
			fpath = fpath2
			err = nil
		}
	}

	// if it isn't a static file, serve up index.html
	if is404 || r.URL.Path == "/" {
		defaultFile := filepath.Join(router.staticDir, router.indexFile(r))
		NoCacheFunc(w, r)
		http.ServeFile(w, r, defaultFile)
		return
	}

	// based on recommendation from https://facebook.github.io/create-react-app/docs/production-build
	if strings.HasPrefix(r.URL.Path, "/static") {
		w.Header().Set("Cache-Control", "max-age=31536000")
	}

	if strings.HasSuffix(r.URL.Path, ".js.map") {
		if !hasAccessToSourceMaps(r) {
			http.Error(w, "You don't have access to source maps.", http.StatusForbidden)
			return
		}
	}

	fileServer.ServeHTTP(w, r)
}

func hasAccessToSourceMaps(r *http.Request) bool {
	if sourceMapToken == "" {
		return true
	}

	c, err := r.Cookie("react-source-map-token")
	if err == nil {
		if c.Value == sourceMapToken {
			return true
		}
	}

	if r.Header.Get("X-REACT_SOURCE_MAP_TOKEN") == sourceMapToken {
		return true
	}

	return false
}

func (router *reactRouter) serveCreateReactApp(w http.ResponseWriter, r *http.Request) {
	u := &url.URL{}
	*u = *r.URL
	u.Host = router.testNodeServerAddr
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
