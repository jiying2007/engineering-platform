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
	queries := []struct {
		dst *uint64
		q   string
	}{
		{&facts.ExternalUnresolved, `SELECT count(*) FROM external_operations WHERE state NOT IN ('CONFIRMED','SAFE_TO_RETRY')`},
		{&facts.MutationOutboxLeases, `SELECT count(*) FROM outbox_events WHERE state='LEASED' AND risk_class<>'OBSERVE' AND lease_until>clock_timestamp()`},
		{&facts.WorkerLeases, `SELECT count(*) FROM worker_inbox WHERE state='LEASED' AND lease_until>clock_timestamp()`},
		{&facts.OfflineUnresolved, `SELECT count(*) FROM worker_offline_executions WHERE state IN ('AUTHORIZED','UNKNOWN')`},
	}
	for _, query := range queries {
		if err := tx.QueryRow(ctx, query.q).Scan(query.dst); err != nil {
			return recovery.Facts{}, err
		}
	}
	return facts, nil
}

func (s *Store) CreateRecoveryProof(ctx context.Context, epoch uint64, reconciler string) (recovery.Proof, error) {
	var proof recovery.Proof
	if s == nil || s.pool == nil || epoch == 0 || reconciler == "" {
		return proof, fmt.Errorf("PostgreSQL recovery proof store is not configured")
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return proof, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
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
	if err := tx.Commit(ctx); err != nil {
		return recovery.Proof{}, err
	}
	return proof, nil
}

func (s *Store) GetRecoveryProof(ctx context.Context, epoch uint64) (recovery.Proof, error) {
	var raw []byte
	if s == nil || s.pool == nil || epoch == 0 {
		return recovery.Proof{}, corestore.ErrNotFound
	}
	if err := s.pool.QueryRow(ctx, `SELECT proof_json FROM recovery_reconciliation_proofs WHERE recovery_epoch=$1`, epoch).Scan(&raw); err != nil {
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
	proof, err := s.GetRecoveryProof(ctx, epoch)
	if err != nil {
		return err
	}
	if subject == "" || proof.Reconciler == subject {
		return fmt.Errorf("recovery completion requires a different authenticated principal from reconciler")
	}
	return nil
}
