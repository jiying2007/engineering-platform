package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jiying2007/engineering-platform/internal/action"
	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/githubpublish"
	"github.com/jiying2007/engineering-platform/internal/recovery"
	corestore "github.com/jiying2007/engineering-platform/internal/store"
	"github.com/jiying2007/engineering-platform/internal/strictjson"
)

// ConfirmRecoveredPublication is not a generic terminal override. Its evidence
// argument is assembled by the configured independent Publisher observer in Core,
// never decoded from a caller's request. It uses the ORIGINAL external_operations
// row and its reserved receipt_json column. Planned no-effect abandonment continues
// to use reconciliation_json with its unchanged schema-v13 constraint.
func (s *Store) ConfirmRecoveredPublication(ctx context.Context, reconciler string, request githubpublish.PublicationRecoveryRequest, evidence githubpublish.PublicationEvidence) (githubpublish.PublicationRecoveryReceipt, error) {
	var zero githubpublish.PublicationRecoveryReceipt
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
	const query = `SELECT operation_id,run_id,execution_epoch,recovery_epoch,idempotency_key,request_digest,action,risk_class,capability,state,receipt_json,reconciliation_json FROM external_operations WHERE operation_id=$1 FOR UPDATE`
	if err := tx.QueryRow(ctx, query, request.Identity.OperationID).Scan(&identity.OperationID, &identity.RunID, &identity.ExecutionEpoch, &identity.RecoveryEpoch, &identity.IdempotencyKey, &identity.RequestDigest, &actionName, &risk, &capability, &state, &raw, &reconciliation); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return zero, corestore.ErrNotFound
		}
		return zero, err
	}
	if identity != request.Identity || actionName != githubpublish.Action || capability != githubpublish.Capability || risk != string(action.ControlledMutation) {
		return zero, corestore.ErrConflict
	}
	requestDigest, _ := request.Digest()
	if state == string(action.Confirmed) && len(raw) > 0 {
		retained, err := readRecoveredPublication(ctx, tx, identity.OperationID)
		if err != nil {
			return zero, err
		}
		if retained.RequestDigest != requestDigest || retained.Reconciler != reconciler {
			return zero, corestore.ErrConflict
		}
		return retained, nil // Read-only exact repeat. Deferred rollback releases locks.
	}
	if !githubpublish.RecoverablePublicationState(action.State(state)) || len(raw) != 0 || len(reconciliation) != 0 {
		return zero, corestore.ErrConflict
	}
	var now time.Time
	if err := tx.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&now); err != nil {
		return zero, err
	}
	evidenceDigest, _ := evidence.Digest()
	retained := githubpublish.PublicationRecoveryReceipt{Kind: "RECOVERY_GITHUB_PUBLICATION_CONFIRMED_V1", Request: request, RequestDigest: requestDigest, Reconciler: reconciler, PreviousState: action.State(state), Evidence: evidence, EvidenceDigest: evidenceDigest, RecordedAt: now}
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
	observation, err := json.Marshal(evidence.Observation.Receipt)
	if err != nil {
		return zero, err
	}
	input, err := auditInput("recovery.github.publication.confirmed", "ExternalOperation", request.Identity.OperationID, retained)
	if err != nil {
		return zero, err
	}
	// Every identity is already compared under the row lock; additionally retain
	// the expected prior state and original empty receipt slot in the UPDATE.
	tag, err := tx.Exec(ctx, `UPDATE external_operations SET state='CONFIRMED',receipt_json=$2::jsonb,external_ref=$3,observed_state=$4,updated_at=$5 WHERE operation_id=$1 AND state=$6 AND receipt_json IS NULL AND reconciliation_json IS NULL`, identity.OperationID, string(raw), evidence.Observation.Receipt.PullRequestURL, string(observation), now, state)
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
func (s *Store) GetRecoveredPublication(ctx context.Context, id string) (githubpublish.PublicationRecoveryReceipt, error) {
	var zero githubpublish.PublicationRecoveryReceipt
	if s == nil || s.pool == nil || id == "" || len(id) > 256 {
		return zero, corestore.ErrNotFound
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return readRecoveredPublication(ctx, s.pool, id)
}

// All three consumers (readback, exact-repeat settlement, and Recovery facts)
// use one consistent row/audit projection. A valid self-hashed replacement is
// not the original journal payload. This detects mismatched restore/corruption;
// it is not authentication against an administrator rewriting the whole journal.
const recoveredPublicationSelect = `SELECT e.receipt_json,e.operation_id,e.run_id,
 e.execution_epoch,e.recovery_epoch,e.action,e.risk_class,e.capability,
 e.idempotency_key,e.request_digest,e.state,COALESCE(e.external_ref,''),
 COALESCE(e.observed_state,''),e.updated_at,
 (SELECT count(*) FROM audit_events a WHERE a.event_type='recovery.github.publication.confirmed'
   AND a.aggregate_type='ExternalOperation' AND a.aggregate_id=e.operation_id),
 COALESCE((SELECT min(a.payload_digest) FROM audit_events a
   WHERE a.event_type='recovery.github.publication.confirmed'
   AND a.aggregate_type='ExternalOperation' AND a.aggregate_id=e.operation_id),'')
 FROM external_operations e`

type publicationRowQuery interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func readRecoveredPublication(ctx context.Context, q publicationRowQuery, id string) (githubpublish.PublicationRecoveryReceipt, error) {
	return scanRecoveredPublication(q.QueryRow(ctx, recoveredPublicationSelect+" WHERE e.operation_id=$1", id))
}

func scanRecoveredPublication(row interface{ Scan(...any) error }) (githubpublish.PublicationRecoveryReceipt, error) {
	var zero githubpublish.PublicationRecoveryReceipt
	var raw []byte
	var op action.Operation
	var risk, state, observed, auditDigest string
	var auditCount uint64
	var updatedAt time.Time
	err := row.Scan(&raw, &op.ID, &op.RunID, &op.ExecutionEpoch, &op.RecoveryEpoch,
		&op.Action, &risk, &op.Capability, &op.IdempotencyKey, &op.RequestDigest,
		&state, &op.ExternalRef, &observed, &updatedAt, &auditCount, &auditDigest)
	if errors.Is(err, pgx.ErrNoRows) {
		return zero, corestore.ErrNotFound
	}
	if err != nil {
		return zero, err
	}
	if len(raw) == 0 && auditCount == 0 {
		return zero, corestore.ErrNotFound
	}
	if len(raw) == 0 || len(raw) > 128<<10 || state != string(action.Confirmed) || auditCount != 1 {
		return zero, githubpublish.ErrDispatchEvidenceUnavailable
	}
	op.RiskClass = action.RiskClass(risk)
	identity, err := githubpublish.IdentityForOperation(op)
	if err != nil {
		return zero, err
	}
	var retained githubpublish.PublicationRecoveryReceipt
	var receipt githubpublish.PublicationReceipt
	if strictjson.Decode(raw, &retained) != nil || retained.Validate() != nil ||
		retained.Request.Identity != identity || strictjson.Decode([]byte(observed), &receipt) != nil ||
		receipt != retained.Evidence.Observation.Receipt || op.ExternalRef != receipt.PullRequestURL || !updatedAt.Equal(retained.RecordedAt) {
		return zero, fmt.Errorf("%w: retained row identity/observation mismatch", githubpublish.ErrDispatchEvidenceUnavailable)
	}
	digest, err := canonical.Digest(retained)
	if err != nil || digest != auditDigest {
		return zero, fmt.Errorf("%w: retained receipt differs from original audit payload", githubpublish.ErrDispatchEvidenceUnavailable)
	}
	return retained, nil
}

// Stream inside the caller's existing Recovery transaction/snapshot, never
// create a second authority. Ordinary confirmed Actions without a recovery
// receipt/event remain untouched; an event without its receipt stays blocked.
func validateRecoveredPublications(ctx context.Context, tx pgx.Tx) error {
	var orphan bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM audit_events a
      LEFT JOIN external_operations e ON e.operation_id=a.aggregate_id
      WHERE a.event_type='recovery.github.publication.confirmed'
        AND a.aggregate_type='ExternalOperation' AND e.operation_id IS NULL)`).Scan(&orphan); err != nil {
		return err
	}
	if orphan {
		return githubpublish.ErrDispatchEvidenceUnavailable
	}
	rows, err := tx.Query(ctx, recoveredPublicationSelect+` WHERE e.receipt_json IS NOT NULL
 OR EXISTS(SELECT 1 FROM audit_events a WHERE a.event_type='recovery.github.publication.confirmed'
 AND a.aggregate_type='ExternalOperation' AND a.aggregate_id=e.operation_id)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		if _, err := scanRecoveredPublication(rows); err != nil {
			return err
		}
	}
	return rows.Err()
}
