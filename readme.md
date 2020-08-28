# GoBase

This is a group of utils I commonly use in my golang applications

## Install

```
# go package
go get github.com/ntbosscher/gobase
```
```
# .env
# postgres | mysql (optional, default=postgres)
DB_TYPE=

# mysql/maria connection string details see: https://github.com/go-sql-driver/mysql
# host=127.0.0.1 port=5432 user=... (required)
CONNECTION_STRING=
 
# is testing mode
# true|false|undefined (optional, default=false)
# - in react-router will use local node server instead of build folder
# TEST=
```

If you're using httpauth, create a `.jwtkey` file. 
You might find [jwtgen-util](./auth/httpauth/jwtgen) to be helpful.

## Sample

```golang
package main

import (
	"context"
	"github.com/ntbosscher/gobase/auth"
	"github.com/ntbosscher/gobase/auth/httpauth"
	"github.com/ntbosscher/gobase/httpdefaults"
	"github.com/ntbosscher/gobase/model"
	"github.com/ntbosscher/gobase/res"
	"log"
)

func main() {
	router := res.NewRouter()
	router.Use(model.AttachTxHandler)
	httpauth.Setup(router, httpauth.Config{
		CredentialChecker: func(ctx context.Context, credential *httpauth.Credential) (*auth.UserInfo, error) {
			// db lookup
			return &auth.UserInfo{
				UserID: 103,
			}, nil
		},
	})

	router.ReactApp("/", "./react-app/build", "localhost:3000")
	router.Get("/api/customers", getCustomers)
	router.Post("/api/customer/create", createCustomer)

	server := httpdefaults.Server("8080", router)
	log.Println("Serving on " + server.Addr)
	log.Fatal(server.ListenAndServe())
}

func getCustomers(rq *res.Request) res.Responder {

	// parse 'limit' query parameter
	limit := rq.GetQueryInt("limit")
	customer := &Customer{}

	// db transaction flows from model.AttachTxHandler through rq.Context() and
	// will be auto committed if we return a non-error response
	//
	// get current user's company id from context
	if err := model.GetContext(rq.Context(), customer, "select * from customer where company = $1 limit $2", auth.Company(rq.Context()), limit); err != nil {
		// return http 500 with json encoded {error: "string"} value
		return res.AppError(err.Error())
	}

	// return http 200 with customer json encoded with camelCase keys
	return res.Ok(customer)
}

func createCustomer(rq *res.Request) res.Responder {

	customer := &Customer{}

	// parse customer from request body
	if err := rq.ParseJSON(customer); err != nil {
		// return http 400 with json encoded {error: "string"} value
		return res.BadRequest(err.Error())
	}

	// db transaction flows from model.AttachTxHandler through rq.Context() and
	// will be auto committed if we return a non-error response
	//
	// get current user's company id from context
	id, err := model.Insert(rq.Context(), "insert into customer set name = $1, company = $2 ", customer.Name, auth.Company(rq.Context()))
	if err != nil {
		// return http 500 with json encoded {error: "string"} value
		return res.AppError(err.Error())
	}

	if err = model.GetContext(rq.Context(), customer, `select * from customer where id = $1`, id); err != nil {
		return res.AppError(err.Error())
	}

	// return http 200 with customer json encoded with camelCase keys
	return res.Ok(customer)
}

type Customer struct {
	Name string
}
```
