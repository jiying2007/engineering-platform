package main

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"time"

	"github.com/jiying2007/engineering-platform/internal/access"
	"github.com/jiying2007/engineering-platform/internal/action"
	"github.com/jiying2007/engineering-platform/internal/api"
	"github.com/jiying2007/engineering-platform/internal/gateway"
	"github.com/jiying2007/engineering-platform/internal/githubpublish"
	corestore "github.com/jiying2007/engineering-platform/internal/store"
)

type configuration struct {
	address     string
	databaseURL string
	autoMigrate bool
	development bool
	tls         *tls.Config
	policy          *access.Policy
	publisherConfig string
}

// Resolve security before opening a DB or running migrations. There is no
// implicit plaintext, anonymous identity or memory-persistence fallback.
func loadConfiguration(env func(string) string) (configuration, error) {
	c := configuration{address: controlPlaneAddress(env("LISTEN_HOST"), env("PORT")), databaseURL: env("DATABASE_URL"), publisherConfig: env("GITHUB_PUBLISHER_CONFIG_FILE")}
	mode, migrate := env("INSECURE_DEV"), env("AUTO_MIGRATE")
	if mode != "" && mode != "0" && mode != "1" {
		return c, fmt.Errorf("INSECURE_DEV must be 0 or 1")
	}
	if migrate != "" && migrate != "0" && migrate != "1" {
		return c, fmt.Errorf("AUTO_MIGRATE must be 0 or 1")
	}
	c.autoMigrate = migrate == "1"
	c.development = mode == "1"
	cert, key, ca, policy := env("CONTROL_TLS_CERT_FILE"), env("CONTROL_TLS_KEY_FILE"), env("CONTROL_CLIENT_CA_FILE"), env("CONTROL_AUTH_POLICY_FILE")
	if c.development {
		host := env("LISTEN_HOST")
		if host != "" && host != "127.0.0.1" && host != "::1" {
			return c, fmt.Errorf("INSECURE_DEV requires a literal loopback host")
		}
		if cert != "" || key != "" || ca != "" || policy != "" || c.publisherConfig != "" {
			return c, fmt.Errorf("development mode cannot ignore supplied TLS/access/publication configuration")
		}
		// Keep unauthenticated fixture writes away from any durable backend.
		if c.databaseURL != "" || c.autoMigrate {
			return c, fmt.Errorf("INSECURE_DEV is memory-only; database and migrations require mTLS mode")
		}
		return c, nil
	}
	if cert == "" || key == "" || ca == "" || policy == "" || c.databaseURL == "" {
		return c, fmt.Errorf("mTLS mode requires server certificate/key, client CA, access policy and DATABASE_URL")
	}
	data, err := access.ReadConfiguration(policy, false)
	if err != nil {
		return c, err
	}
	c.policy, err = access.Decode(data)
	if err != nil {
		return c, err
	}
	c.tls, err = access.LoadServerTLS(cert, key, ca)
	return c, err
}

func assembleServer(c configuration, backend corestore.Store) (*http.Server, error) {
	if backend == nil {
		return nil, fmt.Errorf("backend required")
	}
	var handler http.Handler
	if c.development {
		handler = api.NewServer(backend).Handler()
	} else {
		if c.tls == nil {
			return nil, fmt.Errorf("verified TLS transport required")
		}
		var err error
		options := api.AuthenticatedOptions{}
		if gate, ok := backend.(api.RecoveryCompletionGate); ok {
			options.RecoveryCompletion = gate
		}
		var actions api.ActionGateway
		if c.publisherConfig != "" {
			state, ok := backend.(githubpublish.State)
			if !ok {
				return nil, fmt.Errorf("GitHub publisher requires Core-bound Codex state")
			}
			repository, ok := backend.(action.Repository)
			if !ok {
				return nil, fmt.Errorf("GitHub publisher requires durable action repository")
			}
			provider, loadErr := githubpublish.Load(c.publisherConfig, state)
			if loadErr != nil {
				return nil, loadErr
			}
			authority := gateway.NewAuthority(backend)
			actions = action.NewService(authority, authority, provider, repository)
		}
		handler, err = api.NewAuthenticatedHandler(backend, actions, c.policy, options)
		if err != nil {
			return nil, err
		}
	}
	return &http.Server{Addr: c.address, Handler: handler, TLSConfig: c.tls, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 32 << 10}, nil
}
