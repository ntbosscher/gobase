package main

import (
	"github.com/gorilla/mux"
	"github.com/ntbosscher/gobase/httpdefaults"
	"github.com/ntbosscher/gobase/model"
	"github.com/ntbosscher/gobase/res"
	"log"
)

func main() {
	router := mux.NewRouter()
	router.Use(model.AttachTxHandler)
	routes := res.WrapGorilla(router)

	routes.Get("/api/users", func(rq *res.Request) res.Responder {

		id := rq.GetQueryInt("id")
		customer := &Customer{}

		if err := model.GetContext(rq.Context(), customer, "select * from customer where id = $1", id); err != nil {
			return res.AppError(err.Error())
		}

		return res.Ok(customer)
	})

	server := httpdefaults.Server("8080", router)
	log.Println("Serving on " + server.Addr)
	log.Fatal(server.ListenAndServe())
}

type Customer struct {
}
