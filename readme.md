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
