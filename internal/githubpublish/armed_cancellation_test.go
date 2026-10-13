//go:build linux

package githubpublish

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/testsupport"
)

// Err observes the actual marker after the original arm has synced it. This
// makes cancellation at that boundary deterministic, with no sleep or race.
type cancelAfterIntentContext struct {
	context.Context
	cancel context.CancelFunc
	intent string
}

func (c cancelAfterIntentContext) Err() error {
	if _, err := os.Lstat(c.intent); err == nil {
		c.cancel()
	}
	return c.Context.Err()
}
func cancelArmedOriginal(t *testing.T, s *RemoteService, p Plan) NonInvocationEvidence {
	t.Helper()
	id := testDispatchIdentity(p)
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx := cancelAfterIntentContext{parent, cancel, filepath.Join(s.journal.path, dispatchSlot(p), "invoked.json")}
	raw, err := json.Marshal(publishRequest{Plan: p, Identity: id})
	if err != nil {
		t.Fatal(err)
	}
	req := authorizedPublisherRequest(t, http.MethodPost, "/v1/publish", raw).WithContext(ctx)
	response := httptest.NewRecorder()
	s.Handler().ServeHTTP(response, req)
	if response.Code != http.StatusServiceUnavailable || !errors.Is(ctx.Err(), context.Canceled) {
		t.Fatalf("expected cancelled pre-entry request: code=%d body=%s", response.Code, response.Body.String())
	}
	remote := s.remote.(*serviceRemote)
	if remote.publishCalls != 0 || remote.observeCalls != 0 {
		t.Fatal("cancelled arm reached remote")
	}
	e, err := s.journal.sealUninvoked(context.Background(), id, p)
	if err != nil || e.Validate() != nil || e.Version != 2 || e.Invocation == nil {
		t.Fatalf("original certificate unavailable: %+v %v", e, err)
	}
	return e
}
func TestPublisherNoInvocationAfterArmCancellation(t *testing.T) {
	s, p := publisherServiceFixture(t)
	e := cancelArmedOriginal(t, s, p)
	original, _ := canonical.Digest(e)
	reopened, err := newDispatchJournal(s.journal.path)
	if err != nil {
		t.Fatal(err)
	}
	e2, err := reopened.sealUninvoked(context.Background(), testDispatchIdentity(p), p)
	got, _ := canonical.Digest(e2)
	if err != nil || got != original {
		t.Fatal("restart or exact repeat rewrote original certificate", err)
	}
	for _, id := range []DispatchIdentity{testDispatchIdentity(p), {OperationID: "replacement", RunID: p.RunID, ExecutionEpoch: 1, IdempotencyKey: "replacement", RequestDigest: canonical.BytesDigest([]byte("request"))}} {
		if response := evidenceCall(t, s, p, id, "/v1/publish"); response.Code != http.StatusConflict {
			t.Fatal("certificate allowed replay", response.Code)
		}
	}
	if s.remote.(*serviceRemote).publishCalls != 0 {
		t.Fatal("replay reached Publish")
	}
}
func TestPublisherNoInvocationAfterArmCannotCertifyEnteredCall(t *testing.T) {
	s, p := publisherServiceFixture(t)
	l, start, err := s.journal.begin(context.Background(), testDispatchIdentity(p), p)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	if err = l.prepareInvocation(context.Background(), start); err != nil {
		t.Fatal(err)
	}
	if err = l.retainArmedCancellation(context.Background(), start); err == nil {
		t.Fatal("entered invocation certified as absent")
	}
	if !witnessAbsent(l.Root, "not-invoked.json") {
		t.Fatal("invalid certificate persisted")
	}
	if err = l.prepareInvocation(context.Background(), start); err == nil {
		t.Fatal("one lease entered twice")
	}
}
func TestPublisherNoInvocationAfterArmRequiresOriginalWriter(t *testing.T) {
	s, p := publisherServiceFixture(t)
	l, start, err := s.journal.begin(context.Background(), testDispatchIdentity(p), p)
	if err != nil {
		t.Fatal(err)
	}
	if err = l.arm(context.Background(), start); err != nil {
		t.Fatal(err)
	}
	// Another object with the same directory cannot borrow the in-memory write.
	borrowed := &dispatchLease{Root: l.Root, lock: l.lock}
	if err = borrowed.retainArmedCancellation(context.Background(), start); err == nil {
		t.Fatal("disk intent substituted for original write provenance")
	}
	if err = l.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = s.journal.sealUninvoked(context.Background(), testDispatchIdentity(p), p); err == nil {
		t.Fatal("observer manufactured armed cancellation")
	}
}
func TestPublisherNoInvocationAfterArmRejectsCorruptEvidence(t *testing.T) {
	for _, mode := range []string{"missing-invocation", "wrong-invocation", "wrong-kind", "downgrade", "early-seal", "missing-marker", "partial-seal", "extra-completion", "restored-copy"} {
		t.Run(mode, func(t *testing.T) {
			s, p := publisherServiceFixture(t)
			e := cancelArmedOriginal(t, s, p)
			slot := filepath.Join(s.journal.path, dispatchSlot(p))
			invalidValue := true
			switch mode {
			case "missing-invocation":
				e.Invocation = nil
			case "wrong-invocation":
				e.Invocation.StartDigest = canonical.BytesDigest([]byte("other"))
			case "wrong-kind":
				e.Seal.Kind = "PUBLISH_NOT_INVOKED_V1"
			case "downgrade":
				e.Version = 1
			case "early-seal":
				e.Seal.SealedAt = e.Invocation.ArmedAt.Add(-time.Nanosecond)
			case "missing-marker":
				invalidValue = false
				if err := os.Remove(filepath.Join(slot, "invoked.json")); err != nil {
					t.Fatal(err)
				}
			case "partial-seal":
				invalidValue = false
				if err := os.WriteFile(filepath.Join(slot, "not-invoked.json"), []byte("{"), 0600); err != nil {
					t.Fatal(err)
				}
			case "extra-completion":
				invalidValue = false
				if err := os.WriteFile(filepath.Join(slot, "completed.json"), []byte("{}"), 0600); err != nil {
					t.Fatal(err)
				}
			case "restored-copy":
				invalidValue = false
				root := t.TempDir()
				if err := os.Chmod(root, 0700); err != nil {
					t.Fatal(err)
				}
				dst := filepath.Join(root, dispatchSlot(p))
				if err := os.Mkdir(dst, 0700); err != nil {
					t.Fatal(err)
				}
				for _, name := range []string{"started.json", "invoked.json", "not-invoked.json"} {
					raw, err := os.ReadFile(filepath.Join(slot, name))
					if err != nil {
						t.Fatal(err)
					}
					if err = os.WriteFile(filepath.Join(dst, name), raw, 0600); err != nil {
						t.Fatal(err)
					}
				}
				j, err := newDispatchJournal(root)
				if err != nil {
					t.Fatal(err)
				}
				s.journal = j
			}
			if invalidValue {
				if e.Validate() == nil {
					t.Fatal("inconsistent typed certificate accepted")
				}
				return
			}
			if _, err := s.journal.sealUninvoked(context.Background(), testDispatchIdentity(p), p); err == nil {
				t.Fatal("corrupt/restored certificate accepted")
			}
		})
	}
}
func TestPublisherNoInvocationAfterArmEvidenceRealMTLS(t *testing.T) {
	s, p := publisherServiceFixture(t)
	expected := cancelArmedOriginal(t, s, p)
	pki := testsupport.NewPKI(t)
	server := httptest.NewUnstartedServer(s.Handler())
	server.TLS = pki.ServerTLS()
	server.StartTLS()
	defer server.Close()
	client, err := newRemoteClient(healthRemoteConfig(t, pki, server.URL, s.controlSubject))
	if err != nil {
		t.Fatal(err)
	}
	defer client.client.CloseIdleConnections()
	actual, err := client.FenceDispatch(context.Background(), testDispatchIdentity(p), p)
	a, _ := json.Marshal(expected)
	b, _ := json.Marshal(actual)
	if err != nil || !bytes.Equal(a, b) || actual.Version != 2 {
		t.Fatalf("mTLS certificate changed: %s %v", b, err)
	}
	wrong, err := newRemoteClient(healthRemoteConfig(t, pki, server.URL, "urn:engineering-platform:worker:wrong"))
	if err != nil {
		t.Fatal(err)
	}
	defer wrong.client.CloseIdleConnections()
	if _, err = wrong.FenceDispatch(context.Background(), testDispatchIdentity(p), p); err == nil {
		t.Fatal("untrusted peer read original certificate")
	}
	if _, err = client.Publish(context.Background(), p, "unused", testDispatchIdentity(p)); err == nil {
		t.Fatal("mTLS readback enabled replay")
	}
	if s.remote.(*serviceRemote).publishCalls != 0 || s.remote.(*serviceRemote).observeCalls != 0 {
		t.Fatal("certificate readback touched external provider")
	}
}
func TestPublisherNoInvocationAfterArmFailedRetentionStaysBlocked(t *testing.T) {
	s, p := publisherServiceFixture(t)
	l, start, err := s.journal.begin(context.Background(), testDispatchIdentity(p), p)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	if err = l.arm(context.Background(), start); err != nil {
		t.Fatal(err)
	}
	if err = writeWitness(l.Root, "not-invoked.json", map[string]string{"partial": "existing"}); err != nil {
		t.Fatal(err)
	}
	if err = l.retainArmedCancellation(context.Background(), start); err == nil {
		t.Fatal("existing broken seal was overwritten")
	}
	if err = l.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = s.journal.sealUninvoked(context.Background(), testDispatchIdentity(p), p); err == nil {
		t.Fatal("failed seal became terminal authority")
	}
}

func TestPublisherNoInvocationAfterArmPreservesLegacyBytes(t *testing.T) {
	s, p := publisherServiceFixture(t)
	l, _, err := s.journal.begin(context.Background(), testDispatchIdentity(p), p)
	if err != nil {
		t.Fatal(err)
	}
	if err = l.Close(); err != nil {
		t.Fatal(err)
	}
	e, err := s.journal.sealUninvoked(context.Background(), testDispatchIdentity(p), p)
	if err != nil {
		t.Fatal(err)
	}
	// The original schema's exact field order and omission, not a hash of a new type.
	legacy := struct {
		Version int           `json:"version"`
		Start   DispatchStart `json:"start"`
		Seal    struct {
			Kind        string    `json:"kind"`
			StartDigest string    `json:"start_digest"`
			SealedAt    time.Time `json:"sealed_at"`
		} `json:"seal"`
	}{Version: 1, Start: e.Start}
	legacy.Seal.Kind = e.Seal.Kind
	legacy.Seal.StartDigest = e.Seal.StartDigest
	legacy.Seal.SealedAt = e.Seal.SealedAt
	a, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a, b) {
		t.Fatalf("historical v1 bytes changed: %s != %s", a, b)
	}
}
