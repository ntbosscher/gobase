package res

import (
	sq "github.com/Masterminds/squirrel"
	"github.com/gorilla/mux"
	"github.com/ntbosscher/gobase/env"
	"github.com/ntbosscher/gobase/model"
	"log"
	"net/http"
	"time"
)

func ExampleSetup() {

	listen := env.Optional("LISTEN", ":8080")

	router := WrapGorilla(mux.NewRouter())
	router.NotFoundHandler(ReactApp("./webapp/build"))

	router.Get("/api/todos/list", ListTodos)

	for {
		err := http.ListenAndServe(listen, router)
		log.Println(err)
		time.Sleep(1 * time.Second)
	}
}

func ListTodos(rq *Request) Responder {
	offset := rq.GetQueryInt("offset")

	var list []struct{}

	err := model.SelectContext(rq.Context(), &list,
		sq.Select("id", "name").
			From("todo").
			Offset(uint64(offset)))

	if err == nil {
		return AppError(err.Error())
	}

	return Ok(list)
}
