package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/action"
	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/githubpublish"
	"github.com/jiying2007/engineering-platform/internal/processscope"
	corestore "github.com/jiying2007/engineering-platform/internal/store"
)

// These fixtures use real isolated PostgreSQL but explicitly synthetic witness
// bytes. The mTLS independent Publisher generation is tested separately; this
// suite is transaction/audit/readback proof, never live GitHub qualification.
func publicationDatabaseFixture(t *testing.T, state action.State) (*Store, githubpublish.PublicationRecoveryRequest, githubpublish.PublicationEvidence) {
	t.Helper()
	s := newIsolatedIntegrationStore(t)
	ctx := context.Background()
	runID := setupPostgresActionRun(t, s, fmt.Sprint(time.Now().UnixNano()))
	id := githubpublish.DispatchIdentity{OperationID: "publication-op", RunID: runID, ExecutionEpoch: 1, IdempotencyKey: "publication-reserved", RequestDigest: canonical.BytesDigest([]byte("exact-request"))}
	op := action.Operation{ID: id.OperationID, RunID: runID, ExecutionEpoch: 1, IdempotencyKey: id.IdempotencyKey, RequestDigest: id.RequestDigest, Action: githubpublish.Action, Capability: githubpublish.Capability, RiskClass: action.ControlledMutation, State: action.Planned, UpdatedAt: time.Now().UTC()}
	if err := s.CreateContext(ctx, op); err != nil {
		t.Fatal(err)
	}
	switch state {
	case action.Unknown, action.Manual, action.Reconciling, action.Dispatched, action.SafeToRetry:
		op.State = action.Dispatched
		if err := s.UpdateContext(ctx, op); err != nil {
			t.Fatal(err)
		}
		if state != action.Dispatched {
			op.State = action.Unknown
			if err := s.UpdateContext(ctx, op); err != nil {
				t.Fatal(err)
			}
		}
		if state == action.Reconciling || state == action.Manual || state == action.SafeToRetry {
			op.State = action.Reconciling
			if err := s.UpdateContext(ctx, op); err != nil {
				t.Fatal(err)
			}
		}
		if state == action.Manual {
			op.State = action.Manual
			if err := s.UpdateContext(ctx, op); err != nil {
				t.Fatal(err)
			}
		}
	}
	if state == action.SafeToRetry {
		// Seed an old-version retained state in this isolated fixture only.
		// Current normal authority must never mint SAFE_TO_RETRY or replay it.
		if _, err := s.pool.Exec(ctx, "UPDATE external_operations SET state='SAFE_TO_RETRY' WHERE operation_id=$1 AND state='RECONCILING'", op.ID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.BeginRecoveryContext(ctx, 0); err != nil {
		t.Fatal(err)
	}
	req := githubpublish.PublicationRecoveryRequest{Version: 1, Identity: id, RecoveryEpoch: 1}
	p := githubpublish.Plan{Version: 1, RunID: runID, Repository: "example/recovery", BaseRef: "main", BaseCommit: strings.Repeat("1", 40), Branch: "evidence/one", ResultCommit: strings.Repeat("2", 40), ResultDigest: canonical.BytesDigest([]byte("result")), ReceiptDigest: canonical.BytesDigest([]byte("receipt")), BundleDigest: canonical.BytesDigest([]byte("bundle")), BundleSize: 6, ExecutionID: strings.Repeat("3", 64)}
	at := time.Now().UTC().Add(-time.Second)
	start := githubpublish.DispatchStart{Version: 2, Incarnation: "sha256:" + strings.Repeat("a", 64), Identity: id, Plan: p, StartedAt: at}
	d, _ := canonical.Digest(start)
	pr := githubpublish.PublicationReceipt{Version: 1, Repository: p.Repository, BaseRef: p.BaseRef, BaseCommit: p.BaseCommit, Branch: p.Branch, ResultCommit: p.ResultCommit, PullRequestNumber: 12, PullRequestURL: "https://github.com/example/recovery/pull/12", PullRequestState: "open", PublicationOutcome: "CREATED"}
	completion := githubpublish.DispatchCompletion{Version: 2, StartDigest: d, Receipt: pr, ReturnedAt: at.Add(time.Millisecond)}
	// Controlled fixture; not selected-host process evidence.
	proofs := []processscope.Proof{{Version: 1, Mechanism: processscope.Mechanism, HostPID: 17, NamespaceID: 2, ParentNamespaceID: 1, StartedAt: at, ReapedAt: at, InitReaped: true}}
	proofDigest, _ := canonical.Digest(proofs)
	completion.GitQuiescence = &githubpublish.GitQuiescence{Version: 1, Proofs: proofs, Digest: proofDigest}
	pr.PublicationOutcome = "OBSERVED"
	evidence := githubpublish.PublicationEvidence{Version: 1, Start: start, Completion: completion, Observation: githubpublish.ObserveResult{Outcome: githubpublish.ObservedConfirmed, Receipt: pr}, ObservedAt: at.Add(2 * time.Millisecond)}
	return s, req, evidence
}
func TestPublicationRecoveryDatabaseTerminalReadbackAndNoReplay(t *testing.T) {
	for _, state := range []action.State{action.Dispatched, action.Unknown, action.Reconciling, action.Manual, action.SafeToRetry} {
		t.Run(string(state), func(t *testing.T) {
			s, req, evidence := publicationDatabaseFixture(t, state)
			ctx := context.Background()
			if _, err := s.CreateRecoveryProof(ctx, 1, plannedReconciler); !errors.Is(err, ErrRecoveryFactsUnresolved) {
				t.Fatal("orphan did not block proof", err)
			}
			result, err := s.ConfirmRecoveredPublication(ctx, plannedReconciler, req, evidence)
			if err != nil || result.Validate() != nil {
				t.Fatal(result, err)
			}
			repeat, err := s.ConfirmRecoveredPublication(ctx, plannedReconciler, req, evidence)
			if err != nil || repeat.EvidenceDigest != result.EvidenceDigest || !repeat.RecordedAt.Equal(result.RecordedAt) {
				t.Fatal("repeat changed result", err)
			}
			if _, err := s.ConfirmRecoveredPublication(ctx, "another-reconciler", req, evidence); !errors.Is(err, corestore.ErrConflict) {
				t.Fatal("conflicting principal accepted", err)
			}
			retained, err := s.GetRecoveredPublication(ctx, req.Identity.OperationID)
			if err != nil || retained.RequestDigest != result.RequestDigest {
				t.Fatal("readback failed", err)
			}
			original, err := s.GetByIdempotencyKeyContext(ctx, req.Identity.IdempotencyKey)
			if err != nil || original.State != action.Confirmed || original.RecoveryEpoch != 0 || original.RequestDigest != req.Identity.RequestDigest {
				t.Fatal("original identity changed", err)
			}
			original.State = action.Dispatched
			if err := s.UpdateContext(ctx, original); err == nil {
				t.Fatal("recovered operation redispatched")
			}
			var n int
			if err := s.pool.QueryRow(ctx, "SELECT count(*) FROM audit_events WHERE event_type='recovery.github.publication.confirmed'").Scan(&n); err != nil || n != 1 {
				t.Fatal("duplicate/missing audit", n, err)
			}
			// A different aggregate-proof creator must not let the operation's own
			// reconciler become the completion principal.
			proof, err := s.CreateRecoveryProof(ctx, 1, "urn:engineering-platform:operator:other-proof")
			if err != nil || !proof.Facts.Clear() {
				t.Fatal(proof, err)
			}
			if err := s.AuthorizeCompletion(ctx, plannedReconciler, 1); err == nil {
				t.Fatal("publication reconciler completed same Recovery")
			}
			if err := s.AuthorizeCompletion(ctx, "urn:engineering-platform:operator:completer", 1); err != nil {
				t.Fatal(err)
			}
			if _, err := s.CompleteRecoveryContext(ctx, 1, true); err != nil {
				t.Fatal(err)
			}
			if _, err := s.GetRecoveredPublication(ctx, req.Identity.OperationID); err != nil {
				t.Fatal("post-recovery readback lost", err)
			}
		})
	}
}
func TestPublicationRecoveryDatabaseRejectsInsufficientEvidence(t *testing.T) {
	for _, mode := range []string{"planned", "absent", "missing-completion", "wrong-original-epoch", "wrong-identity", "stale-recovery"} {
		t.Run(mode, func(t *testing.T) {
			state := action.Manual
			if mode == "planned" {
				state = action.Planned
			}
			s, req, e := publicationDatabaseFixture(t, state)
			ctx := context.Background()
			before := actionIdentityAuditCount(t, s, req.Identity.OperationID)
			switch mode {
			case "absent":
				e.Observation.Outcome = githubpublish.ObservedAbsent
			case "missing-completion":
				e.Completion = githubpublish.DispatchCompletion{}
			case "wrong-original-epoch":
				req.Identity.ExecutionEpoch++
			case "wrong-identity":
				req.Identity.IdempotencyKey = "other"
			case "stale-recovery":
				req.RecoveryEpoch++
			}
			if _, err := s.ConfirmRecoveredPublication(ctx, plannedReconciler, req, e); err == nil {
				t.Fatal("insufficient evidence accepted")
			}
			if actionIdentityAuditCount(t, s, req.Identity.OperationID) != before {
				t.Fatal("rejection appended audit")
			}
			original, err := s.GetContext(ctx, req.Identity.OperationID)
			if err != nil || original.State != state {
				t.Fatal("rejection changed ledger", err)
			}
		})
	}
}
