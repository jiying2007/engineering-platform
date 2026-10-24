package api

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/access"
	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/controlclient"
	"github.com/jiying2007/engineering-platform/internal/production"
	"github.com/jiying2007/engineering-platform/internal/store"
	"github.com/jiying2007/engineering-platform/internal/testsupport"
)

const capacityReaderSubject = "urn:engineering-platform:operator:capacity"
const capacityWorkerSubject = "urn:engineering-platform:worker:capacity"

type capacityMemory struct {
	*store.Memory
	reads atomic.Int64
	read  func(context.Context, []production.WorkerCapacityGrant) (production.CapacityFacts, error)
}

func (s *capacityMemory) ReadOperationalCapacity(ctx context.Context, grants []production.WorkerCapacityGrant) (production.CapacityFacts, error) {
	s.reads.Add(1)
	return s.read(ctx, grants)
}

func apiCapacityFacts(grants []production.WorkerCapacityGrant) production.CapacityFacts {
	now := time.Now().UTC()
	facts := production.CapacityFacts{CapturedAt: now, RecoveryMode: "NORMAL", Workers: []production.WorkerCapacityFact{}, Queues: []production.QueueCapacityFact{}, Pool: production.PoolCapacityFact{CapturedAt: now, Maximum: 4, Total: 1, Idle: 1}}
	facts.Authority, _ = production.EvaluateSnapshot(production.Snapshot{Version: production.OperationalStatusVersion, CapturedAt: now, RecoveryMode: "NORMAL", WorkerPolls: []production.WorkerPollObservation{}})
	for _, grant := range grants {
		facts.Workers = append(facts.Workers, production.WorkerCapacityFact{Grant: grant})
	}
	profiles, _ := production.CapacityProfiles(grants)
	for _, profile := range profiles {
		facts.Queues = append(facts.Queues, production.QueueCapacityFact{Profile: profile})
	}
	return facts
}

func apiCapacityPolicy(t *testing.T) (*access.Policy, []byte) {
	t.Helper()
	raw, err := json.Marshal(access.Document{Version: 1, Principals: []access.PrincipalSpec{
		{Subject: capacityReaderSubject, Scope: "platform", Capabilities: []string{access.Read}},
		{Subject: capacityWorkerSubject, Scope: "platform", Capabilities: []string{access.WorkerPoll}, WorkerProfiles: []string{"worker/capacity"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	policy, err := access.Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	return policy, raw
}

func apiCapacityOptions(pki *testsupport.PKI, raw []byte) *OperationalCapacityOptions {
	return &OperationalCapacityOptions{
		Source:  production.CapacitySource{ServerCertificateDigest: canonical.BytesDigest(pki.ServerCertificate.Certificate[0]), AccessPolicyDigest: canonical.BytesDigest(raw), SourceCommit: strings.Repeat("a", 40), InstanceID: strings.Repeat("b", 32)},
		Workers: []production.WorkerCapacityGrant{{Subject: capacityWorkerSubject, Profiles: []string{"worker/capacity"}, Lane: "ADMISSION"}},
	}
}

func apiCapacityServer(t *testing.T, pki *testsupport.PKI, policy *access.Policy, backend store.Store, options *OperationalCapacityOptions) *httptest.Server {
	t.Helper()
	handler, err := NewAuthenticatedHandler(backend, nil, policy, AuthenticatedOptions{OperationalCapacity: options})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(handler)
	server.TLS = pki.ServerTLS()
	server.Config.ErrorLog = log.New(io.Discard, "", 0)
	server.StartTLS()
	t.Cleanup(server.Close)
	return server
}

func TestCapacityAPIUsesOnlyReadCapabilityAndCurrentPolicy(t *testing.T) {
	pki := testsupport.NewPKI(t)
	policy, raw := apiCapacityPolicy(t)
	options := apiCapacityOptions(pki, raw)
	expected := options.Source
	backend := &capacityMemory{Memory: store.NewMemory(), read: func(_ context.Context, grants []production.WorkerCapacityGrant) (production.CapacityFacts, error) {
		if len(grants) != 1 || grants[0].Subject != capacityWorkerSubject || !reflect.DeepEqual(grants[0].Profiles, []string{"worker/capacity"}) {
			return production.CapacityFacts{}, errors.New("policy copy changed")
		}
		return apiCapacityFacts(grants), nil
	}}
	server := apiCapacityServer(t, pki, policy, backend, options)
	// Caller-owned options are not a live policy update surface.
	options.Source.InstanceID = strings.Repeat("c", 32)
	options.Workers[0].Profiles[0] = "worker/forged"
	client, err := controlclient.New(server.URL, &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: pki.Roots, Certificates: []tls.Certificate{pki.ClientCertificate(t, capacityReaderSubject)}})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	read, err := client.ReadCapacity(context.Background(), expected.ServerCertificateDigest)
	if err != nil {
		t.Fatal(err)
	}
	result, err := production.ObserveCapacity(read.Body, read.PeerCertificateDigest, read.RequestedAt, read.ReceivedAt, expected)
	if err != nil || result.Ready || result.ProductionQualified || result.WorkerReservationHeadroom != 0 || result.PoolReservationHeadroom != 4 {
		t.Fatalf("read-only capacity observation: %#v %v", result, err)
	}
	secureRaw(t, pki.Client(t, capacityWorkerSubject), http.MethodGet, server.URL+"/api/v1/operations/capacity", nil, http.StatusForbidden)
	secureRaw(t, pki.Client(t, "urn:engineering-platform:unknown"), http.MethodGet, server.URL+"/api/v1/operations/capacity", nil, http.StatusUnauthorized)
	secureRaw(t, pki.Client(t, capacityReaderSubject), http.MethodPost, server.URL+"/api/v1/operations/capacity", []byte(`{}`), http.StatusForbidden)
	secureRaw(t, pki.Client(t, capacityReaderSubject), http.MethodPost, server.URL+"/api/v1/recovery/begin", []byte(`{}`), http.StatusForbidden)
	if backend.reads.Load() != 1 {
		t.Fatal("denied caller reached capacity reader", backend.reads.Load())
	}
	// The explicit unauthenticated memory fixture also cannot serve this route.
	mustRequest(t, NewServer(backend).Handler(), http.MethodGet, "/api/v1/operations/capacity", nil, http.StatusForbidden)
}

func TestCapacityAPIRejectsAuthorityPopulationDriftAndUnavailableSource(t *testing.T) {
	pki := testsupport.NewPKI(t)
	policy, raw := apiCapacityPolicy(t)
	for _, mutate := range []func(*OperationalCapacityOptions){
		func(o *OperationalCapacityOptions) { o.Workers = []production.WorkerCapacityGrant{} },
		func(o *OperationalCapacityOptions) { o.Workers[0].Profiles[0] = "worker/historical" },
		func(o *OperationalCapacityOptions) { o.Workers[0].Subject = "urn:engineering-platform:worker:retired" },
		func(o *OperationalCapacityOptions) { o.Workers[0].Lane = "PREPARATION" },
	} {
		options := apiCapacityOptions(pki, raw)
		mutate(options)
		if _, err := NewAuthenticatedHandler(store.NewMemory(), nil, policy, AuthenticatedOptions{OperationalCapacity: options}); err == nil {
			t.Fatal("capacity population drift admitted")
		}
	}
	for name, behavior := range map[string]func([]production.WorkerCapacityGrant) production.CapacityFacts{
		"omitted-workers": func(g []production.WorkerCapacityGrant) production.CapacityFacts {
			f := apiCapacityFacts(g)
			f.Workers = []production.WorkerCapacityFact{}
			return f
		},
		"stale": func(g []production.WorkerCapacityGrant) production.CapacityFacts {
			f := apiCapacityFacts(g)
			f.CapturedAt = f.CapturedAt.Add(-time.Minute)
			return f
		},
		"contradictory-pool": func(g []production.WorkerCapacityGrant) production.CapacityFacts {
			f := apiCapacityFacts(g)
			f.Pool.Idle = 2
			return f
		},
	} {
		t.Run(name, func(t *testing.T) {
			backend := &capacityMemory{Memory: store.NewMemory(), read: func(_ context.Context, g []production.WorkerCapacityGrant) (production.CapacityFacts, error) {
				return behavior(g), nil
			}}
			server := apiCapacityServer(t, pki, policy, backend, apiCapacityOptions(pki, raw))
			secureRaw(t, pki.Client(t, capacityReaderSubject), http.MethodGet, server.URL+"/api/v1/operations/capacity", nil, http.StatusServiceUnavailable)
		})
	}
	for _, options := range []*OperationalCapacityOptions{nil, apiCapacityOptions(pki, raw)} {
		server := apiCapacityServer(t, pki, policy, store.NewMemory(), options)
		secureRaw(t, pki.Client(t, capacityReaderSubject), http.MethodGet, server.URL+"/api/v1/operations/capacity", nil, http.StatusServiceUnavailable)
		secureRaw(t, pki.Client(t, capacityReaderSubject), http.MethodGet, server.URL+"/api/v1/recovery", nil, http.StatusOK)
	}
}

func TestCapacityAPIPreservesCallerCancellation(t *testing.T) {
	pki := testsupport.NewPKI(t)
	policy, raw := apiCapacityPolicy(t)
	entered, stopped := make(chan struct{}), make(chan struct{})
	backend := &capacityMemory{Memory: store.NewMemory(), read: func(ctx context.Context, _ []production.WorkerCapacityGrant) (production.CapacityFacts, error) {
		close(entered)
		<-ctx.Done()
		close(stopped)
		return production.CapacityFacts{}, ctx.Err()
	}}
	server := apiCapacityServer(t, pki, policy, backend, apiCapacityOptions(pki, raw))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/api/v1/operations/capacity", nil)
	client := pki.Client(t, capacityReaderSubject)
	done := make(chan error, 1)
	go func() {
		response, err := client.Do(request)
		if response != nil {
			response.Body.Close()
		}
		done <- err
	}()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("capacity request never reached source")
	}
	cancel()
	select {
	case <-stopped:
	case <-time.After(3 * time.Second):
		t.Fatal("capacity source lost request cancellation")
	}
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal("caller cancellation lost", err)
	}
	if backend.reads.Load() != 1 {
		t.Fatal("capacity read retried")
	}
}
