package httpversion

import "net/http"

var VersionHeaderName = "X-APIVersion"

func Parse(r *http.Request) string {
	return r.Header.Get(VersionHeaderName)
}
