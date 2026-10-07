package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jiying2007/engineering-platform/internal/recovery"
	corestore "github.com/jiying2007/engineering-platform/internal/store"
)

func (s *Store) ReconcileExecutionAbandoned(ctx context.Context, reconciler string, request recovery.ExecutionAbandonRequest) (recovery.ExecutionAbandonReceipt, error) {
	var zero recovery.ExecutionAbandonReceipt
	if s == nil || s.pool == nil || request.Validate() != nil || reconciler == "" {
		return zero, fmt.Errorf("valid recovery execution reconciliation required")
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
	if err = tx.QueryRow(ctx, `SELECT recovery_epoch,recovery_mode FROM platform_state WHERE singleton_id=true FOR UPDATE`).Scan(&epoch, &mode); err != nil {
		return zero, err
	}
	if epoch != request.RecoveryEpoch {
		return zero, recovery.ErrStaleEpoch
	}
	if mode != string(recovery.RecoveryReconciliation) {
		return zero, recovery.ErrReconciliationRequired
	}

	var state string
	var leaseUntil time.Time
	var receiptPresent bool
	var storedRaw []byte
	switch request.ExecutionKind {
	case recovery.ExecutionOffline:
		row, readErr := scanOffline(tx.QueryRow(ctx, `SELECT `+offlineColumns+` FROM worker_offline_executions WHERE run_id=$1 AND execution_id=$2 FOR UPDATE`, request.RunID, request.ExecutionID))
		if readErr != nil {
			return zero, readErr
		}
		if row.token.RunID != request.RunID || row.token.ID != request.ExecutionID {
			return zero, corestore.ErrConflict
		}
		state, leaseUntil, receiptPresent = row.state, row.until, row.receipt != nil
		if err = tx.QueryRow(ctx, `SELECT reconciliation_json FROM worker_offline_executions WHERE execution_id=$1`, request.ExecutionID).Scan(&storedRaw); err != nil {
			return zero, err
		}
	case recovery.ExecutionCodex:
		row, readErr := scanCodex(tx.QueryRow(ctx, `SELECT `+codexColumns+` FROM worker_codex_executions WHERE run_id=$1 AND execution_id=$2 FOR UPDATE`, request.RunID, request.ExecutionID))
		if readErr != nil {
			return zero, readErr
		}
		if row.token.RunID != request.RunID || row.token.ID != request.ExecutionID {
			return zero, corestore.ErrConflict
		}
		state, leaseUntil, receiptPresent = row.state, row.until, row.receipt != nil
		if err = tx.QueryRow(ctx, `SELECT reconciliation_json FROM worker_codex_executions WHERE execution_id=$1`, request.ExecutionID).Scan(&storedRaw); err != nil {
			return zero, err
		}
	default:
		return zero, fmt.Errorf("unsupported execution reconciliation kind")
	}

	if state == recovery.ExecutionAbandoned {
		var retained recovery.ExecutionAbandonReceipt
		if len(storedRaw) == 0 || json.Unmarshal(storedRaw, &retained) != nil || retained.Validate() != nil {
			return zero, fmt.Errorf("invalid retained execution reconciliation")
		}
		digest, _ := request.Digest()
		if retained.RequestDigest != digest || retained.Reconciler != reconciler {
			return zero, corestore.ErrConflict
		}
		return retained, tx.Commit(ctx)
	}
	if receiptPresent || (state != "AUTHORIZED" && state != "UNKNOWN") {
		return zero, corestore.ErrConflict
	}
	var now time.Time
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return zero, err
	}
	if leaseUntil.After(now) {
		return zero, corestore.ErrConflict
	}
	var deliveries int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM delivery_receipts WHERE run_id=$1`, request.RunID).Scan(&deliveries); err != nil {
		return zero, err
	}
	if deliveries != 0 {
		return zero, corestore.ErrConflict
	}
	requestDigest, err := request.Digest()
	if err != nil {
		return zero, err
	}
	retained := recovery.ExecutionAbandonReceipt{
		Kind: recovery.ExecutionAbandonmentKind, Request: request, RequestDigest: requestDigest,
		Reconciler: reconciler, PreviousState: state, CreatedAt: now,
	}
	if err = retained.Validate(); err != nil {
		return zero, err
	}
	raw, err := json.Marshal(retained)
	if err != nil {
		return zero, err
	}
	audit, err := auditInput("recovery.execution.abandoned", "Run", request.RunID, retained)
	if err != nil {
		return zero, err
	}
	if _, err = appendAudit(ctx, tx, audit, now); err != nil {
		return zero, err
	}
	switch request.ExecutionKind {
	case recovery.ExecutionOffline:
		result, updateErr := tx.Exec(ctx, `UPDATE worker_offline_executions SET state=$2,reconciliation_json=$3::jsonb WHERE execution_id=$1 AND receipt_json IS NULL AND state=$4`,
			request.ExecutionID, recovery.ExecutionAbandoned, string(raw), state)
		if updateErr != nil {
			return zero, updateErr
		}
		if result.RowsAffected() != 1 {
			return zero, corestore.ErrConflict
		}
	case recovery.ExecutionCodex:
		result, updateErr := tx.Exec(ctx, `UPDATE worker_codex_executions SET state=$2,reconciliation_json=$3::jsonb WHERE execution_id=$1 AND receipt_json IS NULL AND state=$4`,
			request.ExecutionID, recovery.ExecutionAbandoned, string(raw), state)
		if updateErr != nil {
			return zero, updateErr
		}
		if result.RowsAffected() != 1 {
			return zero, corestore.ErrConflict
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return zero, err
	}
	return retained, nil
}
