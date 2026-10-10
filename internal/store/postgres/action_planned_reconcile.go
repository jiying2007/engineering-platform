package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jiying2007/engineering-platform/internal/action"
	"github.com/jiying2007/engineering-platform/internal/recovery"
	corestore "github.com/jiying2007/engineering-platform/internal/store"
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
		var retained recovery.ActionPlannedAbandonReceipt
		if len(reconciliationRaw) == 0 ||
			json.Unmarshal(reconciliationRaw, &retained) != nil ||
			retained.Validate() != nil ||
			retained.Request.OperationID != request.OperationID {
			return zero, fmt.Errorf("invalid retained Action reconciliation receipt")
		}
		if retained.RequestDigest != requestDigest || retained.Reconciler != reconciler {
			return zero, corestore.ErrConflict
		}
		if err := tx.Commit(ctx); err != nil {
			return zero, err
		}
		return retained, nil
	}
	if state != string(action.Planned) || len(effectReceipt) != 0 ||
		externalRef != "" || observedState != "" || len(reconciliationRaw) != 0 {
		return zero, corestore.ErrConflict
	}

	var now time.Time
	if err := tx.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&now); err != nil {
		return zero, err
	}
	retained := recovery.ActionPlannedAbandonReceipt{
		Kind: recovery.ActionPlannedAbandonmentKind,
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
	if err := tx.Commit(ctx); err != nil {
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
	var state string
	var raw []byte
	err := s.pool.QueryRow(ctx,
		"SELECT state,reconciliation_json FROM external_operations WHERE operation_id=$1", operationID,
	).Scan(&state, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return zero, corestore.ErrNotFound
	}
	if err != nil {
		return zero, err
	}
	if state != string(action.AbandonedReconciled) || len(raw) == 0 {
		return zero, corestore.ErrNotFound
	}
	var receipt recovery.ActionPlannedAbandonReceipt
	if json.Unmarshal(raw, &receipt) != nil || receipt.Validate() != nil ||
		receipt.Request.OperationID != operationID {
		return zero, fmt.Errorf("invalid retained planned Action reconciliation")
	}
	return receipt, nil
}
