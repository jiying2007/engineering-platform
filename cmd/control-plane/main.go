package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/jiying2007/engineering-platform/internal/api"
	corestore "github.com/jiying2007/engineering-platform/internal/store"
	pgstore "github.com/jiying2007/engineering-platform/internal/store/postgres"
)

type closeableStore interface {
	corestore.Store
	Close()
}

func main() {
	ctx := context.Background()

	var backend corestore.Store
	var persistent closeableStore
	if databaseURL := os.Getenv("DATABASE_URL"); databaseURL != "" {
		store, err := pgstore.Open(ctx, databaseURL)
		if err != nil {
			log.Fatalf("open PostgreSQL backend: %v", err)
		}
		persistent = store
		backend = store

		if os.Getenv("AUTO_MIGRATE") == "1" {
			if err := store.ApplyCoreMigration(ctx); err != nil {
				store.Close()
				log.Fatalf("apply Core migration: %v", err)
			}
		}
		log.Printf("engineering control plane backend=postgres")
	} else {
		backend = corestore.NewMemory()
		log.Printf("engineering control plane backend=memory (development only)")
	}
	if persistent != nil {
		defer persistent.Close()
	}

	handler := api.NewServer(backend).Handler()

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
