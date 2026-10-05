package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	corestore "github.com/jiying2007/engineering-platform/internal/store"
	pgstore "github.com/jiying2007/engineering-platform/internal/store/postgres"
	"github.com/jiying2007/engineering-platform/internal/workerloop"
)

func main() {
	if err := serve(); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}
func serve() error {
	productionMode := flag.Bool("production", false, "enforce the production startup boundary")
	flag.Parse()
	if flag.NArg() != 0 {
		return fmt.Errorf("unexpected control-plane arguments")
	}
	env := os.Getenv
	if *productionMode {
		if mode := os.Getenv("DEPLOYMENT_MODE"); mode != "" && mode != "production" {
			return fmt.Errorf("production startup rejects conflicting deployment mode")
		}
		env = func(key string) string {
			if key == "DEPLOYMENT_MODE" {
				return "production"
			}
			return os.Getenv(key)
		}
	}
	config, err := loadConfiguration(env)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	var backend corestore.Store
	var persistent *pgstore.Store
	if config.development {
		backend = corestore.NewMemory()
		log.Print("control plane mode=INSECURE_DEV backend=memory; worker inbox disabled")
	} else {
		startup, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		persistent, err = pgstore.Open(startup, config.databaseURL)
		if err != nil {
			return err
		}
		defer persistent.Close()
		if config.autoMigrate {
			if err := persistent.ApplyCoreMigration(startup); err != nil {
				return err
			}
		}
		if err := persistent.CheckWorkerSchema(startup); err != nil {
			return err
		}
		backend = persistent
	}
	server, err := assembleServer(config, backend)
	if err != nil {
		return err
	}
	// Bind before starting the local relay; a failed listener must not consume work.
	listener, err := net.Listen("tcp", server.Addr)
	if err != nil {
		return err
	}
	defer listener.Close()
	var relay func(context.Context) error
	if persistent != nil {
		var nonce [16]byte
		if _, err := rand.Read(nonce[:]); err != nil {
			return err
		}
		owner := "control-relay:" + hex.EncodeToString(nonce[:])
		relay = func(ctx context.Context) error { _, err := persistent.RelayRunStarts(ctx, owner, 16); return err }
	}
	log.Printf("control plane listening on %s; worker lane=input-admission-only", listener.Addr())
	return runControlServer(ctx, server, listener, config.development, relay)
}

// The real entrypoint and command integration tests share this entire serving
// lifecycle. A listener is already bound before any relay callback is invoked.
func runControlServer(ctx context.Context, server *http.Server, listener net.Listener, development bool, relay func(context.Context) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	runCtx, cancelRun := context.WithCancel(ctx)
	defer cancelRun()
	serverDone := make(chan error, 1)
	go func() {
		if development {
			serverDone <- server.Serve(listener)
		} else {
			serverDone <- server.ServeTLS(listener, "", "")
		}
	}()
	var relayDone chan error
	if relay != nil {
		relayDone = make(chan error, 1)
		go func() { relayDone <- workerloop.Run(runCtx, 500*time.Millisecond, relay) }()
	}
	serverRead, relayRead := false, false
	var serverErr, relayErr error
	select {
	case <-ctx.Done():
	case serverErr = <-serverDone:
		serverRead = true
	case relayErr = <-relayDone:
		relayRead = true
	}
	// A relay which independently exits with cancellation is not a successful
	// shutdown. Cancellation is expected only after a parent stop, or after we
	// ask the still-running relay to stop because another component returned.
	relayStopExpected := ctx.Err() != nil || !relayRead
	cancelRun()
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	shutdownErr := server.Shutdown(shutdown)
	if shutdownErr != nil {
		_ = server.Close()
	}
	if !serverRead {
		serverErr = <-serverDone
	}
	if relayDone != nil && !relayRead {
		relayErr = <-relayDone
	}
	// errors.Is matches ANY branch of errors.Join. Clearing such a whole tree
	// would silently discard an independent listener/database/relay failure.
	if onlyControlStopError(serverErr, http.ErrServerClosed) {
		serverErr = nil
	}
	if relayStopExpected && onlyControlStopError(relayErr, context.Canceled) {
		relayErr = nil
	}
	return errors.Join(serverErr, relayErr, shutdownErr)
}

// Normalize only error trees whose EVERY leaf is the exact expected sentinel.
// Opaque Is implementations, empty unwraps, cycles and oversized error trees
// remain failures; do not discard useful error details to simplify exit status.
func onlyControlStopError(err, expected error) bool {
	budget := 128
	var visit func(error) bool
	visit = func(current error) bool {
		budget--
		if current == nil || budget < 0 {
			return false
		}
		switch wrapped := current.(type) {
		case interface{ Unwrap() []error }:
			children := wrapped.Unwrap()
			seen := false
			for _, child := range children {
				if child == nil {
					continue
				}
				seen = true
				if !visit(child) {
					return false
				}
			}
			return seen
		case interface{ Unwrap() error }:
			return visit(wrapped.Unwrap())
		default:
			return current == expected
		}
	}
	return visit(err)
}
