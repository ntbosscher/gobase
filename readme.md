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

## Sample Router Config

```golang
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
```

## Sample Handler

```golang
type Customer struct {
	ID        int
	FirstName string
	Email     string
	Store     int
}

func createCustomer(rq *res.Request) res.Responder {

	// define the request structure, could have used
	// the Customer struct here instead.
	input := &struct {
		FirstName string
		Email     string
		Store     int
	}{}

	// parse request body, if fails will return a 400 with error details
	rq.MustParseJSON(input)

	// create customer using if/err flow
	id := 0
	err := model.QueryRowContext(rq.Context(), `insert into customer 
		(first_name, email, store) 
		values ($1, $2, $3) returning id`,
		input.FirstName, input.Email, input.Store).Scan(&id)

	if model.IsDuplicateKeyError(err) {
		return res.AppError("That email is already in the system")
	}

	// fetch customer
	// db transaction flows from model.AttachTxHandler through rq.Context() and
	// will be auto committed if we return a non-error response
	customer := &Customer{}
	model.MustGetContext(rq.Context(), customer, `select * from customer where id = $1`, id)

	// returns json with http-status 200 -> { id: 1, firstName: "", email: "", store: 0 }
	return res.Ok(customer)
}
```