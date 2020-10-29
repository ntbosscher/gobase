package main

import (
	"github.com/ntbosscher/gobase/auth"
	"github.com/ntbosscher/gobase/auth/httpauth"
	"github.com/ntbosscher/gobase/env"
	"github.com/ntbosscher/gobase/er"
	"github.com/ntbosscher/gobase/httpdefaults"
	"github.com/ntbosscher/gobase/model"
	"github.com/ntbosscher/gobase/res"
	"github.com/ntbosscher/gobase/res/r"
	"log"
	"os"
	"time"
)

func init() {
	// enable error logging
	res.SetErrorResponseLogging(os.Stdout)
}

func main() {

	router := r.NewRouter()

	// setup database transactions
	router.Use(model.AttachTxHandler("/websocket"))

	// setup auth
	router.WithAuth(httpauth.Config{
		// .. setup jwt-based authentication
		// .. oauth setup (optional)
	})

	// serve react-app
	router.ReactApp("/", "./react-app/build", "localhost:3000")

	// simple route
	router.Add("GET", "/api/products", getProducts)

	// role-based routing
	router.WithRole(RoleInternal, func(rt *r.RoleRouter) {
		router.Add("POST", "/api/product", todo)
		router.Add("PUT", "/api/product", todo)
	})

	// api versioning (based on X-APIVersion header)
	router.Versioned("POST", "/api/customer/create",
		r.DefaultVersion(todo),
		r.Version("1", todo),
	)

	// rate limiting
	router.Add("GET", "/api/admin/reports", todo, r.RouteConfig{
		RateLimit: &r.RateLimitConfig{
			Count:  10,
			Window: 10 * time.Second,
		},
	})

	// receive a github post-push hook and auto-update ourselves
	router.GithubContinuousDeployment(res.GithubCDInput{
		Path:           "/api/github-auto-deploy",
		Secret:         env.Require("GITHUB_DEPLOY_KEY"),
		PostPullScript: "./rebuild-and-migrate.sh",
	})

	server := httpdefaults.Server("8080", router)
	log.Println("Serving on " + server.Addr)
	log.Fatal(server.ListenAndServe())
}

const (
	RoleUser      = 0x1 << iota
	RoleSupport   = 0x1 << iota
	RoleSuperUser = 0x1 << iota
)

const (
	RoleInternal = RoleSuperUser | RoleSupport
)

func getProducts(rq *res.Request) res.Responder {
	return res.Todo()
}

func todo(rq *res.Request) res.Responder {
	return res.Todo()
}

func getCustomers(rq *res.Request) res.Responder {

	// parse 'limit' query parameter
	limit := rq.GetQueryInt("limit")
	customer := &Customer{}

	// db transaction flows from model.AttachTxHandler through rq.Context() and
	// will be auto committed if we return a non-error response
	//
	// get current user's company id from context
	err := model.GetContext(rq.Context(), customer, "select * from customer where company = $1 limit $2", auth.Company(rq.Context()), limit)
	er.Check(err) // return http 500 with json encoded {error: "string"} value

	// return http 200 with customer json encoded with camelCase keys
	return res.Ok(customer)
}

func createCustomer(rq *res.Request) res.Responder {

	customer := &Customer{}

	// parse customer from request body
	err := rq.ParseJSON(customer)
	er.CheckForDecode(err) // return http 400 with json encoded {error: "string"} value

	// db transaction flows from model.AttachTxHandler through rq.Context() and
	// will be auto committed if we return a non-error response
	//
	// get current user's company id from context
	id, err := model.Insert(rq.Context(), "insert into customer set name = $1, company = $2 ", customer.Name, auth.Company(rq.Context()))
	er.Check(err) // return http 500 with json encoded {error: "string"} value

	err = model.GetContext(rq.Context(), customer, `select * from customer where id = $1`, id)
	er.Check(err) // return http 500 with json encoded {error: "string"} value

	// return http 200 with customer json encoded with camelCase keys
	return res.Ok(customer)
}

type Customer struct {
	Name string
}
