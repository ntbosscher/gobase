package main

import (
	"bytes"
	"fmt"
	"github.com/ntbosscher/gobase/env"
	"github.com/ntbosscher/gobase/er"
	"github.com/ntbosscher/gobase/httpdefaults"
	"github.com/ntbosscher/gobase/integrations/github/githubcd/githubcdutil"
	"github.com/ntbosscher/gobase/res"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"time"
)

var secret string
var remoteHookPath string
var servers []string

func init() {
	secret = env.Require("GITHUB_SECRET")
	remoteHookPath = env.Require("REMOTE_HOOK_PATH")
	servers = strings.Split(env.Require("SERVERS"), ",")
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

	go func() {
		defer er.HandleErrors(func(input *er.HandlerInput) {
			log.Println(input.Error, input.StackTrace)
		})

		err := performUpgrade(body, servers)
		if err != nil {
			log.Println(err)
		}
	}()

	return res.Func(func(w http.ResponseWriter) {
		w.WriteHeader(200)
		w.Write([]byte("Accepted"))
	})
}

func performUpgrade(requestBody []byte, servers []string) error {

	log.Println("starting upgrade")

	for _, name := range servers {

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

		<-time.After(1 * time.Second)
	}

	log.Println("done deployment")
	return nil
}
