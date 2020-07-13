package res

import (
	"context"
	"encoding/json"
	"github.com/gorilla/mux"
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

func (r *Router) Get(path string, handler HandlerFunc2) {
	r.Route("GET", path, handler)
}

func (r *Router) Post(path string, handler HandlerFunc2) {
	r.Route("POST", path, handler)
}

func (r *Router) Delete(path string, handler HandlerFunc2) {
	r.Route("DELETE", path, handler)
}

func (r *Router) NotFoundHandler(handler http.Handler) {
	r.next.NotFoundHandler = handler
}

func (r *Router) Route(method string, path string, handler HandlerFunc2) {
	r.next.Methods(method).Path(path).HandlerFunc(r.wrapFunc(handler))
}

func (r *Router) wrapFunc(handler HandlerFunc2) http.HandlerFunc {
	return func(wr http.ResponseWriter, req *http.Request) {
		res := handler(&Request{
			req: req,
			wr:  wr,
		})

		res.Respond(wr, req)
	}
}

type HandlerFunc2 = func(rq *Request) Responder

type Request struct {
	req *http.Request
	wr  http.ResponseWriter
}

func (r *Request) Context() context.Context {
	return r.req.Context()
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

func (r *Request) ParseJSON(result interface{}) error {
	return json.NewDecoder(r.req.Body).Decode(result)
}
