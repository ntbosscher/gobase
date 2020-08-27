package main

import (
	"context"
	"github.com/gorilla/mux"
	"github.com/ntbosscher/gobase/auth"
	"github.com/ntbosscher/gobase/auth/authhttp"
	"github.com/ntbosscher/gobase/httpdefaults"
	"github.com/ntbosscher/gobase/model"
	"github.com/ntbosscher/gobase/res"
	"log"
)

func main() {
	router := mux.NewRouter()
	router.Use(
		model.AttachTxHandler,
		authhttp.Middleware(authhttp.Config{
			CredentialChecker: func(ctx context.Context, credential *authhttp.Credential) (*auth.UserInfo, error) {
				// db lookup
				return &auth.UserInfo{
					UserID: 103,
				}, nil
			},
		}))

	routes := res.WrapGorilla(router)

	routes.Get("/api/users", func(rq *res.Request) res.Responder {

		limit := rq.GetQueryInt("limit")
		customer := &Customer{}

		if err := model.GetContext(rq.Context(), customer, "select * from customer where company = $1 limit $2", auth.Company(rq.Context()), limit); err != nil {
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
