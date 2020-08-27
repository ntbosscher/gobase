package res

import (
	"context"
	"fmt"
	"github.com/gorilla/mux"
	"log"
	"mime/multipart"
	"net/http"
	"strconv"
)

func WrapGorilla(router *mux.Router) *Router {
	return &Router{
		next: router,
	}
}

type Router struct {
	next *mux.Router
}

func (rt *Router) Get(path string, handler HandlerFunc2) {
	rt.Route("GET", path, handler)
}

func (rt *Router) Post(path string, handler HandlerFunc2) {
	rt.Route("POST", path, handler)
}

func (rt *Router) Delete(path string, handler HandlerFunc2) {
	rt.Route("DELETE", path, handler)
}

func (rt *Router) NotFoundHandler(handler http.Handler) {
	rt.next.NotFoundHandler = handler
}

func (rt *Router) Route(method string, path string, handler HandlerFunc2) {
	rt.next.Methods(method).Path(path).HandlerFunc(WrapHTTPFunc(handler))
}

func (rt *Router) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rt.next.ServeHTTP(w, r)
}

func (rt *Router) Use(mwf ...mux.MiddlewareFunc) {
	rt.next.Use(mwf...)
}

func (rt *Router) StaticFileDir(urlPrefix string, srcDir string) {
	rt.next.PathPrefix(urlPrefix).Handler(http.FileServer(http.Dir(srcDir)))
}

func WrapHTTPFunc(handler HandlerFunc2) http.HandlerFunc {
	return func(wr http.ResponseWriter, req *http.Request) {
		res := handler(NewRequest(wr, req))
		res.Respond(wr, req)
	}
}

func NewRequest(wr http.ResponseWriter, req *http.Request) *Request {
	return &Request{
		req: req,
		wr:  wr,
	}
}

type HandlerFunc2 = func(rq *Request) Responder

var MultipartMaxFormSize = 10 * 1000 * 1000 // 10 MB

type Request struct {
	req *http.Request
	wr  http.ResponseWriter

	parsedMultipart bool
}

func (r *Request) Cookie(name string) string {
	c, err := r.req.Cookie(name)
	if err != nil {
		return ""
	}

	return c.Value
}

func (r *Request) Context() context.Context {
	return r.req.Context()
}

func (r *Request) Writer() http.ResponseWriter {
	return r.wr
}

func (r *Request) ensureMultipartParsed() bool {
	if !r.parsedMultipart {
		r.parsedMultipart = true
		err := r.req.ParseMultipartForm(int64(MultipartMaxFormSize))
		if err != nil {
			log.Println(fmt.Sprintf("failed to parse multipart form for %s: %s", r.req.URL.Path, err.Error()))
		}
	}

	return r.req.MultipartForm != nil
}

// MultipartValue gets the value from the multipart form if it exists
// If multiple values exist, only returns the first value
// if value doesn't exist, will returns ""
func (r *Request) MultipartValue(value string) string {

	if ok := r.ensureMultipartParsed(); !ok {
		return ""
	}

	values := r.req.MultipartForm.Value[value]
	if len(values) == 0 {
		return ""
	}

	return values[0]
}

// MultipartFile gets the file from the multipart form if it exists
// If multiple file exist for the given key, only returns the first value
// if file doesn't exist, will return nil
func (r *Request) MultipartFile(key string) *multipart.FileHeader {
	if ok := r.ensureMultipartParsed(); !ok {
		return nil
	}

	values := r.req.MultipartForm.File[key]
	if len(values) == 0 {
		return nil
	}

	return values[0]
}

func (r *Request) Request() *http.Request {
	return r.req
}

func (r *Request) Query(key string) string {
	return r.req.URL.Query().Get(key)
}

func (r *Request) GetQueryInt(key string) int {
	str := r.Query(key)
	v, _ := strconv.Atoi(str)
	return v
}

func (r *Request) GetCookieValue(name string) string {
	c, err := r.req.Cookie(name)
	if err != nil {
		return ""
	}

	return c.Value
}

func (r *Request) ParseJSON(result interface{}) error {
	return json.NewDecoder(r.req.Body).Decode(result)
}
