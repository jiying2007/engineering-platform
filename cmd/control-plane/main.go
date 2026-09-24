package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	corestore "github.com/jiying2007/engineering-platform/internal/store"
	pgstore "github.com/jiying2007/engineering-platform/internal/store/postgres"
)

func main() {
	if err := serve(); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}

func serve() error {
	config, err := loadConfiguration(os.Getenv)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	var backend corestore.Store
	if config.development {
		backend = corestore.NewMemory()
		log.Print("control plane mode=INSECURE_DEV backend=memory; unauthenticated loopback fixture only")
	} else {
		startup, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		persistent, err := pgstore.Open(startup, config.databaseURL)
		if err != nil {
			return err
		}
		defer persistent.Close()
		if config.autoMigrate {
			if err := persistent.ApplyCoreMigration(startup); err != nil {
				return err
			}
		}
		backend = persistent
		log.Print("control plane mode=mtls backend=postgres; explicit platform-scoped access policy")
	}
	server, err := assembleServer(config, backend)
	if err != nil {
		return err
	}
	result := make(chan error, 1)
	go func() {
		if config.development {
			result <- server.ListenAndServe()
		} else {
			result <- server.ListenAndServeTLS("", "")
		}
	}()
	log.Printf("control plane listening on %s", server.Addr)
	select {
	case err := <-result:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		err := server.Shutdown(shutdown)
		if err != nil {
			_ = server.Close()
		}
		<-result
		return err
	}
}
