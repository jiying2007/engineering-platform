package githubpublish

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/action"
	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/testsupport"
)

func testDispatchIdentity(p Plan) DispatchIdentity {
	return DispatchIdentity{OperationID: "op-1", RunID: p.RunID, ExecutionEpoch: 1, IdempotencyKey: "key-1", RequestDigest: "sha256:" + strings.Repeat("a", 64)}
}
func evidenceCall(t *testing.T, s *RemoteService, p Plan, id DispatchIdentity, path string) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(publishRequest{Plan: p, Identity: id})
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, authorizedPublisherRequest(t, http.MethodPost, path, raw))
	return w
}

type evidenceRemote struct {
	*serviceRemote
	outcome Observation
	number  int
	err     error
}

func (e *evidenceRemote) Observe(ctx context.Context, p Plan) (ObserveResult, error) {
	e.observeCalls++
	if e.err != nil {
		return ObserveResult{}, e.err
	}
	receipt := PublicationReceipt{Version: 1, Repository: p.Repository, BaseRef: p.BaseRef, BaseCommit: p.BaseCommit, Branch: p.Branch, ResultCommit: p.ResultCommit, PullRequestNumber: 17, PullRequestURL: "https://github.com/" + p.Repository + "/pull/17", PullRequestState: "open", PublicationOutcome: "OBSERVED"}
	if e.number != 0 {
		receipt.PullRequestNumber = e.number
		receipt.PullRequestURL = "https://github.com/" + p.Repository + "/pull/18"
	}
	return ObserveResult{Outcome: e.outcome, Receipt: receipt}, nil
}
func TestPublisherDurableCompletionSurvivesRestartWithoutReplay(t *testing.T) {
	s, p := publisherServiceFixture(t)
	remote := &evidenceRemote{serviceRemote: s.remote.(*serviceRemote), outcome: ObservedConfirmed}
	s.remote = remote
	id := testDispatchIdentity(p)
	if w := evidenceCall(t, s, p, id, "/v1/dispatch-evidence"); w.Code != http.StatusConflict {
		t.Fatal(w.Code, w.Body.String())
	}
	if remote.publishCalls != 0 || remote.observeCalls != 0 {
		t.Fatal("missing witness initiated an external call")
	}
	if w := evidenceCall(t, s, p, id, "/v1/publish"); w.Code != http.StatusOK {
		t.Fatal(w.Code, w.Body.String())
	}
	// Reopen private durable bytes as a restarted service, not process-memory state.
	reopened, err := newDispatchJournal(s.journal.path)
	if err != nil {
		t.Fatal(err)
	}
	s.journal = reopened
	if w := evidenceCall(t, s, p, id, "/v1/publish"); w.Code != http.StatusConflict {
		t.Fatal("duplicate publication accepted", w.Code)
	}
	w := evidenceCall(t, s, p, id, "/v1/dispatch-evidence")
	if w.Code != http.StatusOK {
		t.Fatal(w.Code, w.Body.String())
	}
	var evidence PublicationEvidence
	if json.Unmarshal(w.Body.Bytes(), &evidence) != nil || evidence.Validate() != nil || evidence.Start.Identity != id {
		t.Fatal("unbound witness")
	}
	if remote.publishCalls != 1 || remote.observeCalls != 1 {
		t.Fatal("recovery replayed publication")
	}
}
func TestPublisherStartedOnlyNeverBecomesCompletionFromLaterPositiveRead(t *testing.T) {
	s, p := publisherServiceFixture(t)
	remote := &evidenceRemote{serviceRemote: s.remote.(*serviceRemote), outcome: ObservedConfirmed}
	s.remote = remote
	id := testDispatchIdentity(p)
	remote.onPublish = func(string) error { return errors.New("original transport outcome unknown") }
	if w := evidenceCall(t, s, p, id, "/v1/publish"); w.Code != http.StatusBadGateway {
		t.Fatal(w.Code)
	}
	if w := evidenceCall(t, s, p, id, "/v1/dispatch-evidence"); w.Code != http.StatusConflict {
		t.Fatal("guessed process completion", w.Code)
	}
	if remote.publishCalls != 1 || remote.observeCalls != 0 {
		t.Fatal("started-only record gained observation or replay")
	}
}
func TestPublisherWitnessRejectsNegativeReplacementAndIdentityDrift(t *testing.T) {
	for _, outcome := range []Observation{ObservedAbsent, ObservedPartial, ObservedConflict, "BOGUS"} {
		t.Run(string(outcome), func(t *testing.T) {
			s, p := publisherServiceFixture(t)
			remote := &evidenceRemote{serviceRemote: s.remote.(*serviceRemote), outcome: outcome}
			s.remote = remote
			id := testDispatchIdentity(p)
			if w := evidenceCall(t, s, p, id, "/v1/publish"); w.Code != http.StatusOK {
				t.Fatal(w.Code)
			}
			if w := evidenceCall(t, s, p, id, "/v1/dispatch-evidence"); w.Code != http.StatusConflict {
				t.Fatal(w.Code)
			}
			remote.outcome = ObservedConfirmed
			remote.number = 18
			if w := evidenceCall(t, s, p, id, "/v1/dispatch-evidence"); w.Code != http.StatusConflict {
				t.Fatal("replacement PR accepted")
			}
			before := remote.observeCalls
			id.RequestDigest = "sha256:" + strings.Repeat("b", 64)
			if w := evidenceCall(t, s, p, id, "/v1/dispatch-evidence"); w.Code != http.StatusConflict {
				t.Fatal("identity drift accepted")
			}
			if remote.observeCalls != before || remote.publishCalls != 1 {
				t.Fatal("identity rejection called remote")
			}
		})
	}
}
func TestPublisherWitnessRejectsSymlinkAndPartialBytes(t *testing.T) {
	for _, kind := range []string{"null", "truncated", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			s, p := publisherServiceFixture(t)
			id := testDispatchIdentity(p)
			if w := evidenceCall(t, s, p, id, "/v1/publish"); w.Code != http.StatusOK {
				t.Fatal(w.Code)
			}
			file := filepath.Join(s.journal.path, dispatchSlot(p), "completed.json")
			if err := os.Remove(file); err != nil {
				t.Fatal(err)
			}
			if kind == "symlink" {
				if err := os.Symlink("started.json", file); err != nil {
					t.Fatal(err)
				}
			} else {
				raw := []byte("null")
				if kind == "truncated" {
					raw = []byte(`{"version":1`)
				}
				if err := os.WriteFile(file, raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if _, _, err := s.journal.completed(context.Background(), id, p); err == nil {
				t.Fatal("invalid completion accepted")
			}
		})
	}
}
func TestPublisherCancelledJournalDoesNotAllocate(t *testing.T) {
	s, p := publisherServiceFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := s.journal.begin(ctx, testDispatchIdentity(p), p); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(s.journal.path)
	if err != nil || len(entries) != 0 {
		t.Fatal("cancelled publication allocated witness")
	}
}
func TestPublicationEvidenceCannotOverrideCompletion(t *testing.T) {
	s, p := publisherServiceFixture(t)
	id := testDispatchIdentity(p)
	slot, start, err := s.journal.begin(context.Background(), id, p)
	if err != nil {
		t.Fatal(err)
	}
	defer slot.Close()
	remote := &evidenceRemote{serviceRemote: &serviceRemote{}, outcome: ObservedConfirmed}
	observed, _ := remote.Observe(context.Background(), p)
	digest, _ := canonical.Digest(start)
	original := observed.Receipt
	original.PublicationOutcome = "CREATED"
	completion := DispatchCompletion{1, digest, original, time.Now().UTC()}
	e := PublicationEvidence{1, start, completion, observed, time.Now().UTC()}
	if e.Validate() != nil {
		t.Fatal("fixture invalid")
	}
	e.Completion.Receipt.PublicationOutcome = "OBSERVED"
	if e.Validate() == nil {
		t.Fatal("observation substituted for original publication completion")
	}
	e.Completion.Receipt.PublicationOutcome = "CREATED"
	e.Observation.Receipt.PublicationOutcome = "CREATED"
	if e.Validate() == nil {
		t.Fatal("original receipt substituted for independent observation")
	}
	e.Observation.Receipt.PublicationOutcome = "OBSERVED"
	e.Completion.StartDigest = "sha256:" + strings.Repeat("0", 64)
	if e.Validate() == nil {
		t.Fatal("substituted start accepted")
	}
}

func TestPublisherEvidenceUsesRealMTLSAndRejectsWrongPeer(t *testing.T) {
	service, plan := publisherServiceFixture(t)
	upstream := &evidenceRemote{serviceRemote: service.remote.(*serviceRemote), outcome: ObservedConfirmed}
	service.remote = upstream
	pki := testsupport.NewPKI(t)
	server := httptest.NewUnstartedServer(service.Handler())
	server.TLS = pki.ServerTLS()
	server.StartTLS()
	defer server.Close()
	config := healthRemoteConfig(t, pki, server.URL, service.controlSubject)
	client, err := newRemoteClient(config)
	if err != nil {
		t.Fatal(err)
	}
	defer client.client.CloseIdleConnections()
	id := testDispatchIdentity(plan)
	if _, err := client.Publish(context.Background(), plan, "only-the-publisher-resolves-bundle", id); err != nil {
		t.Fatal(err)
	}
	e, err := client.ObserveDispatch(context.Background(), id, plan)
	if err != nil || e.Validate() != nil {
		t.Fatal(e, err)
	}
	wrong, err := newRemoteClient(healthRemoteConfig(t, pki, server.URL, "urn:engineering-platform:worker:wrong"))
	if err != nil {
		t.Fatal(err)
	}
	defer wrong.client.CloseIdleConnections()
	if _, err := wrong.ObserveDispatch(context.Background(), id, plan); err == nil {
		t.Fatal("wrong peer read original evidence")
	}
	if upstream.publishCalls != 1 || upstream.observeCalls != 1 {
		t.Fatal("evidence route replayed or wrong peer called upstream")
	}
}

func TestPublisherInflightInvocationCannotProduceRecoveryEvidence(t *testing.T) {
	s, p := publisherServiceFixture(t)
	remote := &evidenceRemote{serviceRemote: s.remote.(*serviceRemote), outcome: ObservedConfirmed}
	s.remote = remote
	id := testDispatchIdentity(p)
	entered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan *httptest.ResponseRecorder, 1)
	remote.onPublish = func(string) error { close(entered); <-release; return nil }
	raw, _ := json.Marshal(publishRequest{Plan: p, Identity: id})
	request := authorizedPublisherRequest(t, http.MethodPost, "/v1/publish", raw)
	go func() { w := httptest.NewRecorder(); s.Handler().ServeHTTP(w, request); done <- w }()
	<-entered
	// The original invocation has been admitted and is still blocked in Publish.
	w := evidenceCall(t, s, p, id, "/v1/dispatch-evidence")
	second := evidenceCall(t, s, p, id, "/v1/publish")
	if w.Code != http.StatusConflict || second.Code != http.StatusConflict || remote.observeCalls != 0 {
		close(release)
		<-done
		t.Fatal("in-flight invocation obtained evidence or replay")
	}
	close(release)
	first := <-done
	if first.Code != http.StatusOK {
		t.Fatal(first.Code, first.Body.String())
	}
	if w := evidenceCall(t, s, p, id, "/v1/dispatch-evidence"); w.Code != http.StatusOK {
		t.Fatal(w.Code)
	}
	if remote.publishCalls != 1 || remote.observeCalls != 1 {
		t.Fatal("unexpected remote calls")
	}
}
func TestPublisherWitnessRootReplacementFailsClosed(t *testing.T) {
	s, p := publisherServiceFixture(t)
	root := s.journal.path
	if err := os.Rename(root, root+"-original"); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root + "-original")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	if w := evidenceCall(t, s, p, testDispatchIdentity(p), "/v1/publish"); w.Code != http.StatusConflict {
		t.Fatal("replacement witness root accepted")
	}
	if s.remote.(*serviceRemote).publishCalls != 0 {
		t.Fatal("root substitution reached remote")
	}
}

func TestProviderRecoveryBindsCoreIdentityThroughIndependentMTLSPublisher(t *testing.T) {
	state, config := publisherFixture(t)
	witness := t.TempDir()
	if err := os.Chmod(witness, 0700); err != nil {
		t.Fatal(err)
	}
	journal, err := newDispatchJournal(witness)
	if err != nil {
		t.Fatal(err)
	}
	artifactInfo, err := os.Lstat(config.ArtifactRoot)
	if err != nil {
		t.Fatal(err)
	}
	targets, err := targetMap(config.Version, config.Targets)
	if err != nil {
		t.Fatal(err)
	}
	upstream := &evidenceRemote{serviceRemote: &serviceRemote{}, outcome: ObservedConfirmed}
	service := &RemoteService{remote: upstream, journal: journal, artifactRoot: config.ArtifactRoot, artifactIdentity: artifactInfo, targets: targets, controlSubject: "urn:engineering-platform:control:publisher-client"}
	pki := testsupport.NewPKI(t)
	server := httptest.NewUnstartedServer(service.Handler())
	server.TLS = pki.ServerTLS()
	server.StartTLS()
	defer server.Close()
	remote, err := newRemoteClient(healthRemoteConfig(t, pki, server.URL, service.controlSubject))
	if err != nil {
		t.Fatal(err)
	}
	defer remote.client.CloseIdleConnections()
	provider, err := New(config, state, remote)
	if err != nil {
		t.Fatal(err)
	}
	request := action.Request{ID: "original-action", RunID: state.run.ID, ExecutionEpoch: state.run.CurrentEpoch, Action: Action, Capability: Capability, RiskClass: action.ControlledMutation, IdempotencyKey: "original-reservation", ParametersDigest: state.status.Receipt.ResultDigest, RequestedBy: "runtime"}
	receipt, err := provider.Dispatch(context.Background(), request)
	if err != nil || receipt.Outcome != action.DispatchConfirmed {
		t.Fatal(receipt, err)
	}
	digest, err := action.RequestDigest(request)
	if err != nil {
		t.Fatal(err)
	}
	op := *action.NewWithRequestDigest(request, digest, time.Now().UTC())
	op.State = action.Dispatched // Model loss of Core settlement only, not live failure proof.
	if err := os.Remove(filepath.Join(config.ArtifactRoot, state.status.Token.ID+".bundle")); err != nil {
		t.Fatal(err)
	}
	evidence, err := provider.ObserveOriginalPublication(context.Background(), op)
	if err != nil || evidence.Validate() != nil || evidence.Start.Identity.RequestDigest != digest {
		t.Fatal(evidence, err)
	}
	// A caller cannot substitute another original reservation for the retained plan.
	op.IdempotencyKey = "substituted"
	if _, err := provider.ObserveOriginalPublication(context.Background(), op); err == nil {
		t.Fatal("mismatched Core identity accepted")
	}
	if upstream.publishCalls != 1 || upstream.observeCalls != 1 {
		t.Fatal("readback required bundle or replayed external work")
	}
}

// Exact private witness bytes can be restored into a new root without
// replaying publication. This is a local filesystem backup fixture, not an
// attestation that an arbitrary or incomplete production backup is trustworthy.
func TestPublisherRestoredWitnessKeepsReservation(t *testing.T) {
	s, p := publisherServiceFixture(t)
	remote := &evidenceRemote{serviceRemote: s.remote.(*serviceRemote), outcome: ObservedConfirmed}
	s.remote = remote
	id := testDispatchIdentity(p)
	if w := evidenceCall(t, s, p, id, "/v1/publish"); w.Code != http.StatusOK {
		t.Fatal(w.Code, w.Body.String())
	}
	source := s.journal.path
	root := filepath.Join(t.TempDir(), "restored-witness")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	slot := filepath.Join(root, dispatchSlot(p))
	if err := os.Mkdir(slot, 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"started.json", "invoked.json", "completed.json"} {
		data, err := os.ReadFile(filepath.Join(source, dispatchSlot(p), name))
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(slot, name), data, 0600); err != nil {
			t.Fatal(err)
		}
		copied, err := os.ReadFile(filepath.Join(slot, name))
		if err != nil || string(copied) != string(data) {
			t.Fatal("backup bytes drift", err)
		}
	}
	// Source no longer exists, so the subsequent reads cannot use its files.
	if err := os.RemoveAll(source); err != nil {
		t.Fatal(err)
	}
	journal, err := newDispatchJournal(root)
	if err != nil {
		t.Fatal(err)
	}
	s.journal = journal
	if w := evidenceCall(t, s, p, id, "/v1/publish"); w.Code != http.StatusConflict {
		t.Fatal("restored slot admitted replay", w.Code)
	}
	if w := evidenceCall(t, s, p, id, "/v1/dispatch-evidence"); w.Code != http.StatusOK {
		t.Fatal("restored witness did not validate", w.Code, w.Body.String())
	}
	if remote.publishCalls != 1 || remote.observeCalls != 1 {
		t.Fatal("restore replayed mutation")
	}
	// An incomplete v2 restore cannot omit its durable invocation marker.
	invokedPath := filepath.Join(slot, "invoked.json")
	invoked, err := os.ReadFile(invokedPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(invokedPath); err != nil {
		t.Fatal(err)
	}
	if w := evidenceCall(t, s, p, id, "/v1/dispatch-evidence"); w.Code != http.StatusConflict {
		t.Fatal("missing restored invocation accepted", w.Code)
	}
	if remote.observeCalls != 1 {
		t.Fatal("incomplete restore called upstream")
	}
	if err := os.WriteFile(invokedPath, invoked, 0600); err != nil {
		t.Fatal(err)
	}
	// Incomplete restores deny observation before contacting the upstream.
	if err := os.Remove(filepath.Join(slot, "completed.json")); err != nil {
		t.Fatal(err)
	}
	if w := evidenceCall(t, s, p, id, "/v1/dispatch-evidence"); w.Code != http.StatusConflict {
		t.Fatal("missing completion accepted", w.Code)
	}
	if remote.publishCalls != 1 || remote.observeCalls != 1 {
		t.Fatal("missing restored evidence called upstream")
	}
}
