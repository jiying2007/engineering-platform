package githubpublish

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/processscope"
	"github.com/jiying2007/engineering-platform/internal/testsupport"
)

func proofCompletionFixture(t *testing.T) (*RemoteService, Plan, DispatchStart, DispatchCompletion) {
	t.Helper()
	s, p := publisherServiceFixture(t)
	r := &evidenceRemote{serviceRemote: s.remote.(*serviceRemote), outcome: ObservedConfirmed}
	s.remote = r
	id := testDispatchIdentity(p)
	if w := evidenceCall(t, s, p, id, "/v1/publish"); w.Code != http.StatusOK {
		t.Fatal(w.Code, w.Body.String())
	}
	start, completion, err := s.journal.completed(context.Background(), id, p)
	if err != nil || completion.ValidateForRecovery(start) != nil {
		t.Fatal("qualified fixture missing", err)
	}
	return s, p, start, completion
}
func TestPublisherCompletionProofLegacyCannotBePromoted(t *testing.T) {
	s, p, start, c := proofCompletionFixture(t)
	c.Version = 1
	c.GitQuiescence = nil
	if c.Validate(start) != nil || c.ValidateForRecovery(start) == nil {
		t.Fatal("legacy readback/admission distinction lost")
	}
	raw, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(s.journal.path, dispatchSlot(p), "completed.json")
	if err = os.WriteFile(file, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.journal.completed(context.Background(), testDispatchIdentity(p), p); err != nil {
		t.Fatal("legacy bytes unreadable", err)
	}
	r := s.remote.(*evidenceRemote)
	if w := evidenceCall(t, s, p, testDispatchIdentity(p), "/v1/dispatch-evidence"); w.Code != http.StatusConflict {
		t.Fatal("old completion promoted", w.Code)
	}
	if r.observeCalls != 0 || r.publishCalls != 1 {
		t.Fatal("legacy proof caused new remote activity")
	}
}
func TestPublisherCompletionProofRejectsTampering(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*DispatchCompletion)
	}{
		{"missing", func(c *DispatchCompletion) { c.GitQuiescence = nil }},
		{"digest", func(c *DispatchCompletion) { c.GitQuiescence.Digest = "bad" }},
		{"empty", func(c *DispatchCompletion) { c.GitQuiescence.Proofs = nil }},
		{"unreaped", func(c *DispatchCompletion) { c.GitQuiescence.Proofs[0].InitReaped = false }},
		{"shared-namespace", func(c *DispatchCompletion) {
			c.GitQuiescence.Proofs[0].NamespaceID = c.GitQuiescence.Proofs[0].ParentNamespaceID
		}},
		{"before-start", func(c *DispatchCompletion) { c.GitQuiescence.Proofs[0].StartedAt = time.Unix(0, 0) }},
		{"after-return", func(c *DispatchCompletion) { c.GitQuiescence.Proofs[0].ReapedAt = c.ReturnedAt.Add(time.Second) }},
		{"duplicate", func(c *DispatchCompletion) {
			c.GitQuiescence.Proofs = append(c.GitQuiescence.Proofs, c.GitQuiescence.Proofs[0])
		}},
		{"oversized", func(c *DispatchCompletion) {
			c.GitQuiescence.Proofs = make([]processscope.Proof, maxPublicationGitProofs+1)
		}},
		{"version", func(c *DispatchCompletion) { c.GitQuiescence.Version++ }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, p, start, c := proofCompletionFixture(t)
			tc.mutate(&c)
			if c.GitQuiescence != nil && tc.name != "digest" {
				c.GitQuiescence.Digest, _ = canonical.Digest(c.GitQuiescence.Proofs)
			}
			if c.ValidateForRecovery(start) == nil {
				t.Fatal("altered proof accepted")
			}
			raw, _ := json.Marshal(c)
			if err := os.WriteFile(filepath.Join(s.journal.path, dispatchSlot(p), "completed.json"), raw, 0600); err != nil {
				t.Fatal(err)
			}
			if w := evidenceCall(t, s, p, testDispatchIdentity(p), "/v1/dispatch-evidence"); w.Code != http.StatusConflict {
				t.Fatal(w.Code)
			}
			if s.remote.(*evidenceRemote).observeCalls != 0 {
				t.Fatal("bad proof reached remote observation")
			}
		})
	}
}

type unprovedRemote struct{ *serviceRemote }

func (r *unprovedRemote) Publish(ctx context.Context, p Plan, b string, id DispatchIdentity) (PublicationReceipt, error) {
	// Deliberately drop the service-owned proof collector in this faulty adapter.
	return r.serviceRemote.Publish(context.Background(), p, b, id)
}
func TestPublisherCompletionProofMissingCollectorWithholdsWitness(t *testing.T) {
	s, p := publisherServiceFixture(t)
	r := &unprovedRemote{s.remote.(*serviceRemote)}
	s.remote = r
	if w := evidenceCall(t, s, p, testDispatchIdentity(p), "/v1/publish"); w.Code != http.StatusServiceUnavailable {
		t.Fatal(w.Code, w.Body.String())
	}
	if _, err := os.Lstat(filepath.Join(s.journal.path, dispatchSlot(p), "completed.json")); !os.IsNotExist(err) {
		t.Fatal("missing proofs wrote completed witness", err)
	}
	if w := evidenceCall(t, s, p, testDispatchIdentity(p), "/v1/publish"); w.Code != http.StatusConflict {
		t.Fatal("missing evidence retried original invocation")
	}
	if r.publishCalls != 1 {
		t.Fatal("provider repeated")
	}
}
func TestPublisherCompletionProofCollectorBounds(t *testing.T) {
	_, _, s, c := proofCompletionFixture(t)
	l := &completionProofLog{}
	ctx := context.WithValue(context.Background(), completionProofKey{}, l)
	for i := 0; i < maxPublicationGitProofs; i++ {
		p := c.GitQuiescence.Proofs[0]
		p.HostPID += i
		p.NamespaceID += uint64(i)
		if err := recordPublicationReap(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
	if recordPublicationReap(ctx, c.GitQuiescence.Proofs[0]) == nil || len(l.proofs) != maxPublicationGitProofs {
		t.Fatal("unbounded collector")
	}
	if _, err := l.completed(s, c.Receipt, c.ReturnedAt); err == nil {
		t.Fatal("overflow still minted completion")
	}
}
func TestPublisherCompletionProofActualKernel(t *testing.T) {
	testsupport.RequireProcessNamespaces(t)
	_, plan := publisherServiceFixture(t)
	start := DispatchStart{Version: 2, Incarnation: canonical.BytesDigest([]byte("test-slot")), Identity: testDispatchIdentity(plan), Plan: plan, StartedAt: time.Now().UTC()}
	// Exercise the actual gitOutput path and nested cleanup collector, not a fake
	// Remote's proof. No Git network, credentials or publication is performed.
	l := &completionProofLog{}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ctx = context.WithValue(ctx, completionProofKey{}, l)
	ctx = context.WithValue(ctx, gitProofKey{}, &gitProofLog{})
	git, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	g := &githubRemote{git: git}
	dir := t.TempDir()
	for i := 0; i < 3; i++ {
		if _, err := g.gitOutput(ctx, "", dir, dir, "--version"); err != nil {
			t.Fatal(err)
		}
	}
	q := GitQuiescence{Version: 1, Proofs: l.proofs}
	q.Digest, _ = canonical.Digest(q.Proofs)
	if len(q.Proofs) != 3 || q.validate(start.StartedAt, time.Now().UTC()) != nil {
		t.Fatalf("lost actual original proofs: %+v", q)
	}
}
