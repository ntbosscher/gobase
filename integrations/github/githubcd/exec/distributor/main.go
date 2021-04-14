package main

import (
	"bytes"
	"fmt"
	"github.com/ntbosscher/gobase/env"
	"github.com/ntbosscher/gobase/er"
	"github.com/ntbosscher/gobase/httpdefaults"
	"github.com/ntbosscher/gobase/integrations/github/githubcd/exec/distributor/nginx"
	"github.com/ntbosscher/gobase/integrations/github/githubcd/githubcdutil"
	"github.com/ntbosscher/gobase/res"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"time"
)

var configFile string
var secret string
var remoteHookPath string

func init() {
	configFile = env.Require("NGINX_BACKEND_CONFIG")
	secret = env.Require("GITHUB_SECRET")
	remoteHookPath = env.Require("REMOTE_HOOK_PATH")
}

func main() {
	rt := res.NewRouter()
	rt.Post(env.Require("LISTEN_PATH"), handler)

	listen := env.Require("LISTEN")
	fmt.Println("listening on:", listen)
	for {
		err := httpdefaults.Server(listen, rt).ListenAndServe()
		if err != nil {
			log.Println(err)
			<-time.After(1 * time.Second)
		}
	}
}

func handler(rq *res.Request) res.Responder {

	body, err := githubcdutil.Verify(secret, rq.Request())
	if err != nil {
		return res.WithCodeAndMessage(403, "bad signature")
	}

	data, err := ioutil.ReadFile(configFile)
	if err != nil {
		log.Println("failed to read nginx config:", err)
		return res.InternalServerError("failed to update")
	}

	file, servers, err := nginx.ParseBackends(data)
	if err != nil {
		log.Println("failed to read nginx config:", err)
		return res.InternalServerError("failed to update")
	}

	go func() {
		defer er.HandleErrors(func(input *er.HandlerInput) {
			log.Println(input.Error, input.StackTrace)
		})

		err := performUpgrade(body, file, servers)
		if err != nil {
			log.Println(err)
		}
	}()

	return res.Func(func(w http.ResponseWriter) {
		w.WriteHeader(200)
		w.Write([]byte("Accepted"))
	})
}

func performUpgrade(requestBody []byte, file nginx.File, servers []*nginx.ServerRow) error {

	log.Println("starting upgrade")

	serverMap := map[string][]*nginx.ServerRow{}
	for _, server := range servers {
		// ignore servers that are already down
		if server.Status == "down" {
			continue
		}

		serverMap[server.Name] = append(serverMap[server.Name], server)
	}

	normalExit := false

	// restore config if something fails
	defer func() {
		if normalExit {
			return
		}

		log.Println("restoring after failure")
		for _, values := range serverMap {
			for _, server := range values {
				server.Status = ""
			}
		}

		if err := updateNginxConfig(file); err != nil {
			log.Println("failed to restore nginx config:", err)
		}
	}()

	for name, values := range serverMap {

		log.Println(name, "taking down backend...")

		for _, value := range values {
			value.Status = "down"
		}

		if err := updateNginxConfig(file); err != nil {
			log.Println("failed to update config:", err)
		}

		log.Println(name, "waiting for requests to drain")
		<-time.After(10 * time.Second)

		log.Println("sending", name, "the update webhook...")

		rq, err := http.NewRequest("POST", "http://"+name+remoteHookPath, bytes.NewReader(requestBody))
		if err != nil {
			log.Println(err)
		}

		if err := githubcdutil.SignAndSetHeader(rq, secret, requestBody); err != nil {
			log.Println("failed to sign header:", err)
		}

		resp, err := http.DefaultClient.Do(rq)
		if err != nil {
			log.Println(err)
		}

		if resp.StatusCode >= 400 {
			data, _ := ioutil.ReadAll(resp.Body)
			log.Println(resp.StatusCode, string(data))
		}

		log.Println(name, "delaying while client builds and updates it self...")
		<-time.After(5 * time.Second)

		timeout := time.Now().Add(30 * time.Second)
		for time.Now().Before(timeout) {
			resp, err := http.Get("http://" + name + "/")
			if err != nil {
				log.Println(name, "attempting...", err.Error())
				<-time.After(1 * time.Second)
				continue
			}

			log.Println(name, "got response", resp.StatusCode, "must be good to go")
			break
		}

		log.Println(name, "restoring nginx config...")
		for _, value := range values {
			value.Status = ""
		}

		if err := updateNginxConfig(file); err != nil {
			log.Println("failed to update config:", err)
		}

		<-time.After(1 * time.Second)
	}

	log.Println("done deployment")

	normalExit = true
	return nil
}

func updateNginxConfig(file nginx.File) error {
	err := ioutil.WriteFile(configFile, []byte(file.String()), os.ModePerm)
	if err != nil {
		return err
	}

	return nginx.ReloadConfig()
}
