package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jiying2007/engineering-platform/internal/action"
	"github.com/jiying2007/engineering-platform/internal/githubpublish"
	"github.com/jiying2007/engineering-platform/internal/recovery"
	"testing"
	"time"
)

func TestPublicationRecoveryDatabaseOrdinaryConfirmationIsNotRecovery(t *testing.T) {
	s, req, evidence := publicationDatabaseFixture(t, action.Unknown)
	ctx := context.Background()
	provider := &postgresActionProvider{reconcile: action.ReconcileResult{Outcome: action.ReconcileConfirmed, ExternalRef: evidence.Observation.Receipt.PullRequestURL, ObservedState: "positive but no original completion"}}
	svc := action.NewService(nil, nil, provider, s)
	receipt, err := svc.Reconcile(ctx, req.Identity.OperationID)
	if !errors.Is(err, action.ErrExternalEffectUnresolved) || receipt.OperationID != "" {
		t.Fatalf("proofless normal confirmation bypassed Recovery: %+v %v", receipt, err)
	}
	stored, err := s.GetContext(ctx, req.Identity.OperationID)
	if err != nil || stored.State != action.Manual || stored.RequestDigest != req.Identity.RequestDigest {
		t.Fatal("original MANUAL reservation missing", stored, err)
	}
	if _, err := s.CreateRecoveryProof(ctx, req.RecoveryEpoch, plannedReconciler); !errors.Is(err, ErrRecoveryFactsUnresolved) {
		t.Fatal("proofless observation cleared Recovery", err)
	}
	// Existing independent evidence route remains intact, with original audit,
	// epochs and distinct reconciler/completer. Fixture evidence is synthetic.
	recovered, err := s.ConfirmRecoveredPublication(ctx, plannedReconciler, req, evidence)
	if err != nil || recovered.Validate() != nil || recovered.PreviousState != action.Manual {
		t.Fatal("witnessed recovery regressed", recovered, err)
	}
	if _, err := s.CreateRecoveryProof(ctx, req.RecoveryEpoch, "independent-proof"); err != nil {
		t.Fatal(err)
	}
	if err := s.AuthorizeCompletion(ctx, plannedReconciler, req.RecoveryEpoch); err == nil {
		t.Fatal("same reconciler completed Recovery")
	}
	if err := s.AuthorizeCompletion(ctx, "independent-completer", req.RecoveryEpoch); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CompleteRecoveryContext(ctx, req.RecoveryEpoch, true); err != nil {
		t.Fatal(err)
	}
	if provider.dispatchCalls != 0 || provider.reconcileCalls != 1 {
		t.Fatal("recovery replayed provider")
	}
}
func TestPublicationRecoveryDatabaseDirectConfirmationCannotBypassEvidence(t *testing.T) {
	for _, risk := range []action.RiskClass{action.Observe, action.ControlledMutation, action.HighRisk} {
		t.Run(string(risk), func(t *testing.T) {
			s, req, _ := publicationDatabaseFixture(t, action.Reconciling)
			ctx := context.Background()
			if risk != action.ControlledMutation { // Only private fixture setup, not application authority.
				if _, err := s.pool.Exec(ctx, "UPDATE external_operations SET risk_class=$1 WHERE operation_id=$2", string(risk), req.Identity.OperationID); err != nil {
					t.Fatal(err)
				}
			}
			original, err := s.GetContext(ctx, req.Identity.OperationID)
			if err != nil {
				t.Fatal(err)
			}
			before := actionIdentityAuditCount(t, s, original.ID)
			changed := original
			changed.State = action.Confirmed
			changed.UpdatedAt = time.Now().UTC()
			err = s.UpdateContext(ctx, changed)
			if risk == action.Observe {
				if err != nil {
					t.Fatal("read-only observation rejected", err)
				}
				return
			}
			if !errors.Is(err, action.ErrInvalidTransition) {
				t.Fatal("direct repository accepted unproven confirmation", err)
			}
			stored, err := s.GetContext(ctx, original.ID)
			if err != nil || stored != original || actionIdentityAuditCount(t, s, original.ID) != before {
				t.Fatal("rejected update changed record or audit", err)
			}
		})
	}
}

// Reproduce an old ordinary reconciliation in this random private test schema.
// Its audit/row survive unchanged; no production migration or SQL is provided.
func legacyObservedConfirmation(t *testing.T, s *Store, req githubpublish.PublicationRecoveryRequest, e githubpublish.PublicationEvidence) {
	t.Helper()
	ctx := context.Background()
	op, err := s.GetContext(ctx, req.Identity.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	op.State = action.Confirmed
	op.UpdatedAt = time.Now().UTC()
	op.ExternalRef = e.Observation.Receipt.PullRequestURL
	raw, err := json.Marshal(e.Observation.Receipt)
	if err != nil {
		t.Fatal(err)
	}
	op.ObservedState = string(raw)
	input, err := auditInput("action.state_changed", "ExternalOperation", op.ID, op)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Mutate(ctx, Mutation{Apply: func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, "UPDATE external_operations SET state='CONFIRMED',observed_state=$1,external_ref=$2,updated_at=$3 WHERE operation_id=$4 AND state='RECONCILING'", op.ObservedState, op.ExternalRef, op.UpdatedAt, op.ID)
		return err
	}, Audit: input})
	if err != nil {
		t.Fatal(err)
	}
}
func TestPublicationRecoveryDatabaseHistoricalObservedConfirmationBlocksProof(t *testing.T) {
	s, req, e := publicationDatabaseFixture(t, action.Reconciling)
	ctx := context.Background()
	legacyObservedConfirmation(t, s, req, e)
	original, err := s.GetContext(ctx, req.Identity.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	before := actionIdentityAuditCount(t, s, original.ID)
	if _, err := s.CreateRecoveryProof(ctx, req.RecoveryEpoch, plannedReconciler); !errors.Is(err, ErrRecoveryFactsUnresolved) {
		t.Fatal("historical bare observation still cleared Recovery", err)
	}
	if _, err := s.ReadOperationalStatus(ctx); !errors.Is(err, ErrRecoveryFactsUnresolved) {
		t.Fatal("unsupported confirmation concealed in operational status", err)
	}
	stored, err := s.GetContext(ctx, original.ID)
	if err != nil || stored != original || actionIdentityAuditCount(t, s, original.ID) != before {
		t.Fatal("historical row or audit rewritten", err)
	}
	state, err := s.GetRecoveryContext(ctx)
	if err != nil || state.Mode != recovery.RecoveryReconciliation {
		t.Fatal("Recovery completed without evidence", err)
	}
}

func TestPublicationRecoveryDatabaseOriginalDispatchConfirmationStillValid(t *testing.T) {
	s, req, e := publicationDatabaseFixture(t, action.Dispatched)
	ctx := context.Background()
	original, err := s.GetContext(ctx, req.Identity.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	// Settle an original returned success after Recovery Begin: the current epoch
	// must not erase the original dispatch's independently bounded settlement.
	completed := original
	completed.State = action.Confirmed
	completed.UpdatedAt = time.Now().UTC()
	raw, err := json.Marshal(e.Completion.Receipt)
	if err != nil {
		t.Fatal(err)
	}
	completed.ObservedState = string(raw)
	completed.ExternalRef = e.Completion.Receipt.PullRequestURL
	if err := s.UpdateContext(ctx, completed); err != nil {
		t.Fatal("original settlement lost", err)
	}
	if _, err := s.ReadOperationalStatus(ctx); err != nil {
		t.Fatal("original success hidden from status", err)
	}
	proof, err := s.CreateRecoveryProof(ctx, req.RecoveryEpoch, plannedReconciler)
	if err != nil || !proof.Facts.Clear() {
		t.Fatal("original success no longer clears facts", proof, err)
	}
	if _, err := s.CompleteRecoveryContext(ctx, req.RecoveryEpoch, true); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetRecoveredPublication(ctx, original.ID); err == nil {
		t.Fatal("ordinary success manufactured an independent Recovery receipt")
	}
}

// A plausible audit-event count cannot certify a historical observation, even
// after its row is terminal. Enrichment requires the original complete witness;
// it keeps every old event and writes one new, separately attributed receipt.
func TestPublicationRecoveryDatabaseHistoricalConfirmationEnrichment(t *testing.T) {
	s, req, e := publicationDatabaseFixture(t, action.Reconciling)
	ctx := context.Background()
	legacyObservedConfirmation(t, s, req, e)
	before := actionIdentityAuditCount(t, s, req.Identity.OperationID)
	for _, kind := range []string{"missing-completion", "absence", "retarget", "stale"} {
		t.Run(kind, func(t *testing.T) {
			bad, request := e, req
			switch kind {
			case "missing-completion":
				bad.Completion = githubpublish.DispatchCompletion{}
			case "absence":
				bad.Observation.Outcome = githubpublish.ObservedAbsent
			case "retarget":
				bad.Observation.Receipt.PullRequestNumber++
			case "stale":
				request.RecoveryEpoch++
			}
			if _, err := s.ConfirmRecoveredPublication(ctx, plannedReconciler, request, bad); err == nil {
				t.Fatal("unproven historical confirmation enriched")
			}
			if actionIdentityAuditCount(t, s, req.Identity.OperationID) != before {
				t.Fatal("rejection changed audit")
			}
		})
	}
	if _, err := s.pool.Exec(ctx, "UPDATE external_operations SET external_ref='retargeted-original' WHERE operation_id=$1", req.Identity.OperationID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ConfirmRecoveredPublication(ctx, plannedReconciler, req, e); err == nil {
		t.Fatal("retargeted historical record was silently overwritten")
	}
	if actionIdentityAuditCount(t, s, req.Identity.OperationID) != before {
		t.Fatal("retarget rejection changed original audit")
	}
	if _, err := s.pool.Exec(ctx, "UPDATE external_operations SET external_ref=$2 WHERE operation_id=$1", req.Identity.OperationID, e.Completion.Receipt.PullRequestURL); err != nil {
		t.Fatal(err)
	}
	retained, err := s.ConfirmRecoveredPublication(ctx, plannedReconciler, req, e)
	if err != nil || retained.PreviousState != action.Confirmed || retained.Validate() != nil {
		t.Fatal("full original proof could not enrich exact historical PR", retained, err)
	}
	if actionIdentityAuditCount(t, s, req.Identity.OperationID) != before+1 {
		t.Fatal("old audit changed or duplicate new audit")
	}
	again, err := s.ConfirmRecoveredPublication(ctx, plannedReconciler, req, e)
	if err != nil || again.RequestDigest != retained.RequestDigest || !again.RecordedAt.Equal(retained.RecordedAt) {
		t.Fatal("exact repeat changed", again, err)
	}
	if _, err := s.ConfirmRecoveredPublication(ctx, "other-reconciler", req, e); err == nil {
		t.Fatal("conflicting principal accepted")
	}
	if _, err := s.GetRecoveredPublication(ctx, req.Identity.OperationID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateRecoveryProof(ctx, req.RecoveryEpoch, "separate-proof"); err != nil {
		t.Fatal(err)
	}
	if err := s.AuthorizeCompletion(ctx, plannedReconciler, req.RecoveryEpoch); err == nil {
		t.Fatal("enrichment reconciler self-completed")
	}
	if err := s.AuthorizeCompletion(ctx, "independent-completer", req.RecoveryEpoch); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CompleteRecoveryContext(ctx, req.RecoveryEpoch, true); err != nil {
		t.Fatal(err)
	}
	op, err := s.GetByIdempotencyKeyContext(ctx, req.Identity.IdempotencyKey)
	if err != nil || op.State != action.Confirmed || op.RecoveryEpoch != req.Identity.RecoveryEpoch {
		t.Fatal("original identity changed", op, err)
	}
	op.State = action.Dispatched
	if err := s.UpdateContext(ctx, op); !errors.Is(err, action.ErrInvalidTransition) && !errors.Is(err, recovery.ErrStaleEpoch) {
		t.Fatal("enriched operation was not denied by original state/epoch gate", err)
	}
	if githubpublish.RecoverablePublicationState(action.Confirmed) {
		t.Fatal("historical success leaked into non-invocation abandonment")
	}
}

func originalReturnedConfirmation(t *testing.T, s *Store, req githubpublish.PublicationRecoveryRequest, e githubpublish.PublicationEvidence) action.Operation {
	t.Helper()
	ctx := context.Background()
	op, err := s.GetContext(ctx, req.Identity.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	op.State = action.Confirmed
	op.UpdatedAt = time.Now().UTC()
	raw, err := json.Marshal(e.Completion.Receipt)
	if err != nil {
		t.Fatal(err)
	}
	op.ObservedState = string(raw)
	op.ExternalRef = e.Completion.Receipt.PullRequestURL
	if err := s.UpdateContext(ctx, op); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetContext(ctx, op.ID)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestPublicationRecoveryDatabaseConfirmationProvenanceCorruption(t *testing.T) {
	for _, kind := range []string{"missing-marker", "wrong-payload", "wrong-event-digest", "wrong-row", "wrong-time", "wrong-risk", "wrong-state", "missing-row", "duplicate-marker", "old-count-lookalike"} {
		t.Run(kind, func(t *testing.T) {
			s, req, e := publicationDatabaseFixture(t, action.Dispatched)
			ctx := context.Background()
			op := originalReturnedConfirmation(t, s, req, e)
			if _, err := s.CreateRecoveryProof(ctx, req.RecoveryEpoch, "proof-before-damage"); err != nil {
				t.Fatal(err)
			}
			var q string
			switch kind {
			case "missing-marker", "old-count-lookalike":
				q = "UPDATE audit_events SET causation_id=NULL WHERE aggregate_id=$1 AND causation_id='action.original-dispatch-confirmation.v1'"
			case "wrong-payload":
				q = "UPDATE audit_events SET payload_digest='sha256:wrong' WHERE aggregate_id=$1 AND causation_id='action.original-dispatch-confirmation.v1'"
			case "wrong-event-digest":
				q = "UPDATE audit_events SET event_digest='sha256:wrong' WHERE aggregate_id=$1 AND causation_id='action.original-dispatch-confirmation.v1'"
			case "wrong-row":
				q = "UPDATE external_operations SET observed_state='replacement' WHERE operation_id=$1"
			case "wrong-time":
				q = "UPDATE external_operations SET updated_at=updated_at+interval '1 microsecond' WHERE operation_id=$1"
			case "wrong-risk":
				q = "UPDATE external_operations SET risk_class='OBSERVE' WHERE operation_id=$1"
			case "wrong-state":
				q = "UPDATE external_operations SET state='MANUAL' WHERE operation_id=$1"
			case "missing-row":
				q = "DELETE FROM external_operations WHERE operation_id=$1"
			case "duplicate-marker":
				input, err := auditInput("action.state_changed", "ExternalOperation", op.ID, op)
				if err != nil {
					t.Fatal(err)
				}
				input.CausationID = originalConfirmationCause
				if _, err := s.Mutate(ctx, Mutation{Audit: input}); err != nil {
					t.Fatal(err)
				}
			}
			if q != "" {
				if _, err := s.pool.Exec(ctx, q, op.ID); err != nil {
					t.Fatal(err)
				}
			}
			if kind == "old-count-lookalike" {
				var planned, changed int
				if err := s.pool.QueryRow(ctx, "SELECT count(*) FILTER (WHERE event_type='action.planned'),count(*) FILTER (WHERE event_type='action.state_changed') FROM audit_events WHERE aggregate_id=$1", op.ID).Scan(&planned, &changed); err != nil || planned != 1 || changed != 2 {
					t.Fatal("fixture does not reproduce provisional count rule", planned, changed, err)
				}
			}
			before := actionIdentityAuditCount(t, s, op.ID)
			if _, err := s.ReadOperationalStatus(ctx); !errors.Is(err, ErrRecoveryFactsUnresolved) {
				t.Fatal("corruption hidden by status", err)
			}
			if _, err := s.CompleteRecoveryContext(ctx, req.RecoveryEpoch, true); !errors.Is(err, ErrRecoveryFactsUnresolved) {
				t.Fatal("stale proof cleared corrupted original confirmation", err)
			}
			if actionIdentityAuditCount(t, s, op.ID) != before {
				t.Fatal("failed completion changed audit")
			}
		})
	}
}
