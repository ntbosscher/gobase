// Package reactappver watches the http://localhost:80/index.html for changes in value to
// <meta name="app_version" value="xxxxx" /> and reports the current value via CurrentVersion
package reactappver

import (
	"context"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/ntbosscher/gobase/er"
	"github.com/ntbosscher/gobase/strs"
)

var frontendVersionC = make(chan string)
var Logger = log.New(io.Discard, "react-app-ver: ", log.Lshortfile)

type WatcherInput struct {
	Scheme string // defaults to http:
	Host   string // defaults to localhost
	Port   string // defaults to 80
	Path   string // defaults to /index.html

	HTMLMetaName string // defaults to app_version
}

// FileVersionWatcher keeps an eye on index.html for changes to the meta tag
// and improves performance when calling CurrentVersion as calls to CurrentVersion
// will just pull the cached value (updated every 30s).
func FileVersionWatcher(input WatcherInput) {
	defer er.HandleErrors(func(input *er.HandlerInput) {
		log.Println(input)
	})

	<-time.After(10 * time.Second)

	tc := time.NewTicker(30 * time.Second)
	version := getFileVersion(input)
	Logger.Println("watcher started", version)

	for {
		select {
		case <-tc.C:
			v2 := getFileVersion(input)
			if v2 != version {
				Logger.Println("changed", version, "->", v2)
				version = v2
			}
		case frontendVersionC <- version:
		}
	}
}

func CurrentVersion(ctx context.Context) string {
	select {
	case <-ctx.Done():
		return ""
	case v := <-frontendVersionC:
		return v
	}
}

func getFileVersion(input WatcherInput) string {

	scheme := strs.Coalesce(input.Scheme, "http:")
	host := strs.Coalesce(input.Host, "localhost")
	port := strs.Coalesce(input.Port, "80")
	path := strs.Coalesce(input.Path, "/index.html")

	// use network path to allow this to work locally and with built files
	rs, err := http.Get(scheme + "//" + host + ":" + port + path)
	if err != nil {
		Logger.Println(err)
		return ""
	}

	defer rs.Body.Close()

	doc, err := goquery.NewDocumentFromReader(rs.Body)
	if err != nil {
		Logger.Println(err)
		return ""
	}

	metaName := strs.Coalesce(input.HTMLMetaName, "app_version")

	// meta tag with name="version" is added to index.html by webpack
	return doc.Find("meta[name=\""+metaName+"\"]").AttrOr("content", "")
}
