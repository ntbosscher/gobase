package main

import (
	"github.com/ntbosscher/gobase/auth"
	"github.com/ntbosscher/gobase/auth/httpauth"
	"github.com/ntbosscher/gobase/env"
	"github.com/ntbosscher/gobase/er"
	"github.com/ntbosscher/gobase/httpdefaults"
	"github.com/ntbosscher/gobase/model"
	"github.com/ntbosscher/gobase/requestip"
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

	// make requestip.IP() available
	router.Use(requestip.Middleware())

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

type User struct {
	ID        int
	FirstName string
	Email     string
	CreatedBy int
	Company   int `json:"-"`
}

func createUserHandler(rq *res.Request) res.Responder {

	// define the request structure, could have used
	// the User struct here instead.
	input := &struct {
		FirstName string
		Email     string
	}{}

	// parse request body, if fails will return a 400 with error details
	rq.MustParseJSON(input)

	// use the auth-context to get which company/tenant (for multi-tenant systems)
	company := auth.Company(rq.Context())
	currentUser := auth.User(rq.Context())

	// create user using if/err flow
	id := 0
	err := model.QueryRowContext(rq.Context(), `insert into user 
		(first_name, email, company, created_by) 
		values ($1, $2, $3, $4) returning id`,
		input.FirstName, input.Email, company, currentUser).Scan(&id)

	if model.IsDuplicateKeyError(err) {
		return res.AppError("That email is already in the system")
	}

	// fetch user
	// db transaction flows from model.AttachTxHandler through rq.Context() and
	// will be auto committed if we return a non-error response
	customer := &User{}
	model.MustGetContext(rq.Context(), customer, `select * from user where id = $1`, id)

	// returns json with http-status 200 -> { id: 1, firstName: "", email: "", createdBy: 1 }
	return res.Ok(customer)
}

func todo(rq *res.Request) res.Responder {
	return res.Todo()
}

type Customer struct{}

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
