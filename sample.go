package main

import (
	"github.com/Masterminds/squirrel"
	"github.com/ntbosscher/gobase/auth"
	"github.com/ntbosscher/gobase/auth/httpauth"
	"github.com/ntbosscher/gobase/env"
	"github.com/ntbosscher/gobase/er"
	"github.com/ntbosscher/gobase/httpdefaults"
	"github.com/ntbosscher/gobase/integrations/s3fs"
	"github.com/ntbosscher/gobase/model"
	"github.com/ntbosscher/gobase/model/squtil"
	"github.com/ntbosscher/gobase/requestip"
	"github.com/ntbosscher/gobase/res"
	"github.com/ntbosscher/gobase/res/r"
	"github.com/pkg/errors"
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

	// restrict to internal users
	router.Add("POST", "/api/product", todo, RoleInternal)
	router.Add("PUT", "/api/product", todo, RoleInternal)
	router.Add("POST", "/api/product/upload", uploadProduct, routeSpecificMiddleware, RoleInternal)

	// api versioning (based on X-APIVersion header)
	router.Add("POST", "/api/customer/create", r.Versioned(
		r.DefaultVersion(todo),
		r.Version("1", todo),
	))

	// rate limiting
	router.Add("GET", "/api/admin/reports", todo, r.RateLimit(10, 10*time.Second))

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

var routeSpecificMiddleware = r.Middleware(func(router *r.Router, method string, path string, handler res.HandlerFunc2) res.HandlerFunc2 {
	return func(rq *res.Request) res.Responder {
		if rq.Request().Header.Get("X-Source") == "mobile" {
			return res.Error(errors.New("mobile not supported on this route"))
		}

		return handler(rq)
	}
})

const (
	RoleUser      auth.TRole = 0x1 << iota
	RoleSupport   auth.TRole = 0x1 << iota
	RoleSuperUser auth.TRole = 0x1 << iota
)

const (
	RoleInternal = RoleSuperUser | RoleSupport
)

func getProducts(rq *res.Request) res.Responder {
	return res.Todo()
}

func uploadProduct(rq *res.Request) res.Responder {
	file := rq.MultipartFile("file")

	err := s3fs.Upload(rq.Context(), []*s3fs.UploadInput{{
		FileName:   file.Filename,
		Key:        "product-file",
		FileHeader: file,
	}})

	if err != nil {
		return res.Error(err)
	}

	return res.Ok()
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

	qr := model.Builder.Select("*").From("customer").Where(squirrel.Eq{
		"company": auth.Company(rq.Context()),
	}).Limit(uint64(limit))

	// db transaction flows from model.AttachTxHandler through rq.Context() and
	// will be auto committed if we return a non-error response
	//
	// get current user's company id from context
	err := squtil.GetContext(rq.Context(), customer, qr)
	er.Check(err) // return http 500 with json encoded {error: "string"} value

	// return http 200 with customer json encoded with camelCase keys
	return res.Ok(customer)
}
