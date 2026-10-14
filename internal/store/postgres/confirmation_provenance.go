package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jiying2007/engineering-platform/internal/action"
	"github.com/jiying2007/engineering-platform/internal/audit"
	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/strictjson"
)

// Written only by the original DISPATCHED->CONFIRMED transaction, not by
// ordinary reconciliation. It is not an external Provider quiescence receipt.
const originalConfirmationCause = "action.original-dispatch-confirmation.v1"

// At most two matching markers are fetched per row: zero or duplicates reject.
// A matching payload, exact original identity, reconstructed event digest and
// stable DB timestamp are required, not merely a count of state-change events.
// A pre-upgrade bare CONFIRMED row remains read-only and unqualified until the
// positive Recovery route independently obtains and retains the full witness.
const ordinaryConfirmationSelect = `SELECT e.operation_id,e.run_id,e.execution_epoch,
 e.recovery_epoch,e.action,e.risk_class,e.capability,e.idempotency_key,e.request_digest,
 e.state,COALESCE(e.external_ref,''),COALESCE(e.observed_state,''),e.updated_at,
 COALESCE((SELECT jsonb_agg(to_jsonb(a)) FROM (
   SELECT sequence,event_type AS type,aggregate_type,aggregate_id,payload_digest,
          COALESCE(previous_digest,'') AS previous_digest,event_digest AS digest,
          COALESCE(correlation_id,'') AS correlation_id,causation_id,created_at
   FROM audit_events WHERE aggregate_type='ExternalOperation'
     AND aggregate_id=e.operation_id AND event_type='action.state_changed'
     AND causation_id='action.original-dispatch-confirmation.v1'
   ORDER BY sequence LIMIT 2
 ) a),'[]'::jsonb)
 FROM external_operations e
 WHERE e.receipt_json IS NULL AND (
 (e.state='CONFIRMED' AND e.risk_class<>'OBSERVE') OR EXISTS(
  SELECT 1 FROM audit_events marker WHERE marker.aggregate_type='ExternalOperation'
  AND marker.aggregate_id=e.operation_id AND marker.event_type='action.state_changed'
  AND marker.causation_id='action.original-dispatch-confirmation.v1'))`

func validateOrdinaryConfirmations(ctx context.Context, tx pgx.Tx) error {
	var orphan bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM audit_events a
	 LEFT JOIN external_operations e ON e.operation_id=a.aggregate_id
	 WHERE a.aggregate_type='ExternalOperation' AND a.event_type='action.state_changed'
	 AND a.causation_id='action.original-dispatch-confirmation.v1' AND e.operation_id IS NULL)`).Scan(&orphan); err != nil {
		return err
	}
	if orphan {
		return unqualifiedConfirmationError()
	}
	rows, err := tx.Query(ctx, ordinaryConfirmationSelect)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var op action.Operation
		var risk, state string
		var raw []byte
		if err := rows.Scan(&op.ID, &op.RunID, &op.ExecutionEpoch, &op.RecoveryEpoch,
			&op.Action, &risk, &op.Capability, &op.IdempotencyKey, &op.RequestDigest,
			&state, &op.ExternalRef, &op.ObservedState, &op.UpdatedAt, &raw); err != nil {
			return err
		}
		op.RiskClass = action.RiskClass(risk)
		op.State = action.State(state)
		// pgx may attach the local timezone to a decoded timestamptz. Normalize
		// only the representation, never invent lost historic nanoseconds.
		op.UpdatedAt = op.UpdatedAt.UTC()
		if err := validateOriginalConfirmation(op, raw); err != nil {
			return err
		}
	}
	return rows.Err()
}

func validateOriginalConfirmation(op action.Operation, raw []byte) error {
	var entries []json.RawMessage
	if len(raw) == 0 || len(raw) > 16<<10 || json.Unmarshal(raw, &entries) != nil || len(entries) != 1 {
		return unqualifiedConfirmationError()
	}
	var e audit.Event
	if strictjson.Decode(entries[0], &e) != nil {
		return unqualifiedConfirmationError()
	}
	expected, err := canonical.Digest(op)
	if err != nil || op.State != action.Confirmed || op.UpdatedAt.IsZero() ||
		!op.UpdatedAt.Equal(op.UpdatedAt.Truncate(time.Microsecond)) ||
		e.Type != "action.state_changed" || e.AggregateType != "ExternalOperation" ||
		e.AggregateID != op.ID || e.CausationID != originalConfirmationCause ||
		e.CorrelationID != "" || e.PayloadDigest != expected || e.CreatedAt.IsZero() || e.CreatedAt.Before(op.UpdatedAt) {
		return unqualifiedConfirmationError()
	}
	rebuilt, err := audit.Build(e.Sequence, e.PreviousDigest, audit.Input{
		Type: e.Type, AggregateType: e.AggregateType, AggregateID: e.AggregateID,
		PayloadDigest: e.PayloadDigest, CausationID: e.CausationID, CorrelationID: e.CorrelationID,
	}, e.CreatedAt)
	if err != nil || rebuilt.Digest != e.Digest {
		return unqualifiedConfirmationError()
	}
	return nil
}

func unqualifiedConfirmationError() error {
	return fmt.Errorf("%w: original confirmation requires bound provenance or independent Recovery evidence", ErrRecoveryFactsUnresolved)
}
