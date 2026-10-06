package githubpublish

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/testsupport"
)

func healthRemoteConfig(t *testing.T, pki *testsupport.PKI, endpoint, subject string) RemoteConfiguration {
	t.Helper()
	dir := t.TempDir()
	cert := pki.ClientCertificate(t, subject)
	keyDER, err := x509.MarshalPKCS8PrivateKey(cert.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{
		"client.crt": pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Certificate[0]}),
		"client.key": pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}),
		"ca.crt":     pki.CAPEM,
	}
	for name, raw := range files {
		if err := os.WriteFile(filepath.Join(dir, name), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	return RemoteConfiguration{Version: 1, Endpoint: endpoint, ClientCertFile: filepath.Join(dir, "client.crt"), ClientKeyFile: filepath.Join(dir, "client.key"), ServerCAFile: filepath.Join(dir, "ca.crt")}
}

func TestProbeRemoteHealthUsesAuthenticatedPublisherWithoutUpstreamCall(t *testing.T) {
	service, _ := publisherServiceFixture(t)
	pki := testsupport.NewPKI(t)
	server := httptest.NewUnstartedServer(service.Handler())
	server.TLS = pki.ServerTLS()
	server.StartTLS()
	defer server.Close()

	config := healthRemoteConfig(t, pki, server.URL, service.controlSubject)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, err := ProbeRemoteHealth(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	configDigest, err := canonical.Digest(config)
	if err != nil {
		t.Fatal(err)
	}
	remote := service.remote.(*serviceRemote)
	if result.Version != 1 || result.Status != "PUBLISHER_ENDPOINT_OBSERVED" ||
		result.Service != "engineering-github-publisher" || result.ConfigurationDigest != configDigest || result.ObservedAt.IsZero() ||
		!result.EndpointObserved || result.UpstreamObserved || result.CapacityObserved ||
		result.PublicationAuthorized || result.ExecutionAuthorized || result.ProductionQualified ||
		remote.publishCalls != 0 || remote.observeCalls != 0 {
		t.Fatal("health probe promoted authority or contacted upstream", result, remote)
	}

	wrong := healthRemoteConfig(t, pki, server.URL, "urn:engineering-platform:control:wrong-publisher-client")
	if _, err := ProbeRemoteHealth(ctx, wrong); err == nil || !strings.Contains(err.Error(), "HTTP 401") {
		t.Fatal("wrong client identity was accepted", err)
	}
}

func TestProbeRemoteHealthRejectsAmbiguousOrRedirectedResponses(t *testing.T) {
	for _, mode := range []string{"wrong-service", "extra-field", "encoding", "redirect", "large"} {
		t.Run(mode, func(t *testing.T) {
			pki := testsupport.NewPKI(t)
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/healthz" || r.Header.Get("Accept") != "application/json" {
					t.Error("unexpected health request")
				}
				switch mode {
				case "redirect":
					http.Redirect(w, r, "/other", http.StatusFound)
					return
				case "encoding":
					w.Header().Set("Content-Encoding", "gzip")
				}
				w.Header().Set("Content-Type", "application/json")
				switch mode {
				case "wrong-service":
					_, _ = w.Write([]byte(`{"status":"ok","service":"other"}`))
				case "extra-field":
					_, _ = w.Write([]byte(`{"status":"ok","service":"engineering-github-publisher","ready":true}`))
				case "large":
					_, _ = w.Write([]byte(`{"status":"ok","service":"engineering-github-publisher","padding":"` + strings.Repeat("x", 5000) + `"}`))
				default:
					_, _ = w.Write([]byte(`{"status":"ok","service":"engineering-github-publisher"}`))
				}
			})
			server := httptest.NewUnstartedServer(handler)
			server.TLS = pki.ServerTLS()
			server.StartTLS()
			defer server.Close()
			config := healthRemoteConfig(t, pki, server.URL, "urn:engineering-platform:control:publisher-client")
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if _, err := ProbeRemoteHealth(ctx, config); err == nil {
				t.Fatal("invalid publisher health response accepted", mode)
			}
		})
	}
}

func TestProbeRemoteHealthHonorsCancellationBeforeNetwork(t *testing.T) {
	pki := testsupport.NewPKI(t)
	called := false
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	server.TLS = pki.ServerTLS()
	server.StartTLS()
	defer server.Close()
	config := healthRemoteConfig(t, pki, server.URL, "urn:engineering-platform:control:publisher-client")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ProbeRemoteHealth(ctx, config); err == nil || called {
		t.Fatal("cancelled health probe contacted endpoint", err, called)
	}
}
