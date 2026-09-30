package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jiying2007/engineering-platform/internal/access"
	"github.com/jiying2007/engineering-platform/internal/githubpublish"
)

func main() {
	if err := serve(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func serve() error {
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
	done := make(chan error, 1)
	go func() { done <- server.ServeTLS(listener, "", "") }()
	select {
	case <-ctx.Done():
	case err := <-done:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdown); err != nil {
		_ = server.Close()
		return err
	}
	err = <-done
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
