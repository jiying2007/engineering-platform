package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/access"
	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/controlclient"
	"github.com/jiying2007/engineering-platform/internal/distribution"
	"github.com/jiying2007/engineering-platform/internal/production"
	"github.com/jiying2007/engineering-platform/internal/store"
	"github.com/jiying2007/engineering-platform/internal/testsupport"
)

func capacityBuildInfo() *debug.BuildInfo {
	// Explicit test metadata exercises admission rules; it never qualifies the
	// test executable or bypasses debug.ReadBuildInfo in the real startup path.
	return &debug.BuildInfo{Path: distribution.Module + "/cmd/control-plane", Settings: []debug.BuildSetting{
		{Key: "GOOS", Value: "linux"}, {Key: "GOARCH", Value: "amd64"}, {Key: "vcs", Value: "git"},
		{Key: "vcs.revision", Value: strings.Repeat("a", 40)}, {Key: "vcs.modified", Value: "false"},
	}}
}

func TestCapacitySourceRequiresCleanActualBuildMetadata(t *testing.T) {
	pki := testsupport.NewPKI(t)
	raw := []byte(`{"version":1,"principals":[{"subject":"urn:engineering-platform:operator","scope":"platform","capabilities":["core:read"]}]}`)
	policy, err := access.Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	first, err := capacityFromLoaded(policy, raw, pki.ServerTLS(), capacityBuildInfo())
	if err != nil {
		t.Fatal(err)
	}
	second, err := capacityFromLoaded(policy, raw, pki.ServerTLS(), capacityBuildInfo())
	if err != nil {
		t.Fatal(err)
	}
	if first.Source.InstanceID == second.Source.InstanceID || first.Source.AccessPolicyDigest != canonical.BytesDigest(raw) || first.Source.ServerCertificateDigest != canonical.BytesDigest(pki.ServerCertificate.Certificate[0]) || len(first.Workers) != 0 {
		t.Fatal("startup source identity was guessed or reused", first, second)
	}
	for name, mutate := range map[string]func(*debug.BuildInfo){
		"dirty":            func(b *debug.BuildInfo) { b.Settings[4].Value = "true" },
		"wrong-role":       func(b *debug.BuildInfo) { b.Path = distribution.Module + "/cmd/worker" },
		"missing-revision": func(b *debug.BuildInfo) { b.Settings[3].Value = "" },
		"duplicate-proof": func(b *debug.BuildInfo) {
			b.Settings = append(b.Settings, debug.BuildSetting{Key: "vcs.modified", Value: "false"})
		},
		"other-vcs":    func(b *debug.BuildInfo) { b.Settings[2].Value = "unknown" },
		"other-target": func(b *debug.BuildInfo) { b.Settings[0].Value = "other" },
	} {
		t.Run(name, func(t *testing.T) {
			info := capacityBuildInfo()
			mutate(info)
			if source, err := capacityFromLoaded(policy, raw, pki.ServerTLS(), info); err == nil || source != nil {
				t.Fatal("unidentified executable acquired capacity source identity")
			}
		})
	}
	if source, err := capacityFromLoaded(policy, raw, pki.ServerTLS(), nil); err == nil || source != nil {
		t.Fatal("missing build info accepted")
	}
}

type capacityAssemblyStore struct{ *store.Memory }

func (s *capacityAssemblyStore) ReadOperationalCapacity(_ context.Context, grants []production.WorkerCapacityGrant) (production.CapacityFacts, error) {
	now := time.Now().UTC()
	authority, err := production.EvaluateSnapshot(production.Snapshot{Version: production.OperationalStatusVersion, CapturedAt: now, RecoveryMode: "NORMAL", WorkerPolls: []production.WorkerPollObservation{}})
	if err != nil {
		return production.CapacityFacts{}, err
	}
	facts := production.CapacityFacts{Authority: authority, CapturedAt: now, RecoveryMode: "NORMAL", Workers: []production.WorkerCapacityFact{}, Queues: []production.QueueCapacityFact{}, Pool: production.PoolCapacityFact{CapturedAt: now, Maximum: 4, Total: 1, Idle: 1}}
	for _, grant := range grants {
		facts.Workers = append(facts.Workers, production.WorkerCapacityFact{Grant: grant})
	}
	profiles, err := production.CapacityProfiles(grants)
	if err != nil {
		return production.CapacityFacts{}, err
	}
	for _, profile := range profiles {
		facts.Queues = append(facts.Queues, production.QueueCapacityFact{Profile: profile})
	}
	return facts, nil
}

func TestCapacityLoadedConfigurationBytesRemainBound(t *testing.T) {
	pki := testsupport.NewPKI(t)
	subject := "urn:engineering-platform:operator:capacity"
	policyRaw, err := json.MarshalIndent(access.Document{Version: 1, Principals: []access.PrincipalSpec{{Subject: subject, Scope: "platform", Capabilities: []string{access.Read}}}}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	env := map[string]string{"DATABASE_URL": "injected-test-only", "PORT": "0", "LISTEN_HOST": "127.0.0.1"}
	for key, raw := range map[string][]byte{"CONTROL_TLS_CERT_FILE": pki.ServerCertPEM, "CONTROL_TLS_KEY_FILE": pki.ServerKeyPEM, "CONTROL_CLIENT_CA_FILE": pki.CAPEM, "CONTROL_AUTH_POLICY_FILE": policyRaw} {
		name := filepath.Join(dir, key)
		if err := os.WriteFile(name, raw, 0600); err != nil {
			t.Fatal(err)
		}
		env[key] = name
	}
	cfg, err := loadConfiguration(func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	// Unidentified test builds must not prevent ordinary startup/read APIs.
	if cfg.capacity != nil {
		t.Fatal("test binary claimed a clean production source")
	}
	backend := &capacityAssemblyStore{Memory: store.NewMemory()}
	unidentified, err := assembleServer(cfg, backend)
	if err != nil {
		t.Fatal("new diagnostic blocked existing startup", err)
	}
	fixture := httptest.NewUnstartedServer(unidentified.Handler)
	fixture.TLS = cfg.tls.Clone()
	fixture.Config.ErrorLog = log.New(io.Discard, "", 0)
	fixture.StartTLS()
	defer fixture.Close()
	client := pki.Client(t, subject)
	for path, want := range map[string]int{"/api/v1/recovery": http.StatusOK, "/api/v1/operations/capacity": http.StatusServiceUnavailable} {
		response, err := client.Get(fixture.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != want {
			t.Fatal("wrong unidentified-build boundary", path, response.StatusCode)
		}
	}
	cfg.capacity, err = capacityFromLoaded(cfg.policy, policyRaw, cfg.tls, capacityBuildInfo())
	if err != nil {
		t.Fatal(err)
	}
	expected := cfg.capacity.Source
	// Replace every path after loading. Capacity and mTLS must continue to bind
	// to the same already verified bytes as the immutable access policy.
	for _, key := range []string{"CONTROL_TLS_CERT_FILE", "CONTROL_TLS_KEY_FILE", "CONTROL_CLIENT_CA_FILE", "CONTROL_AUTH_POLICY_FILE"} {
		if err := os.WriteFile(env[key], []byte("replacement must never be reread"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	server, err := assembleServer(cfg, backend)
	if err != nil {
		t.Fatal("assembly reread replaced configuration", err)
	}
	fixture2 := httptest.NewUnstartedServer(server.Handler)
	fixture2.TLS = cfg.tls.Clone()
	fixture2.Config.ErrorLog = log.New(io.Discard, "", 0)
	fixture2.StartTLS()
	defer fixture2.Close()
	reader, err := controlclient.New(fixture2.URL, &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: pki.Roots, Certificates: []tls.Certificate{pki.ClientCertificate(t, subject)}})
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	read, err := reader.ReadCapacity(context.Background(), expected.ServerCertificateDigest)
	if err != nil {
		t.Fatal(err)
	}
	result, err := production.ObserveCapacity(read.Body, read.PeerCertificateDigest, read.RequestedAt, read.ReceivedAt, expected)
	if err != nil || result.Response.Source.AccessPolicyDigest != canonical.BytesDigest(policyRaw) || result.Ready {
		t.Fatal("source drift across configuration replacement", result, err)
	}
}
