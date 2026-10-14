package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jiying2007/engineering-platform/internal/access"
	"github.com/jiying2007/engineering-platform/internal/githubpublish"
	"github.com/jiying2007/engineering-platform/internal/servicelifecycle"
	"github.com/jiying2007/engineering-platform/internal/servicenotify"
)

func main() {
	if err := serve(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func serve(args []string) error {
	// Never silently start a service for an unsupported operation/argument.
	if len(args) != 0 {
		return fmt.Errorf("publisher-service accepts no command-line arguments")
	}
	notifier, err := servicenotify.FromEnvironment()
	if err != nil {
		return err
	}
	configFile := os.Getenv("PUBLISHER_CONFIG_FILE")
	certFile := os.Getenv("PUBLISHER_TLS_CERT_FILE")
	keyFile := os.Getenv("PUBLISHER_TLS_KEY_FILE")
	clientCAFile := os.Getenv("PUBLISHER_CLIENT_CA_FILE")
	controlSubject := os.Getenv("PUBLISHER_CONTROL_SUBJECT")
	if configFile == "" || certFile == "" || keyFile == "" || clientCAFile == "" || controlSubject == "" {
		return fmt.Errorf("publisher service requires config, TLS server identity, client CA and exact control subject")
	}
	tlsConfig, err := access.LoadServerTLS(certFile, keyFile, clientCAFile)
	if err != nil {
		return err
	}
	service, err := githubpublish.LoadRemoteService(configFile, controlSubject)
	if err != nil {
		return err
	}
	host := os.Getenv("LISTEN_HOST")
	if host == "" {
		host = "127.0.0.1"
	}
	if host != "127.0.0.1" && host != "::1" {
		return fmt.Errorf("publisher service v1 requires literal loopback LISTEN_HOST")
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "18444"
	}
	listener, err := net.Listen("tcp", net.JoinHostPort(host, port))
	if err != nil {
		return err
	}
	defer listener.Close()
	server := &http.Server{
		Addr: listener.Addr().String(), Handler: service.Handler(), TLSConfig: tlsConfig,
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 20 * time.Second,
		WriteTimeout: 45 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 32 << 10,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	log.Printf("publisher service listening on %s; scope=authenticated-endpoint-not-provider-readiness", listener.Addr())
	return runPublisherServer(ctx, server, notifier.Listener(ctx, listener))
}

// The entrypoint and loopback mTLS tests share the same serving lifecycle.
// Stop request-scoped work before draining; independently authorized settlement
// still uses its existing bounded Context and must finish before graceful exit.
func runPublisherServer(ctx context.Context, server *http.Server, listener net.Listener) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	runCtx, cancelRun := context.WithCancel(ctx)
	defer cancelRun()
	if err := servicelifecycle.BindHTTP(runCtx, server); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- server.ServeTLS(listener, "", "") }()
	var serverErr error
	serverRead := false
	select {
	case <-ctx.Done():
	case serverErr = <-done:
		serverRead = true
	}
	// Listener failure must also revoke active request contexts and drain them;
	// returning directly from the Serve case could leave handlers running.
	cancelRun()
	shutdown, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	shutdownErr := server.Shutdown(shutdown)
	var closeErr error
	if shutdownErr != nil {
		closeErr = server.Close()
	}
	if !serverRead {
		serverErr = <-done
	}
	// A mixed listener error must not disappear because one joined leaf is the
	// normal HTTP stop sentinel. Reuse the original Control classifier.
	if servicelifecycle.OnlyStopError(serverErr, http.ErrServerClosed) {
		serverErr = nil
	}
	return errors.Join(serverErr, shutdownErr, closeErr)
}
