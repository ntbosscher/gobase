# GoBase

This is a group of utils I commonly use in my golang applications

## Install

```
# go package
go get github.com/ntbosscher/gobase
```
```
# .env
touch .env # add .env to your .gitignore

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

## Sample

```golang
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
```
