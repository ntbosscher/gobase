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
	rt.next.Methods(method).Path(path).HandlerFunc(rt.wrapFunc(handler))
}

func (rt *Router) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rt.next.ServeHTTP(w, r)
}

func (rt *Router) wrapFunc(handler HandlerFunc2) http.HandlerFunc {
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
