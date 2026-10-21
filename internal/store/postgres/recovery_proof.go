package postgres

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jiying2007/engineering-platform/internal/audit"
	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/recovery"
	corestore "github.com/jiying2007/engineering-platform/internal/store"
	"github.com/jiying2007/engineering-platform/internal/strictjson"
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
	// Creation retains the head under the journal lock and appends the proof
	// audit at head+1 in the same transaction. Read its independent row and
	// original audit together, even when GET uses a pool without an outer tx.
	// Both audit lookups use sequence primary keys; later history is irrelevant
	// to the identity of this original proof and must not replace its audit.
	const query = `SELECT p.proof_json,p.recovery_epoch,p.reconciler,p.facts_digest,
 p.audit_sequence,COALESCE(p.audit_digest,''),p.created_at,
 COALESCE(h.sequence,0),COALESCE(h.event_digest,''),
 (SELECT to_jsonb(a) FROM (
   SELECT sequence,event_type AS type,aggregate_type,aggregate_id,payload_digest,
          COALESCE(previous_digest,'') AS previous_digest,event_digest AS digest,
          COALESCE(correlation_id,'') AS correlation_id,
          COALESCE(causation_id,'') AS causation_id,created_at
   FROM audit_events WHERE sequence=p.audit_sequence+1
 ) a)
 FROM recovery_reconciliation_proofs p
 LEFT JOIN audit_events h ON h.sequence=p.audit_sequence
 WHERE p.recovery_epoch=$1`
	var raw, auditRaw []byte
	var rowEpoch, auditSequence, headSequence uint64
	var reconciler, factsDigest, auditDigest, headDigest string
	var createdAt time.Time
	if err := q.QueryRow(ctx, query, epoch).Scan(&raw, &rowEpoch, &reconciler,
		&factsDigest, &auditSequence, &auditDigest, &createdAt, &headSequence, &headDigest, &auditRaw); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return recovery.Proof{}, corestore.ErrNotFound
		}
		return recovery.Proof{}, err
	}
	var proof recovery.Proof
	if len(raw) == 0 || len(raw) > 16<<10 || strictjson.Decode(raw, &proof) != nil ||
		!completeRecoveryProofJSON(raw) || proof.Validate() != nil ||
		proof.RecoveryEpoch != epoch || proof.RecoveryEpoch != rowEpoch ||
		proof.Reconciler != reconciler || proof.FactsDigest != factsDigest ||
		proof.AuditSequence != auditSequence || proof.AuditDigest != auditDigest ||
		!proof.CreatedAt.Equal(createdAt) || headSequence != auditSequence || headDigest != auditDigest {
		return recovery.Proof{}, invalidRecoveryProof()
	}
	// The proof binds only the preceding head's sequence and digest. Reading
	// its full event here would impose a new JSON-size contract on unrelated
	// historical aggregate IDs. The proof's own original event is fully checked.
	var original audit.Event
	if len(auditRaw) == 0 || len(auditRaw) > 16<<10 || strictjson.Decode(auditRaw, &original) != nil {
		return recovery.Proof{}, invalidRecoveryProof()
	}
	rebuilt, err := audit.Build(original.Sequence, original.PreviousDigest, audit.Input{
		Type: original.Type, AggregateType: original.AggregateType, AggregateID: original.AggregateID,
		PayloadDigest: original.PayloadDigest, CausationID: original.CausationID, CorrelationID: original.CorrelationID,
	}, original.CreatedAt)
	if err != nil || rebuilt.Digest != original.Digest {
		return recovery.Proof{}, invalidRecoveryProof()
	}
	digest, err := canonical.Digest(proof)
	if err != nil || original.Sequence != auditSequence+1 || original.PreviousDigest != auditDigest ||
		original.Type != "recovery.reconciliation_proved" || original.AggregateType != "PlatformState" ||
		original.AggregateID != "singleton" || original.PayloadDigest != digest ||
		original.CausationID != "" || original.CorrelationID != "" || !original.CreatedAt.Equal(createdAt) {
		return recovery.Proof{}, invalidRecoveryProof()
	}
	return proof, nil
}

// A retained proof has no optional members. In particular, encoding/json would
// otherwise turn missing/null facts or counters into the same all-zero Facts
// that permit completion. Strict decoding alone does not reject those forms.
func completeRecoveryProofJSON(raw []byte) bool {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return false
	}
	for _, key := range []string{"kind", "recovery_epoch", "reconciler", "facts", "facts_digest", "audit_sequence", "audit_digest", "created_at"} {
		if len(fields[key]) == 0 || bytes.Equal(bytes.TrimSpace(fields[key]), []byte("null")) {
			return false
		}
	}
	var facts map[string]json.RawMessage
	if json.Unmarshal(fields["facts"], &facts) != nil {
		return false
	}
	for _, key := range []string{"external_unresolved", "mutation_outbox_leases", "worker_leases", "offline_unresolved"} {
		if len(facts[key]) == 0 || bytes.Equal(bytes.TrimSpace(facts[key]), []byte("null")) {
			return false
		}
	}
	return true
}

func invalidRecoveryProof() error {
	return fmt.Errorf("%w: recovery proof requires its exact epoch, original row and bound audit", ErrRecoveryFactsUnresolved)
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
