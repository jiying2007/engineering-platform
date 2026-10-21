package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/recovery"
	corestore "github.com/jiying2007/engineering-platform/internal/store"
)

var ErrRecoveryFactsUnresolved = errors.New("recovery reconciliation facts are not clear")

func recoveryFacts(ctx context.Context, tx pgx.Tx) (recovery.Facts, error) {
	var facts recovery.Facts
	if err := validatePlannedActionAbandonments(ctx, tx); err != nil {
		return facts, err
	}
	if err := validateRecoveredPublications(ctx, tx); err != nil {
		return facts, fmt.Errorf("%w: %w", ErrRecoveryFactsUnresolved, err)
	}
	if err := validateUninvokedPublications(ctx, tx); err != nil {
		return facts, fmt.Errorf("%w: %w", ErrRecoveryFactsUnresolved, err)
	}
	if err := validateOrdinaryConfirmations(ctx, tx); err != nil {
		return facts, err
	}
	queries := []struct {
		dst *uint64
		q   string
	}{
		{&facts.ExternalUnresolved, `SELECT count(*) FROM external_operations WHERE state NOT IN ('CONFIRMED','ABANDONED_RECONCILED')`},
		{&facts.MutationOutboxLeases, `SELECT count(*) FROM outbox_events WHERE state='LEASED' AND risk_class<>'OBSERVE' AND lease_until>clock_timestamp()`},
		{&facts.WorkerLeases, `SELECT count(*) FROM worker_inbox WHERE state='LEASED' AND lease_until>clock_timestamp()`},
		{&facts.OfflineUnresolved, `SELECT count(*) FROM worker_offline_executions WHERE state IN ('AUTHORIZED','UNKNOWN')`},
	}
	for _, query := range queries {
		if err := tx.QueryRow(ctx, query.q).Scan(query.dst); err != nil {
			return recovery.Facts{}, err
		}
	}
	var codexReady bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM core_schema_migrations WHERE version=8)`).Scan(&codexReady); err != nil {
		return recovery.Facts{}, err
	}
	if codexReady {
		var codexUnresolved uint64
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM worker_codex_executions WHERE state IN ('AUTHORIZED','UNKNOWN')`).Scan(&codexUnresolved); err != nil {
			return recovery.Facts{}, err
		}
		facts.OfflineUnresolved += codexUnresolved
	}
	return facts, nil
}

func (s *Store) CreateRecoveryProof(ctx context.Context, epoch uint64, reconciler string) (recovery.Proof, error) {
	ctx, cancel := authorityContext(ctx)
	defer cancel()

	var proof recovery.Proof
	if s == nil || s.pool == nil || epoch == 0 || reconciler == "" {
		return proof, fmt.Errorf("PostgreSQL recovery proof store is not configured")
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return proof, err
	}
	// The caller may be cancelled while waiting on platform_state. Preserve
	// independently bounded transaction cleanup rather than attempting
	// rollback with that already-cancelled request Context.
	defer rollbackOutbox(tx)
	var currentEpoch uint64
	var mode string
	if err := tx.QueryRow(ctx, `SELECT recovery_epoch,recovery_mode FROM platform_state WHERE singleton_id=true FOR UPDATE`).Scan(&currentEpoch, &mode); err != nil {
		return proof, err
	}
	if currentEpoch != epoch || mode != string(recovery.RecoveryReconciliation) {
		return proof, corestore.ErrConflict
	}
	facts, err := recoveryFacts(ctx, tx)
	if err != nil {
		return proof, err
	}
	if !facts.Clear() {
		return proof, fmt.Errorf("%w: %+v", ErrRecoveryFactsUnresolved, facts)
	}
	factsDigest, err := canonical.Digest(facts)
	if err != nil {
		return proof, err
	}
	var auditSequence uint64
	var auditDigest string
	if err := tx.QueryRow(ctx, `SELECT last_sequence,last_digest FROM audit_journal_state WHERE singleton_id=true FOR SHARE`).Scan(&auditSequence, &auditDigest); err != nil {
		return proof, err
	}
	var now time.Time
	if err := tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return proof, err
	}
	proof = recovery.Proof{
		Kind: recovery.ProofKind, RecoveryEpoch: epoch, Reconciler: reconciler,
		Facts: facts, FactsDigest: factsDigest, AuditSequence: auditSequence, AuditDigest: auditDigest, CreatedAt: now,
	}
	if err := proof.Validate(); err != nil {
		return recovery.Proof{}, err
	}
	raw, err := json.Marshal(proof)
	if err != nil {
		return recovery.Proof{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO recovery_reconciliation_proofs
	 (recovery_epoch,reconciler,facts_digest,audit_sequence,audit_digest,proof_json,created_at)
	 VALUES($1,$2,$3,$4,$5,$6::jsonb,$7)`,
		epoch, reconciler, factsDigest, auditSequence, nullIfEmpty(auditDigest), string(raw), now)
	if err != nil {
		return recovery.Proof{}, mapWriteError(err)
	}
	input, err := auditInput("recovery.reconciliation_proved", "PlatformState", "singleton", proof)
	if err != nil {
		return recovery.Proof{}, err
	}
	if _, err := appendAudit(ctx, tx, input, now); err != nil {
		return recovery.Proof{}, err
	}
	if err := commitAuthority(ctx, tx); err != nil {
		return recovery.Proof{}, err
	}
	return proof, nil
}

func (s *Store) GetRecoveryProof(ctx context.Context, epoch uint64) (recovery.Proof, error) {
	ctx, cancel := authorityContext(ctx)
	defer cancel()

	if s == nil || s.pool == nil || epoch == 0 {
		return recovery.Proof{}, corestore.ErrNotFound
	}
	return readRecoveryProof(ctx, s.pool, epoch)
}

func readRecoveryProof(ctx context.Context, q publicationRowQuery, epoch uint64) (recovery.Proof, error) {
	var raw []byte
	if err := q.QueryRow(ctx, `SELECT proof_json FROM recovery_reconciliation_proofs WHERE recovery_epoch=$1`, epoch).Scan(&raw); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return recovery.Proof{}, corestore.ErrNotFound
		}
		return recovery.Proof{}, err
	}
	var proof recovery.Proof
	if err := json.Unmarshal(raw, &proof); err != nil {
		return recovery.Proof{}, fmt.Errorf("decode recovery proof: %w", err)
	}
	if err := proof.Validate(); err != nil {
		return recovery.Proof{}, err
	}
	return proof, nil
}

// AuthorizeCompletion is the authenticated API gate. The completion identity
// must be distinct from the identity that generated the database facts.
func (s *Store) AuthorizeCompletion(ctx context.Context, subject string, epoch uint64) error {
	ctx, cancel := authorityContext(ctx)
	defer cancel()

	if s == nil || s.pool == nil || epoch == 0 {
		return corestore.ErrNotFound
	}
	// Authorization is read-only, but the proof, abandonment provenance and
	// participant identities must come from the same database snapshot.
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return err
	}
	defer rollbackOutbox(tx)
	proof, err := readRecoveryProof(ctx, tx, epoch)
	if err != nil {
		return err
	}
	if subject == "" || proof.Reconciler == subject {
		return fmt.Errorf("recovery completion requires a different authenticated principal from reconciler")
	}
	if err := validatePlannedActionAbandonments(ctx, tx); err != nil {
		return err
	}
	// A damaged PLANNED receipt cannot evade participant validation by losing
	// both its kind and audit marker or impersonating the other terminal kind.
	if err := validateUninvokedPublications(ctx, tx); err != nil {
		return fmt.Errorf("%w: %w", ErrRecoveryFactsUnresolved, err)
	}
	// A principal that settled any original Action may not complete that
	// Recovery epoch through another principal's aggregate proof, even if a
	// later policy revision gives the old reconciler the completion role.
	var participated bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM external_operations
  WHERE (receipt_json->>'kind'='RECOVERY_GITHUB_PUBLICATION_CONFIRMED_V1'
    AND receipt_json->'request'->>'recovery_epoch'=$1
    AND receipt_json->>'reconciler'=$2) OR
    (reconciliation_json->>'kind' IN ('RECOVERY_GITHUB_PUBLICATION_NOT_INVOKED_V1','RECOVERY_ACTION_PLANNED_ABANDONMENT_V1')
    AND reconciliation_json->'request'->>'recovery_epoch'=$1
    AND reconciliation_json->>'reconciler'=$2))`, fmt.Sprint(epoch), subject).Scan(&participated); err != nil {
		return err
	}
	if participated {
		return fmt.Errorf("Action reconciler cannot complete the same Recovery epoch")
	}
	return nil
}
