package nonce

import (
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/ntbosscher/gobase/er"
)

type nonce struct {
	used map[string]time.Time
	mu   *sync.Mutex
}

func (n *nonce) ServeHTTP(wr http.ResponseWriter, rq *http.Request, next http.Handler) {
	key := rq.Header.Get("X-Nonce")

	if rq.Method == "GET" || key == "" {
		next.ServeHTTP(wr, rq)
		return
	}

	if !n.take(key) {
		wr.WriteHeader(http.StatusBadRequest)
		wr.Write([]byte("{\"error\":\"Nonce already used. refresh the page and try again\"}"))
		return
	}

	next.ServeHTTP(wr, rq)
}

func (n *nonce) take(key string) bool {
	n.mu.Lock()
	defer n.mu.Unlock()

	if _, ok := n.used[key]; ok {
		return false
	}

	n.used[key] = time.Now()
	return true
}

func (n *nonce) cleaner() {
	defer er.HandleErrors(func(input *er.HandlerInput) {
		log.Println(input)
	})

	tc := time.NewTicker(5 * time.Minute)
	for {
		<-tc.C
		n.mu.Lock()
		for k, v := range n.used {
			if time.Since(v) > 10*time.Minute {
				delete(n.used, k)
			}
		}
		n.mu.Unlock()
	}

}

func New() func(next http.Handler) http.Handler {
	n := &nonce{
		used: map[string]time.Time{},
		mu:   &sync.Mutex{},
	}

	go n.cleaner()

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(wr http.ResponseWriter, rq *http.Request) {
			n.ServeHTTP(wr, rq, next)
		})
	}
}
