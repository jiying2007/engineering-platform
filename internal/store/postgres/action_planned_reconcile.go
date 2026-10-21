package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jiying2007/engineering-platform/internal/action"
	"github.com/jiying2007/engineering-platform/internal/audit"
	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/recovery"
	corestore "github.com/jiying2007/engineering-platform/internal/store"
	"github.com/jiying2007/engineering-platform/internal/strictjson"
)

// ReconcilePlannedActionAbandoned is intentionally narrower than generic
// UNKNOWN reconciliation. Lock ordering is platform -> external operation.
// Recovery Begin has already fenced every new dispatch with the same platform
// row lock. A still-PLANNED row was therefore never an authorized remote call.
//
// DISPATCHED, UNKNOWN, RECONCILING and MANUAL are deliberately NOT accepted:
// the remote effect can be in flight or historically committed. Those states
// require independent provider-specific observation and cannot be reduced
// to a no-effect assumption by this procedure.
func (s *Store) ReconcilePlannedActionAbandoned(ctx context.Context, reconciler string, request recovery.ActionPlannedAbandonRequest) (recovery.ActionPlannedAbandonReceipt, error) {
	var zero recovery.ActionPlannedAbandonReceipt
	if s == nil || s.pool == nil || request.Validate() != nil ||
		reconciler == "" || len(reconciler) > 256 {
		return zero, fmt.Errorf("valid bounded planned Action reconciliation required")
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

	var runID, idempotencyKey, originalDigest, state, externalRef, observedState string
	var executionEpoch, originalRecoveryEpoch uint64
	var effectReceipt, reconciliationRaw []byte
	const query = `SELECT run_id,execution_epoch,recovery_epoch,idempotency_key,request_digest,
 state,COALESCE(external_ref,''),COALESCE(observed_state,''),receipt_json,reconciliation_json
 FROM external_operations WHERE operation_id=$1 FOR UPDATE`
	if err := tx.QueryRow(ctx, query, request.OperationID).Scan(
		&runID, &executionEpoch, &originalRecoveryEpoch, &idempotencyKey,
		&originalDigest, &state, &externalRef, &observedState,
		&effectReceipt, &reconciliationRaw,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return zero, corestore.ErrNotFound
		}
		return zero, err
	}
	if runID != request.RunID || executionEpoch != request.ExecutionEpoch ||
		originalRecoveryEpoch != request.OriginalRecoveryEpoch ||
		idempotencyKey != request.IdempotencyKey ||
		originalDigest != request.OriginalRequestDigest {
		return zero, corestore.ErrConflict
	}

	requestDigest, err := request.Digest()
	if err != nil {
		return zero, err
	}
	if state == string(action.AbandonedReconciled) {
		retained, err := readPlannedActionAbandonReceipt(ctx, tx, request.OperationID)
		if err != nil {
			return zero, err
		}
		if retained.RequestDigest != requestDigest || retained.Reconciler != reconciler {
			return zero, corestore.ErrConflict
		}
		return retained, nil // Read-only exact repeat; rollback releases locks.
	}
	if state != string(action.Planned) || len(effectReceipt) != 0 ||
		externalRef != "" || observedState != "" || len(reconciliationRaw) != 0 {
		return zero, corestore.ErrConflict
	}
	// A restored old row must not overwrite a surviving original settlement
	// audit. Its missing receipt is corruption, not first-time reconciliation.
	var priorAudit bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM audit_events
 WHERE aggregate_type='ExternalOperation' AND aggregate_id=$1
 AND event_type='recovery.action.planned.abandoned')`, request.OperationID).Scan(&priorAudit); err != nil {
		return zero, err
	}
	if priorAudit {
		return zero, invalidPlannedActionReceipt()
	}

	var now time.Time
	if err := tx.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&now); err != nil {
		return zero, err
	}
	retained := recovery.ActionPlannedAbandonReceipt{
		Kind:    recovery.ActionPlannedAbandonmentKind,
		Request: request, RequestDigest: requestDigest,
		Reconciler: reconciler, PreviousState: string(action.Planned),
		CreatedAt: now,
	}
	if err := retained.Validate(); err != nil {
		return zero, err
	}
	raw, err := json.Marshal(retained)
	if err != nil {
		return zero, err
	}
	auditInput, err := auditInput("recovery.action.planned.abandoned", "ExternalOperation", request.OperationID, retained)
	if err != nil {
		return zero, err
	}
	tag, err := tx.Exec(ctx, `UPDATE external_operations
 SET state='ABANDONED_RECONCILED',reconciliation_json=$2::jsonb,updated_at=$3
 WHERE operation_id=$1 AND state='PLANNED' AND reconciliation_json IS NULL
 AND external_ref IS NULL AND observed_state IS NULL AND receipt_json IS NULL`,
		request.OperationID, string(raw), now)
	if err != nil {
		return zero, err
	}
	if tag.RowsAffected() != 1 {
		return zero, corestore.ErrConflict
	}
	if _, err := appendAudit(ctx, tx, auditInput, now); err != nil {
		return zero, err
	}
	if err := commitAuthority(ctx, tx); err != nil {
		return zero, err
	}
	return retained, nil
}

// Readback is the only portable outcome after a response/transaction commit
// becomes unknown. It never grants replay or production qualification.
func (s *Store) GetPlannedActionAbandonReceipt(ctx context.Context, operationID string) (recovery.ActionPlannedAbandonReceipt, error) {
	var zero recovery.ActionPlannedAbandonReceipt
	if s == nil || s.pool == nil || operationID == "" || len(operationID) > 256 {
		return zero, corestore.ErrNotFound
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return readPlannedActionAbandonReceipt(ctx, s.pool, operationID)
}

// The original reservation, immutable receipt and unique causal audit are one
// projection for direct readback, exact repeat and every aggregate Recovery gate.
// LIMIT 2 detects duplicate markers without accepting a count as provenance.
const plannedActionAbandonSelect = `SELECT e.reconciliation_json,e.operation_id,e.run_id,
 e.execution_epoch,e.recovery_epoch,e.idempotency_key,e.request_digest,e.state,
 COALESCE(e.external_ref,''),COALESCE(e.observed_state,''),e.updated_at,e.receipt_json,
 COALESCE((SELECT jsonb_agg(to_jsonb(a)) FROM (
   SELECT sequence,event_type AS type,aggregate_type,aggregate_id,payload_digest,
          COALESCE(previous_digest,'') AS previous_digest,event_digest AS digest,
          COALESCE(correlation_id,'') AS correlation_id,
          COALESCE(causation_id,'') AS causation_id,created_at
   FROM audit_events WHERE aggregate_type='ExternalOperation'
     AND aggregate_id=e.operation_id AND event_type='recovery.action.planned.abandoned'
   ORDER BY sequence LIMIT 2
 ) a),'[]'::jsonb)
 FROM external_operations e`

func readPlannedActionAbandonReceipt(ctx context.Context, q publicationRowQuery, id string) (recovery.ActionPlannedAbandonReceipt, error) {
	return scanPlannedActionAbandonReceipt(q.QueryRow(ctx, plannedActionAbandonSelect+" WHERE e.operation_id=$1", id))
}

func scanPlannedActionAbandonReceipt(row interface{ Scan(...any) error }) (recovery.ActionPlannedAbandonReceipt, error) {
	var zero recovery.ActionPlannedAbandonReceipt
	var raw, success, auditRaw []byte
	var operationID, runID, key, originalDigest, state, externalRef, observed string
	var executionEpoch, originalRecoveryEpoch uint64
	var updated time.Time
	err := row.Scan(&raw, &operationID, &runID, &executionEpoch, &originalRecoveryEpoch,
		&key, &originalDigest, &state, &externalRef, &observed, &updated, &success, &auditRaw)
	if errors.Is(err, pgx.ErrNoRows) {
		return zero, corestore.ErrNotFound
	}
	if err != nil {
		return zero, err
	}
	var events []json.RawMessage
	if len(auditRaw) == 0 || len(auditRaw) > 16<<10 || json.Unmarshal(auditRaw, &events) != nil {
		return zero, invalidPlannedActionReceipt()
	}
	if len(raw) == 0 && len(events) == 0 && state != string(action.AbandonedReconciled) {
		return zero, corestore.ErrNotFound
	}
	// A publication non-invocation receipt belongs to its own endpoint.
	// Aggregate callers never route that kind here unless a PLANNED audit also
	// survives, in which case the contradictory provenance must be rejected.
	if len(events) == 0 && len(raw) > 0 {
		var kind struct {
			Kind string `json:"kind"`
		}
		if json.Unmarshal(raw, &kind) == nil && kind.Kind == "RECOVERY_GITHUB_PUBLICATION_NOT_INVOKED_V1" {
			return zero, corestore.ErrNotFound
		}
	}
	if len(raw) == 0 || len(raw) > 16<<10 || len(success) != 0 || len(events) != 1 ||
		state != string(action.AbandonedReconciled) || externalRef != "" || observed != "" {
		return zero, invalidPlannedActionReceipt()
	}
	var retained recovery.ActionPlannedAbandonReceipt
	if strictjson.Decode(raw, &retained) != nil || retained.Validate() != nil ||
		retained.Request.OperationID != operationID || retained.Request.RunID != runID ||
		retained.Request.ExecutionEpoch != executionEpoch ||
		retained.Request.OriginalRecoveryEpoch != originalRecoveryEpoch ||
		retained.Request.IdempotencyKey != key || retained.Request.OriginalRequestDigest != originalDigest ||
		!retained.CreatedAt.Equal(updated) {
		return zero, invalidPlannedActionReceipt()
	}
	var event audit.Event
	digest, err := canonical.Digest(retained)
	if err != nil || strictjson.Decode(events[0], &event) != nil ||
		event.Type != "recovery.action.planned.abandoned" || event.AggregateType != "ExternalOperation" ||
		event.AggregateID != operationID || event.PayloadDigest != digest ||
		event.CausationID != "" || event.CorrelationID != "" || !event.CreatedAt.Equal(retained.CreatedAt) {
		return zero, invalidPlannedActionReceipt()
	}
	rebuilt, err := audit.Build(event.Sequence, event.PreviousDigest, audit.Input{
		Type: event.Type, AggregateType: event.AggregateType, AggregateID: event.AggregateID,
		PayloadDigest: event.PayloadDigest, CausationID: event.CausationID, CorrelationID: event.CorrelationID,
	}, event.CreatedAt)
	if err != nil || rebuilt.Digest != event.Digest {
		return zero, invalidPlannedActionReceipt()
	}
	return retained, nil
}

func invalidPlannedActionReceipt() error {
	return fmt.Errorf("%w: planned Action abandonment requires its original row and bound audit", ErrRecoveryFactsUnresolved)
}

func validatePlannedActionAbandonments(ctx context.Context, tx pgx.Tx) error {
	var orphan bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM audit_events a
 LEFT JOIN external_operations e ON e.operation_id=a.aggregate_id
 WHERE a.aggregate_type='ExternalOperation' AND a.event_type='recovery.action.planned.abandoned'
 AND e.operation_id IS NULL)`).Scan(&orphan); err != nil {
		return err
	}
	if orphan {
		return invalidPlannedActionReceipt()
	}
	rows, err := tx.Query(ctx, plannedActionAbandonSelect+` WHERE
 (e.state='ABANDONED_RECONCILED' AND e.reconciliation_json->>'kind'
   IS DISTINCT FROM 'RECOVERY_GITHUB_PUBLICATION_NOT_INVOKED_V1')
 OR e.reconciliation_json->>'kind'='RECOVERY_ACTION_PLANNED_ABANDONMENT_V1'
 OR EXISTS(SELECT 1 FROM audit_events a WHERE a.aggregate_type='ExternalOperation'
 AND a.aggregate_id=e.operation_id AND a.event_type='recovery.action.planned.abandoned')`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		if _, err := scanPlannedActionAbandonReceipt(rows); err != nil {
			return err
		}
	}
	return rows.Err()
}
