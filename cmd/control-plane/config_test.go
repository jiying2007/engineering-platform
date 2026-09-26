package main

import (
	"crypto/tls"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/access"
	"github.com/jiying2007/engineering-platform/internal/store"
	"github.com/jiying2007/engineering-platform/internal/testsupport"
)

func TestStartupFailsBeforeImplicitInsecureOrDurableFallback(t *testing.T) {
	for _, env := range []map[string]string{
		{}, {"DATABASE_URL": "not-opened"},
		{"INSECURE_DEV": "yes"}, {"INSECURE_DEV": "1", "LISTEN_HOST": "0.0.0.0"},
		{"INSECURE_DEV": "1", "LISTEN_HOST": "localhost"},
		{"INSECURE_DEV": "1", "CONTROL_AUTH_POLICY_FILE": "do-not-ignore"},
		{"INSECURE_DEV": "1", "DATABASE_URL": "do-not-open"},
		{"INSECURE_DEV": "1", "AUTO_MIGRATE": "1"},
		{"INSECURE_DEV": "1", "GITHUB_PUBLISHER_CONFIG_FILE": "do-not-ignore"},
	} {
		if _, err := loadConfiguration(func(key string) string { return env[key] }); err == nil {
			t.Fatalf("unsafe startup accepted: %v", env)
		}
	}
	env := map[string]string{"INSECURE_DEV": "1"}
	config, err := loadConfiguration(func(key string) string { return env[key] })
	if err != nil || config.address != "127.0.0.1:8080" || !config.development {
		t.Fatalf("explicit local fixture failed: %v", err)
	}
}

func TestCommandAssemblyUsesRealMTLSAndPolicy(t *testing.T) {
	pki := testsupport.NewPKI(t)
	dir := t.TempDir()
	subject := "urn:engineering-platform:operator:read-only"
	policy, err := json.Marshal(access.Document{Version: 1, Principals: []access.PrincipalSpec{{Subject: subject, Scope: "platform", Capabilities: []string{access.Read}}}})
	if err != nil {
		t.Fatal(err)
	}
	env := map[string]string{"DATABASE_URL": "provided-by-operator", "LISTEN_HOST": "127.0.0.1", "PORT": "0"}
	for key, data := range map[string][]byte{"CONTROL_TLS_CERT_FILE": pki.ServerCertPEM, "CONTROL_TLS_KEY_FILE": pki.ServerKeyPEM, "CONTROL_CLIENT_CA_FILE": pki.CAPEM, "CONTROL_AUTH_POLICY_FILE": policy} {
		path := filepath.Join(dir, key)
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		env[key] = path
	}
	config, err := loadConfiguration(func(key string) string { return env[key] })
	if err != nil {
		t.Fatal(err)
	}
	// Backend is deliberately injected for command-assembly testing; the actual
	// startup path requires and opens PostgreSQL before assembling this handler.
	server, err := assembleServer(config, store.NewMemory())
	if err != nil {
		t.Fatal(err)
	}
	if server.TLSConfig.ClientAuth != tls.RequireAndVerifyClientCert || server.ReadTimeout <= 0 || server.WriteTimeout <= 0 {
		t.Fatal("unbounded or unauthenticated server")
	}
	testServer := httptest.NewUnstartedServer(server.Handler)
	testServer.Config = server
	testServer.Config.ErrorLog = log.New(io.Discard, "", 0)
	testServer.TLS = config.tls.Clone()
	testServer.StartTLS()
	defer testServer.Close()
	client := pki.Client(t, subject)
	response, err := client.Get(testServer.URL + "/api/v1/recovery")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("command did not wire authenticated API: %d", response.StatusCode)
	}
	response, err = client.Post(testServer.URL+"/api/v1/recovery/begin", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("read-only principal can mutate: %d", response.StatusCode)
	}
	transport := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pki.Roots, MinVersion: tls.VersionTLS13}}
	defer transport.CloseIdleConnections()
	if response, err = (&http.Client{Transport: transport, Timeout: 5 * time.Second}).Get(testServer.URL + "/api/v1/recovery"); err == nil {
		response.Body.Close()
		t.Fatal("anonymous TLS accepted by command assembly")
	}
}
