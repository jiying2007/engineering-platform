package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/action"
	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/githubpublish"
	"github.com/jiying2007/engineering-platform/internal/processscope"
	"github.com/jiying2007/engineering-platform/internal/recovery"
	"github.com/jiying2007/engineering-platform/internal/store"
)

// These handler fixtures deliberately do not pretend to be original host/GitHub
// witnesses. They verify real TLS/capability routing and fault classification.
type publicationRecoveryTestStore struct {
	*store.Memory
	op        action.Operation
	current   recovery.Manager
	saved     *githubpublish.PublicationRecoveryReceipt
	calls     int
	commitErr error
}

func (s *publicationRecoveryTestStore) GetContext(context.Context, string) (action.Operation, error) {
	return s.op, nil
}
func (s *publicationRecoveryTestStore) GetRecoveryContext(context.Context) (recovery.Manager, error) {
	return s.current, nil
}
func (s *publicationRecoveryTestStore) GetRecoveredPublication(context.Context, string) (githubpublish.PublicationRecoveryReceipt, error) {
	if s.saved != nil {
		return *s.saved, nil
	}
	return githubpublish.PublicationRecoveryReceipt{}, store.ErrNotFound
}
func (s *publicationRecoveryTestStore) ConfirmRecoveredPublication(_ context.Context, subject string, r githubpublish.PublicationRecoveryRequest, e githubpublish.PublicationEvidence) (githubpublish.PublicationRecoveryReceipt, error) {
	s.calls++
	d, _ := r.Digest()
	ed, _ := e.Digest()
	saved := githubpublish.PublicationRecoveryReceipt{Kind: "RECOVERY_GITHUB_PUBLICATION_CONFIRMED_V1", Request: r, RequestDigest: d, Reconciler: subject, PreviousState: s.op.State, Evidence: e, EvidenceDigest: ed, RecordedAt: time.Now().UTC()}
	if s.commitErr != nil {
		return githubpublish.PublicationRecoveryReceipt{}, s.commitErr
	}
	s.saved = &saved
	return saved, nil
}

type publicationRecoveryTestObserver struct {
	e     githubpublish.PublicationEvidence
	calls int
	err   error
}

func (o *publicationRecoveryTestObserver) ObserveOriginalPublication(context.Context, action.Operation) (githubpublish.PublicationEvidence, error) {
	o.calls++
	return o.e, o.err
}
func publicationRecoveryTestFixture() (githubpublish.PublicationRecoveryRequest, *publicationRecoveryTestStore, *publicationRecoveryTestObserver) {
	id := githubpublish.DispatchIdentity{OperationID: "op-recovery", RunID: "run-recovery", ExecutionEpoch: 1, IdempotencyKey: "reserved", RequestDigest: "sha256:" + strings.Repeat("a", 64)}
	req := githubpublish.PublicationRecoveryRequest{Version: 1, Identity: id, RecoveryEpoch: 1}
	p := githubpublish.Plan{Version: 1, RunID: id.RunID, Repository: "example/fixture", BaseRef: "main", BaseCommit: strings.Repeat("1", 40), Branch: "test/result", ResultCommit: strings.Repeat("2", 40), ResultDigest: "sha256:" + strings.Repeat("3", 64), ReceiptDigest: "sha256:" + strings.Repeat("4", 64), BundleDigest: "sha256:" + strings.Repeat("5", 64), BundleSize: 7, ExecutionID: strings.Repeat("6", 64)}
	receipt := githubpublish.PublicationReceipt{Version: 1, Repository: p.Repository, BaseRef: p.BaseRef, BaseCommit: p.BaseCommit, Branch: p.Branch, ResultCommit: p.ResultCommit, PullRequestNumber: 7, PullRequestURL: "https://github.com/example/fixture/pull/7", PullRequestState: "open", PublicationOutcome: "CREATED"}
	at := time.Now().UTC().Add(-time.Second)
	start := githubpublish.DispatchStart{Version: 2, Incarnation: "sha256:" + strings.Repeat("a", 64), Identity: id, Plan: p, StartedAt: at}
	d, _ := canonical.Digest(start)
	completion := githubpublish.DispatchCompletion{Version: 2, StartDigest: d, Receipt: receipt, ReturnedAt: at.Add(time.Millisecond)}
	// Controlled fixture; not selected-host process evidence.
	proofs := []processscope.Proof{{Version: 1, Mechanism: processscope.Mechanism, HostPID: 17, NamespaceID: 2, ParentNamespaceID: 1, StartedAt: at, ReapedAt: at, InitReaped: true}}
	proofDigest, _ := canonical.Digest(proofs)
	completion.GitQuiescence = &githubpublish.GitQuiescence{Version: 1, Proofs: proofs, Digest: proofDigest}
	receipt.PublicationOutcome = "OBSERVED"
	e := githubpublish.PublicationEvidence{Version: 1, Start: start, Completion: completion, Observation: githubpublish.ObserveResult{Outcome: githubpublish.ObservedConfirmed, Receipt: receipt}, ObservedAt: at.Add(2 * time.Millisecond)}
	backend := &publicationRecoveryTestStore{Memory: store.NewMemory(), op: action.Operation{ID: id.OperationID, RunID: id.RunID, ExecutionEpoch: id.ExecutionEpoch, RecoveryEpoch: id.RecoveryEpoch, Action: githubpublish.Action, Capability: githubpublish.Capability, RiskClass: action.ControlledMutation, IdempotencyKey: id.IdempotencyKey, RequestDigest: id.RequestDigest, State: action.Manual}, current: recovery.Manager{Epoch: 1, Mode: recovery.RecoveryReconciliation}}
	return req, backend, &publicationRecoveryTestObserver{e: e}
}
func TestPublicationRecoveryMTLSRequiresSeparateReconcilerAndRetainsReadback(t *testing.T) {
	req, backend, observer := publicationRecoveryTestFixture()
	server, pki := securedTestServer(t, backend, nil, AuthenticatedOptions{PublicationRecovery: observer})
	post := server.URL + "/api/v1/recovery/actions/confirm-publication"
	get := server.URL + "/api/v1/recovery/actions/op-recovery/publication"
	for _, subject := range []string{engineerSubject, recoverySubject, ciSubject} {
		secureCall(t, pki.Client(t, subject), post, req, http.StatusForbidden)
	}
	secureCall(t, pki.Client(t, reconcilerSubject), post, map[string]any{"version": 1, "identity": req.Identity, "recovery_epoch": 1, "evidence": observer.e}, http.StatusBadRequest)
	bad := req
	bad.Identity.IdempotencyKey = "different"
	secureCall(t, pki.Client(t, reconcilerSubject), post, bad, http.StatusConflict)
	if observer.calls != 0 || backend.calls != 0 {
		t.Fatal("denied request crossed observer/DB boundary")
	}
	raw := secureCall(t, pki.Client(t, reconcilerSubject), post, req, http.StatusCreated)
	var receipt githubpublish.PublicationRecoveryReceipt
	if json.Unmarshal(raw, &receipt) != nil || receipt.Validate() != nil || receipt.ReplayAuthorized {
		t.Fatal("invalid recovered receipt")
	}
	// After completion, exact repeat reads the existing fact even if observer dies.
	observer.err = githubpublish.ErrDispatchEvidenceUnavailable
	backend.current.Mode = recovery.Normal
	secureCall(t, pki.Client(t, reconcilerSubject), post, req, http.StatusOK)
	secureRaw(t, pki.Client(t, reconcilerSubject), http.MethodGet, get, nil, http.StatusOK)
	if observer.calls != 1 || backend.calls != 1 {
		t.Fatal("repeat re-observed or re-wrote settlement")
	}
}
func TestPublicationRecoveryRejectsUnprovedAndPreservesCommitUnknown(t *testing.T) {
	for _, mode := range []string{"absent", "wrong-pr", "missing-completion", "observer-error", "commit-unknown", "normal", "stale", "legacy-completion"} {
		t.Run(mode, func(t *testing.T) {
			req, backend, observer := publicationRecoveryTestFixture()
			want := http.StatusConflict
			switch mode {
			case "legacy-completion":
				observer.e.Completion.Version = 1
				observer.e.Completion.GitQuiescence = nil
			case "absent":
				observer.e.Observation.Outcome = githubpublish.ObservedAbsent
			case "wrong-pr":
				observer.e.Observation.Receipt.PullRequestNumber++
			case "missing-completion":
				observer.e.Completion = githubpublish.DispatchCompletion{}
			case "observer-error":
				observer.err = githubpublish.ErrDispatchEvidenceUnavailable
			case "commit-unknown":
				backend.commitErr = store.ErrCommitOutcomeUnknown
				want = http.StatusServiceUnavailable
			case "normal":
				backend.current.Mode = recovery.Normal
			case "stale":
				backend.current.Epoch++
			}
			server, pki := securedTestServer(t, backend, nil, AuthenticatedOptions{PublicationRecovery: observer})
			raw := secureCall(t, pki.Client(t, reconcilerSubject), server.URL+"/api/v1/recovery/actions/confirm-publication", req, want)
			if mode == "commit-unknown" && !strings.Contains(string(raw), "commit_outcome_unknown") {
				t.Fatal("lost commit uncertainty")
			}
			if backend.saved != nil {
				t.Fatal("invalid evidence generated a receipt")
			}
			if mode != "commit-unknown" && backend.calls != 0 {
				t.Fatal("invalid evidence reached mutation")
			}
		})
	}
}

// Retained exact readback is independent of current Publisher configuration.
func TestPublicationRecoveryReadbackWithoutObserver(t *testing.T) {
	req, backend, observer := publicationRecoveryTestFixture()
	if _, err := backend.ConfirmRecoveredPublication(context.Background(), reconcilerSubject, req, observer.e); err != nil {
		t.Fatal(err)
	}
	backend.current.Mode = recovery.Normal
	server, pki := securedTestServer(t, backend, nil, AuthenticatedOptions{})
	secureCall(t, pki.Client(t, reconcilerSubject), server.URL+"/api/v1/recovery/actions/confirm-publication", req, http.StatusOK)
	secureRaw(t, pki.Client(t, reconcilerSubject), http.MethodGet, server.URL+"/api/v1/recovery/actions/other-operation/publication", nil, http.StatusServiceUnavailable)
	if backend.calls != 1 || observer.calls != 0 {
		t.Fatal("readback performed additional work")
	}
}
