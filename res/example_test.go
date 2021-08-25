package res

import (
	sq "github.com/Masterminds/squirrel"
	"github.com/gorilla/mux"
	"github.com/ntbosscher/gobase/env"
	"github.com/ntbosscher/gobase/model/squtil"
	"log"
	"net/http"
	"testing"
	"time"
)

func ExampleSetup() {

	listen := env.Optional("LISTEN", ":8080")

	router := WrapGorilla(mux.NewRouter())
	router.NotFoundHandler(ReactApp("./webapp/build", ":8080"))

	router.Get("/api/todos/list", ListTodos)

	for {
		err := http.ListenAndServe(listen, router)
		log.Println(err)
		time.Sleep(1 * time.Second)
	}
}

func TestExampleSetup(t *testing.T) {

	router := WrapGorilla(mux.NewRouter())
	router.NotFoundHandler(ReactApp("./webapp/build", ":8080", ReactCustomIndexFile(func(r *http.Request) string {
		return "fake.hxml"
	})))

	router.Get("/api/todos/list", ListTodos)

}

func ListTodos(rq *Request) Responder {
	offset := rq.GetQueryInt("offset")

	var list []struct{}

	err := squtil.SelectContext(rq.Context(), &list,
		sq.Select("id", "name").
			From("todo").
			Offset(uint64(offset)))

	if err == nil {
		return AppError(err.Error())
	}

	return Ok(list)
}
