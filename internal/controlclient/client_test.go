package controlclient

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type pki struct {
	roots          *x509.CertPool
	server, client tls.Certificate
}

func newPKI(t *testing.T) pki {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test CA"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	der, err := x509.CreateCertificate(rand.Reader, ca, ca, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	ca, err = x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(ca)
	makeCert := func(client bool) tls.Certificate {
		k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		cert := &x509.Certificate{SerialNumber: big.NewInt(2), NotBefore: ca.NotBefore, NotAfter: ca.NotAfter, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}}
		if client {
			u, _ := url.Parse("urn:engineering-platform:worker:test")
			cert.SerialNumber = big.NewInt(3)
			cert.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
			cert.URIs = []*url.URL{u}
		}
		b, err := x509.CreateCertificate(rand.Reader, cert, ca, &k.PublicKey, key)
		if err != nil {
			t.Fatal(err)
		}
		return tls.Certificate{Certificate: [][]byte{b}, PrivateKey: k}
	}
	return pki{roots: roots, server: makeCert(false), client: makeCert(true)}
}
func (p pki) clientTLS() *tls.Config {
	return &tls.Config{RootCAs: p.roots, Certificates: []tls.Certificate{p.client}, MinVersion: tls.VersionTLS13}
}
func testServer(t *testing.T, p pki, h http.Handler) *httptest.Server {
	t.Helper()
	s := httptest.NewUnstartedServer(h)
	s.Config.ErrorLog = log.New(io.Discard, "", 0)
	s.TLS = &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{p.server}, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: p.roots}
	s.StartTLS()
	t.Cleanup(s.Close)
	return s
}
func TestRealMTLSAndNoAmbientProxy(t *testing.T) {
	p := newPKI(t)
	var calls atomic.Int32
	s := testServer(t, p, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.TLS == nil || len(r.TLS.VerifiedChains) == 0 {
			t.Error("unverified TLS")
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"ok":true}`)
	}))
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")
	c, err := New(s.URL, p.clientTLS())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	var out struct {
		OK bool `json:"ok"`
	}
	if err := c.Call(context.Background(), "GET", "/healthz", nil, &out); err != nil || !out.OK {
		t.Fatalf("mTLS request failed: %v", err)
	}
	if c.Subject() != "urn:engineering-platform:worker:test" || calls.Load() != 1 {
		t.Fatal("bad principal or calls")
	}
	foreign := newPKI(t)
	cfg := p.clientTLS()
	cfg.Certificates = []tls.Certificate{foreign.client}
	bad, err := New(s.URL, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer bad.Close()
	if _, err := bad.Raw(context.Background(), "GET", "/healthz", nil); err == nil {
		t.Fatal("foreign client CA accepted")
	}
}
func TestUnsafeClientConfigurationAndPathsFailClosed(t *testing.T) {
	p := newPKI(t)
	for _, endpoint := range []string{"http://127.0.0.1", "https://user:pass@example.com", "https://example.com/path", "https://example.com?x=1", "https://example.com/#fragment"} {
		if c, err := New(endpoint, p.clientTLS()); err == nil {
			c.Close()
			t.Fatalf("unsafe endpoint %q", endpoint)
		}
	}
	cfg := p.clientTLS()
	cfg.InsecureSkipVerify = true
	if _, err := New("https://example.com", cfg); err == nil {
		t.Fatal("insecure TLS accepted")
	}
	cfg = p.clientTLS()
	cfg.Certificates = nil
	if _, err := New("https://example.com", cfg); err == nil {
		t.Fatal("anonymous client accepted")
	}
	cfg = p.clientTLS()
	cfg.RootCAs = nil
	if _, err := New("https://example.com", cfg); err == nil {
		t.Fatal("ambient roots accepted")
	}
	cfg = p.clientTLS()
	cfg.Certificates = []tls.Certificate{p.server}
	if _, err := New("https://example.com", cfg); err == nil {
		t.Fatal("server-purpose certificate accepted")
	}
	c, err := New("https://example.com", p.clientTLS())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	for _, path := range []string{"//example.com/healthz", "/api/v1/../healthz", "/api/v1/%2e%2e", "/api/v1/x?secret=1", "/healthz#x", "/api/v1/x\\y"} {
		if _, err := c.Raw(context.Background(), "GET", path, nil); err == nil {
			t.Fatalf("unsafe path %q", path)
		}
	}
	if _, err := c.Raw(context.Background(), "POST", "/api/v1/runs", []byte(`[]`)); err == nil {
		t.Fatal("non-object accepted")
	}
}
func TestRedirectsErrorsAndOversizeResponses(t *testing.T) {
	p := newPKI(t)
	var calls atomic.Int32
	s := testServer(t, p, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/redirect":
			w.Header().Set("Location", "https://example.invalid/healthz")
			w.WriteHeader(307)
		case "/api/v1/denied":
			w.WriteHeader(403)
		case "/api/v1/large":
			io.WriteString(w, `{"data":"`+strings.Repeat("x", MaxResponse)+`"}`)
		case "/api/v1/malformed":
			io.WriteString(w, `}{`)
		default:
			io.WriteString(w, `{}`)
		}
	}))
	c, err := New(s.URL, p.clientTLS())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	for _, tc := range []struct {
		path   string
		status int
	}{{"redirect", 307}, {"denied", 403}} {
		_, err := c.Raw(context.Background(), "POST", "/api/v1/"+tc.path, []byte(`{}`))
		var he *HTTPError
		if !errors.As(err, &he) || he.Status != tc.status {
			t.Fatalf("wrong HTTP error: %v", err)
		}
	}
	if calls.Load() != 2 {
		t.Fatal("POST or redirect retried")
	}
	for _, path := range []string{"large", "malformed"} {
		if _, err := c.Raw(context.Background(), "GET", "/api/v1/"+path, nil); err == nil {
			t.Fatalf("bad response %s", path)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Raw(ctx, "GET", "/healthz", nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("lost cancellation: %v", err)
	}
}
func TestTLSKeyFileBounds(t *testing.T) {
	dir := t.TempDir()
	name := filepath.Join(dir, "key")
	if err := os.WriteFile(name, []byte("test"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := readTLSFile(name, true); err == nil {
		t.Fatal("readable private key accepted")
	}
	if err := os.Chmod(name, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readTLSFile(name, true); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(dir, "alias")
	if err := os.Symlink(name, alias); err != nil {
		t.Fatal(err)
	}
	if _, err := readTLSFile(alias, true); err == nil {
		t.Fatal("symlink private key accepted")
	}
	if err := os.WriteFile(name, []byte(strings.Repeat("x", MaxRequest+1)), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readTLSFile(name, true); err == nil {
		t.Fatal("unbounded key file accepted")
	}
}
func TestTLSConstructionDoesNotInheritOverridesOrDERAliases(t *testing.T) {
	p := newPKI(t)
	s := testServer(t, p, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{}`)
	}))
	cfg := p.clientTLS()
	cfg.ServerName = "wrong.example.invalid"
	cfg.Time = func() time.Time { return time.Unix(1, 0) }
	cfg.KeyLogWriter = io.Discard
	c, err := New(s.URL, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	derived := c.http.Transport.(*http.Transport).TLSClientConfig
	if derived.ServerName != "127.0.0.1" || derived.Time != nil || derived.KeyLogWriter != nil {
		t.Fatal("inherited a TLS override")
	}
	cfg.Certificates[0].Certificate[0][0] ^= 1
	if _, err := c.Raw(context.Background(), "GET", "/healthz", nil); err != nil {
		t.Fatalf("DER was not copied: %v", err)
	}
}
