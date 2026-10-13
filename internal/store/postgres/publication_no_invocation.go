package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jiying2007/engineering-platform/internal/action"
	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/githubpublish"
	"github.com/jiying2007/engineering-platform/internal/recovery"
	corestore "github.com/jiying2007/engineering-platform/internal/store"
	"github.com/jiying2007/engineering-platform/internal/strictjson"
)

// AbandonUninvokedPublication accepts only Core-fetched independent Publisher
// evidence for a v2 pre-invocation slot, sealed under its inode lock. DISPATCHED
// in Core is not evidence that Publish was invoked. There is no remote absence
// query and no interpretation of v1/historical/missing/armed witnesses. Reuse the
// existing non-success state and reconciliation_json, never a success receipt.
func (s *Store) AbandonUninvokedPublication(ctx context.Context, reconciler string, request githubpublish.PublicationRecoveryRequest, evidence githubpublish.NonInvocationEvidence) (githubpublish.NonInvocationRecoveryReceipt, error) {
	var zero githubpublish.NonInvocationRecoveryReceipt
	if s == nil || s.pool == nil || request.Validate() != nil || evidence.Validate() != nil || evidence.Start.Identity != request.Identity {
		return zero, githubpublish.ErrDispatchEvidenceUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return zero, err
	}
	defer rollbackOutbox(tx)
	var epoch uint64
	var mode string
	if err := tx.QueryRow(ctx, `SELECT recovery_epoch,recovery_mode FROM platform_state WHERE singleton_id=true FOR UPDATE`).Scan(&epoch, &mode); err != nil {
		return zero, err
	}
	if epoch != request.RecoveryEpoch {
		return zero, recovery.ErrStaleEpoch
	}
	if mode != string(recovery.RecoveryReconciliation) {
		return zero, recovery.ErrReconciliationRequired
	}
	var identity githubpublish.DispatchIdentity
	var actionName, risk, capability, state string
	var raw, reconciliation []byte
	var originalObservation, externalRef string
	const query = `SELECT operation_id,run_id,execution_epoch,recovery_epoch,idempotency_key,request_digest,action,risk_class,capability,state,receipt_json,reconciliation_json,COALESCE(observed_state,''),COALESCE(external_ref,'') FROM external_operations WHERE operation_id=$1 FOR UPDATE`
	if err := tx.QueryRow(ctx, query, request.Identity.OperationID).Scan(&identity.OperationID, &identity.RunID, &identity.ExecutionEpoch, &identity.RecoveryEpoch, &identity.IdempotencyKey, &identity.RequestDigest, &actionName, &risk, &capability, &state, &raw, &reconciliation, &originalObservation, &externalRef); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return zero, corestore.ErrNotFound
		}
		return zero, err
	}
	if identity != request.Identity || actionName != githubpublish.Action || capability != githubpublish.Capability || risk != string(action.ControlledMutation) {
		return zero, corestore.ErrConflict
	}
	requestDigest, _ := request.Digest()
	if state == string(action.AbandonedReconciled) && len(reconciliation) > 0 {
		retained, err := readUninvokedPublication(ctx, tx, identity.OperationID)
		if err != nil {
			return zero, err
		}
		if retained.RequestDigest != requestDigest || retained.Reconciler != reconciler {
			return zero, corestore.ErrConflict
		}
		return retained, nil // Read-only exact repeat. Deferred rollback releases locks.
	}
	if !githubpublish.RecoverablePublicationState(action.State(state)) || len(raw) != 0 || len(reconciliation) != 0 || externalRef != "" {
		return zero, corestore.ErrConflict
	}
	var now time.Time
	if err := tx.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&now); err != nil {
		return zero, err
	}
	evidenceDigest, _ := canonical.Digest(evidence)
	retained := githubpublish.NonInvocationRecoveryReceipt{Kind: githubpublish.NonInvocationRecoveryKind, Request: request, RequestDigest: requestDigest, Reconciler: reconciler, PreviousState: action.State(state), Evidence: evidence, EvidenceDigest: evidenceDigest, RecordedAt: now, PreviousObservation: originalObservation}
	if err := retained.Validate(); err != nil {
		return zero, err
	}
	raw, err = json.Marshal(retained)
	if err != nil {
		return zero, err
	}
	if len(raw) > 128<<10 {
		return zero, githubpublish.ErrDispatchEvidenceUnavailable
	}
	input, err := auditInput("recovery.github.publication.not_invoked", "ExternalOperation", request.Identity.OperationID, retained)
	if err != nil {
		return zero, err
	}
	// Every identity is already compared under the row lock; additionally retain
	// the expected prior state and original empty receipt slot in the UPDATE.
	tag, err := tx.Exec(ctx, `UPDATE external_operations SET state='ABANDONED_RECONCILED',reconciliation_json=$2::jsonb,updated_at=$3 WHERE operation_id=$1 AND state=$4 AND receipt_json IS NULL AND reconciliation_json IS NULL`, identity.OperationID, string(raw), now, state)
	if err != nil {
		return zero, err
	}
	if tag.RowsAffected() != 1 {
		return zero, corestore.ErrConflict
	}
	if _, err := appendAudit(ctx, tx, input, now); err != nil {
		return zero, err
	}
	if err := commitAuthority(ctx, tx); err != nil {
		return zero, err
	} // Preserve COMMIT unknown; no retry.
	return retained, nil
}
func (s *Store) GetUninvokedPublication(ctx context.Context, id string) (githubpublish.NonInvocationRecoveryReceipt, error) {
	var zero githubpublish.NonInvocationRecoveryReceipt
	if s == nil || s.pool == nil || id == "" || len(id) > 256 {
		return zero, corestore.ErrNotFound
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return readUninvokedPublication(ctx, s.pool, id)
}

// Every readback, exact repeat, Recovery proof and completion uses the same
// immutable row/audit projection. Earlier PLANNED-only receipts remain distinct.
const uninvokedPublicationSelect = `SELECT e.reconciliation_json,e.operation_id,e.run_id,
 e.execution_epoch,e.recovery_epoch,e.action,e.risk_class,e.capability,
 e.idempotency_key,e.request_digest,e.state,COALESCE(e.external_ref,''),
 COALESCE(e.observed_state,''),e.updated_at,e.receipt_json,
 (SELECT count(*) FROM audit_events a WHERE a.event_type='recovery.github.publication.not_invoked'
   AND a.aggregate_type='ExternalOperation' AND a.aggregate_id=e.operation_id),
 COALESCE((SELECT min(a.payload_digest) FROM audit_events a
   WHERE a.event_type='recovery.github.publication.not_invoked'
   AND a.aggregate_type='ExternalOperation' AND a.aggregate_id=e.operation_id),'')
 FROM external_operations e`

func readUninvokedPublication(ctx context.Context, q publicationRowQuery, id string) (githubpublish.NonInvocationRecoveryReceipt, error) {
	return scanUninvokedPublication(q.QueryRow(ctx, uninvokedPublicationSelect+" WHERE e.operation_id=$1", id))
}
func scanUninvokedPublication(row interface{ Scan(...any) error }) (githubpublish.NonInvocationRecoveryReceipt, error) {
	var zero githubpublish.NonInvocationRecoveryReceipt
	var raw, success []byte
	var op action.Operation
	var risk, state, auditDigest string
	var updated time.Time
	var count uint64
	err := row.Scan(&raw, &op.ID, &op.RunID, &op.ExecutionEpoch, &op.RecoveryEpoch, &op.Action, &risk, &op.Capability,
		&op.IdempotencyKey, &op.RequestDigest, &state, &op.ExternalRef, &op.ObservedState, &updated, &success, &count, &auditDigest)
	if errors.Is(err, pgx.ErrNoRows) {
		return zero, corestore.ErrNotFound
	}
	if err != nil {
		return zero, err
	}
	if len(raw) == 0 && count == 0 {
		return zero, corestore.ErrNotFound
	}
	// Existing valid PLANNED receipts belong to their existing endpoint, not this one.
	if count == 0 && len(raw) > 0 {
		var legacy recovery.ActionPlannedAbandonReceipt
		if strictjson.Decode(raw, &legacy) == nil && legacy.Validate() == nil {
			return zero, corestore.ErrNotFound
		}
	}
	if len(raw) == 0 || len(raw) > 128<<10 || len(success) > 0 || state != string(action.AbandonedReconciled) || op.ExternalRef != "" || count != 1 {
		return zero, githubpublish.ErrDispatchEvidenceUnavailable
	}
	op.RiskClass = action.RiskClass(risk)
	identity, err := githubpublish.IdentityForOperation(op)
	if err != nil {
		return zero, err
	}
	var r githubpublish.NonInvocationRecoveryReceipt
	if strictjson.Decode(raw, &r) != nil || r.Validate() != nil || r.Request.Identity != identity ||
		r.PreviousObservation != op.ObservedState || !updated.Equal(r.RecordedAt) {
		return zero, githubpublish.ErrDispatchEvidenceUnavailable
	}
	digest, err := canonical.Digest(r)
	if err != nil || digest != auditDigest {
		return zero, githubpublish.ErrDispatchEvidenceUnavailable
	}
	return r, nil
}
func validateUninvokedPublications(ctx context.Context, tx pgx.Tx) error {
	var orphan bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM audit_events a LEFT JOIN external_operations e ON e.operation_id=a.aggregate_id
  WHERE a.event_type='recovery.github.publication.not_invoked' AND a.aggregate_type='ExternalOperation' AND e.operation_id IS NULL)`).Scan(&orphan); err != nil {
		return err
	}
	if orphan {
		return githubpublish.ErrDispatchEvidenceUnavailable
	}
	rows, err := tx.Query(ctx, uninvokedPublicationSelect+` WHERE
 (e.reconciliation_json IS NOT NULL AND e.reconciliation_json->>'kind' IS DISTINCT FROM 'RECOVERY_ACTION_PLANNED_ABANDONMENT_V1')
 OR EXISTS(SELECT 1 FROM audit_events a WHERE a.event_type='recovery.github.publication.not_invoked'
 AND a.aggregate_type='ExternalOperation' AND a.aggregate_id=e.operation_id)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		if _, err := scanUninvokedPublication(rows); err != nil {
			return err
		}
	}
	return rows.Err()
}
