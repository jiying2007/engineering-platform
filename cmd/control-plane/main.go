package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/jiying2007/engineering-platform/internal/api"
	"github.com/jiying2007/engineering-platform/internal/store"
)

func main() {
	handler := api.NewServer(store.NewMemory()).Handler()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	server := &http.Server{
		Addr:              ":" + port,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	log.Printf("engineering control plane listening on %s", server.Addr)
	log.Fatal(server.ListenAndServe())
}
