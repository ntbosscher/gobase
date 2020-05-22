package main

import (
	"github.com/gorilla/mux"
	"github.com/ntbosscher/gobase/httpdefaults"
	"github.com/ntbosscher/gobase/model"
	"log"
)

func main() {
	router := mux.NewRouter()
	router.Use(model.AttachTxHandler)

	server := httpdefaults.Server("8080", router)
	log.Println("Serving on " + server.Addr)
	log.Fatal(server.ListenAndServe())
}

