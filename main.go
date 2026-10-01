package main

import (
	"fmt"
	"log"
	"net/http"
)

func main() {
	server := NewServer()
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /{key}", server.handleCreateKV)
	mux.HandleFunc("GET /{key}", server.handleRetrieveKV)
	mux.HandleFunc("DELETE /{key}", server.handleDeleteKV)

	srv := http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	fmt.Println("server is starting...")
	log.Fatal(srv.ListenAndServe())
}
