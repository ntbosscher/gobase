package res

import (
	"context"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"

	"github.com/gorilla/mux"
	"github.com/ntbosscher/gobase/apiversion"
	"github.com/ntbosscher/gobase/env"
	"github.com/ntbosscher/gobase/er"
	"github.com/ntbosscher/gobase/integrations/github/githubcd"
	"github.com/ntbosscher/gobase/strs"
)

var Verbose = false

func WrapGorilla(router *mux.Router) *Router {
	return &Router{
		next: router,
	}
}

type Router struct {
	next *mux.Router
}

func NewRouter() *Router {
	return WrapGorilla(mux.NewRouter())
}

func (rt *Router) Get(path string, handler HandlerFunc2) {
	rt.Route("GET", path, handler)
}

func (rt *Router) Put(path string, handler HandlerFunc2) {
	rt.Route("PUT", path, handler)
}

func (rt *Router) Post(path string, handler HandlerFunc2) {
	rt.Route("POST", path, handler)
}

func (rt *Router) Delete(path string, handler HandlerFunc2) {
	rt.Route("DELETE", path, handler)
}

type GithubCDInput struct {

	// default: /api/github-deploy
	Path string

	// required
	Secret string

	// required: script to run after the git repo is updated
	PostPullScript string
}

const DefaultGithubCdPath = "/api/github-deploy"

func (rt *Router) GithubContinuousDeployment(input GithubCDInput) {
	path := strs.Coalesce(input.Path, DefaultGithubCdPath)
	handler := githubcd.New(input.Secret, input.PostPullScript)

	rt.next.Methods("POST").Path(path).Handler(handler)
}

func (rt *Router) NotFoundHandler(handler http.Handler) {
	rt.next.NotFoundHandler = handler
}

func (rt *Router) Route(method string, path string, handler HandlerFunc2) {
	if !strings.HasPrefix(path, "/") {
		panic("path must start with /")
	}

	rt.next.Methods(method).Path(path).HandlerFunc(WrapHTTPFunc(handler))
}

func (rt *Router) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Apply the body-size cap at the router entry so it covers every route,
	// including raw http.Handler routes that don't go through WrapHTTPFunc (e.g.
	// the github-deploy webhook, which reads its body to verify the HMAC). Normal
	// res handlers re-apply the same cap in WrapHTTPFunc; re-wrapping with the same
	// limit is harmless.
	if MaxRequestBodySize > 0 && r.Body != nil {
		r.Body = http.MaxBytesReader(w, r.Body, MaxRequestBodySize)
	}

	rt.next.ServeHTTP(w, r)
}

func (rt *Router) Use(mwf ...mux.MiddlewareFunc) {
	rt.next.Use(mwf...)
}

func (rt *Router) StaticFileDir(urlPrefix string, srcDir string) {
	fileServer := http.StripPrefix(urlPrefix, http.FileServer(http.Dir(srcDir)))
	rt.next.PathPrefix(urlPrefix).Handler(fileServer)
}

// ReactApp serves the react app located at srcDir. This works the similar to
// StaticFileDir except:
// - ReactApp serves index.html on all not-found routes (to support virtual routing)
// - In testing mode (environment variable TEST=true) reverse proxies to create-react-app's node server on port given
func (rt *Router) ReactApp(urlPrefix string, srcDir string, testNodeServerAddr string, cfg ...ReactConfig) {
	react := ReactApp(srcDir, testNodeServerAddr, cfg...)

	if !env.IsTesting {
		react = http.StripPrefix(urlPrefix, react)
	}

	rt.next.PathPrefix(urlPrefix).
		Handler(funcToHttpServer(func(writer http.ResponseWriter, request *http.Request) {
			if strings.HasPrefix(request.URL.Path, urlPrefix) {
				react.ServeHTTP(writer, request)
				return
			}

			http.NotFound(writer, request)
		}))
}

func funcToHttpServer(handler http.HandlerFunc) http.Handler {
	return &funcServer{
		handler: handler,
	}
}

type funcServer struct {
	handler http.HandlerFunc
}

func (f *funcServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.handler(w, r)
}

func WrapHTTPFunc(handler HandlerFunc2) http.HandlerFunc {
	return func(wr http.ResponseWriter, req *http.Request) {
		defer er.HandleErrors(func(input *er.HandlerInput) {
			handlePanic(input).Respond(wr, req)
		})

		if MaxRequestBodySize > 0 && req.Body != nil {
			req.Body = http.MaxBytesReader(wr, req.Body, MaxRequestBodySize)
		}

		res := handler(NewRequest(wr, req))
		res.Respond(wr, req)
	}
}

func AutoHandleHttpPanics(wr http.ResponseWriter, req *http.Request) {
	er.HandleErrors(func(input *er.HandlerInput) {
		handlePanic(input).Respond(wr, req)
	})
}

func NewRequest(wr http.ResponseWriter, req *http.Request) *Request {
	return &Request{
		req: req,
		wr:  wr,
	}
}

type HandlerFunc2 = func(rq *Request) Responder

var MultipartMaxFormSize = 10 * 1000 * 1000 // 10 MB

// MaxRequestBodySize caps the total number of bytes any handler can read from a
// request body (via http.MaxBytesReader), across the whole request rather than
// just the in-memory portion. MultipartMaxFormSize only bounds what a multipart
// parse buffers in memory before spilling the rest to disk; without this cap an
// upload can be driven to exhaust disk, and handlers that io.ReadAll +
// decode the whole body are exposed to decompression/size DoS.
//
// The limit is enforced at read time: once exceeded the body read fails, which
// surfaces as an error through the normal handler/panic path. Apps that legitimately
// accept larger uploads should raise this (or set it to 0 to disable the cap)
// during setup, before routes serve traffic.
var MaxRequestBodySize int64 = 64 * 1000 * 1000 // 64 MB

type Request struct {
	req *http.Request
	wr  http.ResponseWriter

	parsedMultipart bool
	multipartErr    error

	parsedForm bool
	formErr    error
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

func (r *Request) ensureFormParsed() bool {
	if !r.parsedForm {
		r.parsedForm = true
		err := r.req.ParseForm()
		if err != nil {
			r.formErr = err
		}
	}

	return r.req.Form != nil
}

func (r *Request) FormError() error {
	r.ensureFormParsed()
	return r.formErr
}

// FormValue gets the value of the form if it exists
// If the form is invalid, returns blank string
func (r *Request) FormValue(key string) string {
	if !r.ensureFormParsed() {
		return ""
	}

	return r.req.PostForm.Get(key)
}

func (r *Request) ensureMultipartParsed() bool {
	if !r.parsedMultipart {
		r.parsedMultipart = true
		err := r.req.ParseMultipartForm(int64(MultipartMaxFormSize))
		if err != nil {
			r.multipartErr = err
		}
	}

	return r.req.MultipartForm != nil
}
func (r *Request) MultipartError() error {
	r.ensureMultipartParsed()
	return r.multipartErr
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

func (r *Request) WithContext(ctx context.Context) {
	r.req = r.req.WithContext(ctx)
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

func (r *Request) MustParseJSON(result interface{}) {
	err := r.ParseJSON(result)
	er.CheckForDecode(err)
}

func MustParseJSON[T any](r *Request, result T) T {
	err := r.ParseJSON(result)
	er.CheckForDecode(err)
	return result
}

func (r *Request) APIVersion() *apiversion.Ver {
	return apiversion.Current(r.Context())
}
