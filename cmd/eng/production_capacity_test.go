package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/access"
	"github.com/jiying2007/engineering-platform/internal/api"
	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/controlclient"
	"github.com/jiying2007/engineering-platform/internal/production"
	"github.com/jiying2007/engineering-platform/internal/store"
	"github.com/jiying2007/engineering-platform/internal/testsupport"
)

func capacityArgs(source production.CapacitySource) []string {
	return []string{"--server-cert-digest", source.ServerCertificateDigest, "--access-policy-digest", source.AccessPolicyDigest, "--source-commit", source.SourceCommit, "--instance", source.InstanceID}
}

func TestCapacityCLIRequiresEveryExternalSourcePin(t *testing.T) {
	source := production.CapacitySource{ServerCertificateDigest: canonical.BytesDigest([]byte("server")), AccessPolicyDigest: canonical.BytesDigest([]byte("policy")), SourceCommit: strings.Repeat("a", 40), InstanceID: strings.Repeat("b", 32)}
	good := capacityArgs(source)
	if got, err := parseProductionCapacity(good); err != nil || got != source {
		t.Fatal(got, err)
	}
	bad := [][]string{{}}
	for i := 0; i < len(good); i += 2 {
		bad = append(bad, append(append([]string{}, good[:i]...), good[i+2:]...))
	}
	for _, extra := range [][]string{{"--instance", source.InstanceID}, {"--source-commit=" + source.SourceCommit}, {"--require-ready"}, {"--repair"}, {"--identity", "other"}, {"unexpected"}} {
		bad = append(bad, append(append([]string{}, good...), extra...))
	}
	for _, args := range bad {
		if _, err := parseProductionCapacity(args); err == nil {
			t.Fatal("missing, ambiguous or authority-expanding option accepted", args)
		}
	}
}

type capacityCLIStore struct {
	*store.Memory
	mode    string
	entered chan struct{}
	reads   atomic.Int64
}

func (s *capacityCLIStore) ReadOperationalCapacity(ctx context.Context, grants []production.WorkerCapacityGrant) (production.CapacityFacts, error) {
	s.reads.Add(1)
	if s.mode == "cancel" {
		close(s.entered)
		<-ctx.Done()
		return production.CapacityFacts{}, ctx.Err()
	}
	now := time.Now().UTC()
	if s.mode == "stale" {
		now = now.Add(-time.Minute)
	}
	authority, err := production.EvaluateSnapshot(production.Snapshot{Version: production.OperationalStatusVersion, CapturedAt: now, RecoveryMode: "NORMAL", WorkerPolls: []production.WorkerPollObservation{}})
	if err != nil {
		return production.CapacityFacts{}, err
	}
	return production.CapacityFacts{Authority: authority, CapturedAt: now, RecoveryMode: "NORMAL", Workers: []production.WorkerCapacityFact{}, Queues: []production.QueueCapacityFact{}, Pool: production.PoolCapacityFact{CapturedAt: now, Maximum: 4, Total: 1, Idle: 1}}, nil
}

// The real executable traverses the unchanged Core mTLS/capability middleware.
// Capacity facts come from an explicit inert test Store; this is not a host or
// Worker capacity qualification. Real PostgreSQL projection tests are separate.
func TestCapacityCLIThroughAuthenticatedCore(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	binary := filepath.Join(t.TempDir(), "eng")
	build := exec.CommandContext(ctx, filepath.Join(runtime.GOROOT(), "bin", "go"), "build", "-trimpath", "-o", binary, ".")
	if raw, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v %s", err, raw)
	}
	for _, mode := range []string{"complete-unready", "instance-drift", "policy-drift", "source-drift", "wrong-peer", "read-denied", "missing-instance", "forbidden-repair", "stale", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			pki := testsupport.NewPKI(t)
			readerSubject, deniedSubject := "urn:engineering-platform:operator:capacity", "urn:engineering-platform:operator:no-capacity"
			policyRaw, err := json.Marshal(access.Document{Version: 1, Principals: []access.PrincipalSpec{
				{Subject: readerSubject, Scope: "platform", Capabilities: []string{access.Read}},
				{Subject: deniedSubject, Scope: "platform", Capabilities: []string{access.WorkCreate}},
			}})
			if err != nil {
				t.Fatal(err)
			}
			policy, err := access.Decode(policyRaw)
			if err != nil {
				t.Fatal(err)
			}
			source := production.CapacitySource{ServerCertificateDigest: canonical.BytesDigest(pki.ServerCertificate.Certificate[0]), AccessPolicyDigest: canonical.BytesDigest(policyRaw), SourceCommit: strings.Repeat("a", 40), InstanceID: strings.Repeat("b", 32)}
			backend := &capacityCLIStore{Memory: store.NewMemory(), mode: mode, entered: make(chan struct{})}
			handler, err := api.NewAuthenticatedHandler(backend, nil, policy, api.AuthenticatedOptions{OperationalCapacity: &api.OperationalCapacityOptions{Source: source, Workers: []production.WorkerCapacityGrant{}}})
			if err != nil {
				t.Fatal(err)
			}
			var requests atomic.Int64
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				if r.Method != http.MethodGet || r.URL.Path != "/api/v1/operations/capacity" {
					t.Error("capacity command reached another route or mutated Core")
				}
				handler.ServeHTTP(w, r)
			}))
			server.TLS = pki.ServerTLS()
			server.StartTLS()
			defer server.Close()
			expected := source
			switch mode {
			case "instance-drift":
				expected.InstanceID = strings.Repeat("c", 32)
			case "policy-drift":
				expected.AccessPolicyDigest = canonical.BytesDigest([]byte("other policy"))
			case "source-drift":
				expected.SourceCommit = strings.Repeat("c", 40)
			case "wrong-peer":
				expected.ServerCertificateDigest = canonical.BytesDigest([]byte("other peer"))
			}
			args := append([]string{"production-capacity"}, capacityArgs(expected)...)
			if mode == "missing-instance" {
				args = args[:len(args)-2]
			}
			if mode == "forbidden-repair" {
				args = append(args, "--repair")
			}
			subject := readerSubject
			if mode == "read-denied" {
				subject = deniedSubject
			}
			cert := pki.ClientCertificate(t, subject)
			key, err := x509.MarshalPKCS8PrivateKey(cert.PrivateKey)
			if err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			environment := append(os.Environ(), "CONTROL_ENDPOINT="+server.URL)
			for name, data := range map[string][]byte{
				"CONTROL_CLIENT_CERT_FILE": pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Certificate[0]}),
				"CONTROL_CLIENT_KEY_FILE":  pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: key}),
				"CONTROL_SERVER_CA_FILE":   pki.CAPEM,
			} {
				path := filepath.Join(dir, name)
				if err := os.WriteFile(path, data, 0600); err != nil {
					t.Fatal(err)
				}
				environment = append(environment, name+"="+path)
			}
			command := exec.CommandContext(ctx, binary, args...)
			command.Env = environment
			var stdout, stderr bytes.Buffer
			command.Stdout, command.Stderr = &stdout, &stderr
			if err := command.Start(); err != nil {
				t.Fatal(err)
			}
			if mode == "cancel" {
				select {
				case <-backend.entered:
				case <-time.After(3 * time.Second):
					command.Process.Kill()
					command.Wait()
					t.Fatal("CLI read did not reach backend")
				}
				if err := command.Process.Signal(syscall.SIGTERM); err != nil {
					t.Fatal(err)
				}
			}
			err = command.Wait()
			wantCalls := int64(1)
			if mode == "missing-instance" || mode == "forbidden-repair" {
				wantCalls = 0
			}
			if requests.Load() != wantCalls {
				t.Fatal("CLI did not make the expected single read", requests.Load(), wantCalls)
			}
			if mode != "complete-unready" {
				if err == nil || stdout.Len() != 0 {
					t.Fatalf("failed source read emitted success: %v %s / %s", err, stdout.Bytes(), stderr.Bytes())
				}
				return
			}
			if err != nil {
				t.Fatalf("CLI failed: %v %s", err, stderr.Bytes())
			}
			var result production.CapacityObservation
			decoder := json.NewDecoder(&stdout)
			if err := decoder.Decode(&result); err != nil {
				t.Fatal(err)
			}
			if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
				t.Fatal("extra output after one observation", err)
			}
			if result.Ready || result.ProductionQualified || result.ExecutionAuthorized || result.WorkerProcessCapacityObserved || result.PublisherCapacityObserved || result.ServiceReadiness != production.ServiceReadinessUnobserved || result.ExpectedSource != source || result.PoolReservationHeadroom != 4 || result.WorkerReservationHeadroom != 0 || result.ResponseDigest != canonical.BytesDigest(result.ResponseBytes) {
				t.Fatal("capacity result overclaimed or lost retained source", result)
			}
		})
	}
}

type capacityShortWriter struct{}

func (capacityShortWriter) Write(p []byte) (int, error) { return len(p) - 1, nil }

func TestCapacityCLIOutputShortWriteFails(t *testing.T) {
	pki := testsupport.NewPKI(t)
	source := production.CapacitySource{ServerCertificateDigest: canonical.BytesDigest(pki.ServerCertificate.Certificate[0]), AccessPolicyDigest: canonical.BytesDigest([]byte("test-policy")), SourceCommit: strings.Repeat("a", 40), InstanceID: strings.Repeat("b", 32)}
	var requests atomic.Int64
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		backend := capacityCLIStore{Memory: store.NewMemory()}
		facts, err := backend.ReadOperationalCapacity(r.Context(), nil)
		if err != nil {
			t.Error(err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		json.NewEncoder(w).Encode(production.CapacityResponse{Version: production.CapacityVersion, Scope: production.CapacityScope, Source: source, GeneratedAt: time.Now().UTC(), Facts: facts})
	}))
	server.TLS = pki.ServerTLS()
	server.StartTLS()
	defer server.Close()
	client, err := controlclient.New(server.URL, &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: pki.Roots, Certificates: []tls.Certificate{pki.ClientCertificate(t, "urn:engineering-platform:operator:capacity")}})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if err := writeProductionCapacity(context.Background(), client, source, capacityShortWriter{}); !errors.Is(err, io.ErrShortWrite) {
		t.Fatal("short stdout write returned success", err)
	}
	if requests.Load() != 1 {
		t.Fatal("failed output caused another GET")
	}
}
