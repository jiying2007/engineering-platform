package postgres

import (
	"context"
	"errors"
	"github.com/jiying2007/engineering-platform/internal/action"
	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/githubpublish"
	corestore "github.com/jiying2007/engineering-platform/internal/store"
	"testing"
	"time"
)

// Isolated real SQL with synthetic Publisher witnesses; never live GitHub proof.
func noInvocationDatabaseFixture(t *testing.T, state action.State, version int) (*Store, githubpublish.PublicationRecoveryRequest, githubpublish.NonInvocationEvidence) {
	s, request, positive := publicationDatabaseFixture(t, state)
	start := positive.Start
	start.Version = 2
	start.Incarnation = canonical.BytesDigest([]byte("synthetic-local-slot"))
	digest, _ := canonical.Digest(start)
	seal := githubpublish.DispatchNonInvocation{Kind: "PUBLISH_NOT_INVOKED_V1", StartDigest: digest, SealedAt: start.StartedAt.Add(time.Millisecond)}
	e := githubpublish.NonInvocationEvidence{Version: 1, Start: start, Seal: seal}
	if version == 2 {
		invocation := &githubpublish.DispatchInvocation{Version: 1, StartDigest: digest, ArmedAt: start.StartedAt.Add(time.Microsecond)}
		e.Version = 2
		e.Invocation = invocation
		e.Seal.Kind = "PUBLISH_NOT_INVOKED_AFTER_ARM_V2"
		e.Seal.InvocationDigest, _ = canonical.Digest(*invocation)
	}
	if e.Validate() != nil {
		t.Fatal("invalid synthetic evidence fixture", version)
	}
	return s, request, e
}
func TestNoInvocationDatabaseTerminalReadbackAndNoReplay(t *testing.T) {
	forNoInvocationVersions(t, testNoInvocationDatabaseTerminalReadbackAndNoReplay)
}
func testNoInvocationDatabaseTerminalReadbackAndNoReplay(t *testing.T, version int) {
	for _, state := range []action.State{action.Dispatched, action.Unknown, action.Reconciling, action.Manual, action.SafeToRetry} {
		t.Run(string(state), func(t *testing.T) {
			s, req, evidence := noInvocationDatabaseFixture(t, state, version)
			ctx := context.Background()
			if _, err := s.CreateRecoveryProof(ctx, 1, plannedReconciler); !errors.Is(err, ErrRecoveryFactsUnresolved) {
				t.Fatal("orphan did not block proof", err)
			}
			result, err := s.AbandonUninvokedPublication(ctx, plannedReconciler, req, evidence)
			if err != nil || result.Validate() != nil {
				t.Fatal(result, err)
			}
			repeat, err := s.AbandonUninvokedPublication(ctx, plannedReconciler, req, evidence)
			if err != nil || repeat.EvidenceDigest != result.EvidenceDigest || !repeat.RecordedAt.Equal(result.RecordedAt) {
				t.Fatal("repeat changed result", err)
			}
			if _, err := s.AbandonUninvokedPublication(ctx, "another-reconciler", req, evidence); !errors.Is(err, corestore.ErrConflict) {
				t.Fatal("conflicting principal accepted", err)
			}
			retained, err := s.GetUninvokedPublication(ctx, req.Identity.OperationID)
			if err != nil || retained.RequestDigest != result.RequestDigest {
				t.Fatal("readback failed", err)
			}
			original, err := s.GetByIdempotencyKeyContext(ctx, req.Identity.IdempotencyKey)
			if err != nil || original.State != action.AbandonedReconciled || original.RecoveryEpoch != 0 || original.RequestDigest != req.Identity.RequestDigest {
				t.Fatal("original identity changed", err)
			}
			if original.ExternalRef != "" {
				t.Fatal("non-invocation manufactured a remote success")
			}
			var success []byte
			if err := s.pool.QueryRow(ctx, "SELECT receipt_json FROM external_operations WHERE operation_id=$1", original.ID).Scan(&success); err != nil || len(success) != 0 {
				t.Fatal("success receipt manufactured", err)
			}
			original.State = action.Dispatched
			if err := s.UpdateContext(ctx, original); err == nil {
				t.Fatal("recovered operation redispatched")
			}
			var n int
			if err := s.pool.QueryRow(ctx, "SELECT count(*) FROM audit_events WHERE event_type='recovery.github.publication.not_invoked'").Scan(&n); err != nil || n != 1 {
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
			if _, err := s.GetUninvokedPublication(ctx, req.Identity.OperationID); err != nil {
				t.Fatal("post-recovery readback lost", err)
			}
		})
	}
}
func TestNoInvocationDatabaseRejectsInsufficientEvidence(t *testing.T) {
	forNoInvocationVersions(t, testNoInvocationDatabaseRejectsInsufficientEvidence)
}
func testNoInvocationDatabaseRejectsInsufficientEvidence(t *testing.T, version int) {
	for _, mode := range []string{"planned", "legacy", "missing-seal", "wrong-seal", "wrong-identity", "stale-recovery", "success-ref"} {
		t.Run(mode, func(t *testing.T) {
			state := action.Manual
			if mode == "planned" {
				state = action.Planned
			}
			s, req, e := noInvocationDatabaseFixture(t, state, version)
			ctx := context.Background()
			before := actionIdentityAuditCount(t, s, req.Identity.OperationID)
			switch mode {
			case "legacy":
				e.Start.Version = 1
			case "missing-seal":
				e.Seal = githubpublish.DispatchNonInvocation{}
			case "wrong-seal":
				e.Seal.StartDigest = canonical.BytesDigest([]byte("not original"))
			case "wrong-identity":
				req.Identity.IdempotencyKey = "changed"
			case "stale-recovery":
				req.RecoveryEpoch++
			case "success-ref":
				if _, err := s.pool.Exec(ctx, "UPDATE external_operations SET external_ref='conflicting-reference' WHERE operation_id=$1", req.Identity.OperationID); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := s.AbandonUninvokedPublication(ctx, plannedReconciler, req, e); err == nil {
				t.Fatal("insufficient non-invocation accepted")
			}
			op, err := s.GetContext(ctx, req.Identity.OperationID)
			if err != nil || op.State != state || actionIdentityAuditCount(t, s, op.ID) != before {
				t.Fatal("rejection mutated ledger", op, err)
			}
		})
	}
}

// Both evidence versions use the same production transaction/audit/state path.
// No version/environment flag is introduced into production code.
func forNoInvocationVersions(t *testing.T, run func(*testing.T, int)) {
	for _, version := range []int{1, 2} {
		name := "before-arm"
		if version == 2 {
			name = "cancelled-after-arm"
		}
		t.Run(name, func(t *testing.T) { run(t, version) })
	}
}
