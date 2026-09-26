package main

import (
	"crypto/x509"
	"encoding/pem"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jiying2007/engineering-platform/internal/access"
)

func TestLocalPilotStackBootstrapGeneratesUsableMTLSAndPolicy(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash unavailable")
	}
	if _, err := exec.LookPath("openssl"); err != nil {
		t.Skip("openssl unavailable")
	}
	root := filepath.Join(t.TempDir(), "pilot")
	script := filepath.Join("..", "..", "examples", "pilots", "local-stack", "bootstrap.sh")
	digest := "sha256:" + strings.Repeat("a", 64)
	cmd := exec.Command(bash, script, root, digest, "worker/codex-pilot")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("bootstrap failed: %v: %s", err, out)
	}

	policyPath := filepath.Join(root, "operator", "access-policy.json")
	policyBytes, err := access.ReadConfiguration(policyPath, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := access.Decode(policyBytes); err != nil {
		t.Fatal(err)
	}

	serverCert := filepath.Join(root, "pki", "server.crt")
	serverKey := filepath.Join(root, "pki", "server.key")
	caCert := filepath.Join(root, "pki", "ca.crt")
	if _, err := access.LoadServerTLS(serverCert, serverKey, caCert); err != nil {
		t.Fatal(err)
	}
	requireMode(t, serverKey, 0o600)
	requireMode(t, filepath.Join(root, "pki", "ca.key"), 0o600)
	requireMode(t, policyPath, 0o600)

	ca := parsePilotCert(t, caCert)
	if !ca.IsCA || ca.KeyUsage&x509.KeyUsageCertSign == 0 {
		t.Fatalf("invalid pilot CA: %#v", ca)
	}
	roots := x509.NewCertPool()
	roots.AddCert(ca)

	server := parsePilotCert(t, serverCert)
	if len(server.DNSNames) != 1 || server.DNSNames[0] != "localhost" ||
		len(server.IPAddresses) != 1 || server.IPAddresses[0].String() != "127.0.0.1" ||
		!hasEKU(server, x509.ExtKeyUsageServerAuth) {
		t.Fatalf("unexpected server certificate identity: %#v", server)
	}
	if _, err := server.Verify(x509.VerifyOptions{
		Roots:     roots,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}); err != nil {
		t.Fatalf("server certificate does not verify: %v", err)
	}

	wantSubjects := map[string]string{
		"owner":          "urn:engineering-platform:operator:pilot-owner",
		"worker":         "urn:engineering-platform:worker:codex-pilot",
		"publisher":      "urn:engineering-platform:operator:pilot-publisher",
		"codex-evidence": "urn:engineering-platform:codex-evidence-importer",
		"git-evidence":   "urn:engineering-platform:git-evidence-importer",
		"ci-evidence":    "urn:engineering-platform:github-ci-importer",
		"verifier":       "urn:engineering-platform:verifier:pilot",
		"reviewer":       "urn:engineering-platform:reviewer:pilot",
		"closure":        "urn:engineering-platform:closure:pilot",
	}
	for slug, want := range wantSubjects {
		certPath := filepath.Join(root, "pki", slug+".crt")
		keyPath := filepath.Join(root, "pki", slug+".key")
		cert := parsePilotCert(t, certPath)
		if len(cert.URIs) != 1 || cert.URIs[0].String() != want || !hasEKU(cert, x509.ExtKeyUsageClientAuth) {
			t.Fatalf("%s client identity mismatch: %#v", slug, cert)
		}
		if _, err := cert.Verify(x509.VerifyOptions{
			Roots:     roots,
			KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		}); err != nil {
			t.Fatalf("%s certificate does not verify: %v", slug, err)
		}
		requireMode(t, keyPath, 0o600)
		requireMode(t, filepath.Join(root, "clients", slug+".env"), 0o600)
		envText, err := os.ReadFile(filepath.Join(root, "clients", slug+".env"))
		if err != nil {
			t.Fatal(err)
		}
		text := string(envText)
		if !strings.Contains(text, "CONTROL_ENDPOINT=https://127.0.0.1:18443") ||
			!strings.Contains(text, "CONTROL_CLIENT_CERT_FILE="+certPath) ||
			!strings.Contains(text, "CONTROL_CLIENT_KEY_FILE="+keyPath) ||
			!strings.Contains(text, "CONTROL_SERVER_CA_FILE="+caCert) {
			t.Fatalf("%s client environment is incomplete: %s", slug, text)
		}
	}

	controlEnv, err := os.ReadFile(filepath.Join(root, "operator", "control-plane.env"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"LISTEN_HOST=127.0.0.1",
		"PORT=18443",
		"DATABASE_URL=postgres://postgres:postgres@127.0.0.1:55432/engineering_platform?sslmode=disable",
		"CONTROL_TLS_CERT_FILE=" + serverCert,
		"CONTROL_TLS_KEY_FILE=" + serverKey,
		"CONTROL_CLIENT_CA_FILE=" + caCert,
		"CONTROL_AUTH_POLICY_FILE=" + policyPath,
	} {
		if !strings.Contains(string(controlEnv), want) {
			t.Fatalf("control-plane env missing %q: %s", want, controlEnv)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "secrets", "github-token")); !os.IsNotExist(err) {
		t.Fatal("bootstrap must not manufacture a publisher credential")
	}
}

func TestLocalPilotStackShellSyntax(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash unavailable")
	}
	for _, name := range []string{"bootstrap.sh", "postgres.sh", "status.sh"} {
		path := filepath.Join("..", "..", "examples", "pilots", "local-stack", name)
		cmd := exec.Command(bash, "-n", path)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s syntax: %v: %s", name, err, out)
		}
	}
}

func parsePilotCert(t *testing.T, path string) *x509.Certificate {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	block, rest := pem.Decode(data)
	if block == nil || block.Type != "CERTIFICATE" || len(rest) != 0 {
		t.Fatalf("invalid certificate PEM %s", path)
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	return cert
}

func hasEKU(cert *x509.Certificate, usage x509.ExtKeyUsage) bool {
	for _, value := range cert.ExtKeyUsage {
		if value == usage {
			return true
		}
	}
	return false
}

func requireMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != want {
		t.Fatalf("%s mode=%#o want=%#o", path, info.Mode().Perm(), want)
	}
}
