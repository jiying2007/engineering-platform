package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jiying2007/engineering-platform/internal/action"
	"github.com/jiying2007/engineering-platform/internal/outbox"
)

var _ outbox.Repository = (*Store)(nil)

const outboxColumns = `outbox_id,outbox_key,topic,COALESCE(aggregate_type,''),COALESCE(aggregate_id,''),
 risk_class,payload_json,state,attempt_count,COALESCE(lease_owner,''),lease_until,
 next_attempt_at,COALESCE(last_error,''),created_at,dispatched_at,lease_recovery_epoch`

func scanOutbox(row pgx.Row) (outbox.Message, error) {
	var m outbox.Message
	var payload []byte
	var leaseUntil, dispatchedAt pgtype.Timestamptz
	var epoch pgtype.Int8
	if err := row.Scan(&m.ID, &m.Key, &m.Topic, &m.AggregateType, &m.AggregateID, &m.RiskClass, &payload,
		&m.State, &m.AttemptCount, &m.LeaseOwner, &leaseUntil, &m.NextAttemptAt, &m.LastError, &m.CreatedAt, &dispatchedAt, &epoch); err != nil {
		return outbox.Message{}, err
	}
	m.Payload = append(json.RawMessage(nil), payload...)
	if leaseUntil.Valid {
		m.LeaseUntil = leaseUntil.Time
	}
	if dispatchedAt.Valid {
		m.DispatchedAt = dispatchedAt.Time
	}
	if epoch.Valid {
		m.LeaseRecoveryEpoch = uint64(epoch.Int64)
	}
	return m, nil
}

func (s *Store) Claim(ctx context.Context, workerID string, limit int, duration time.Duration) ([]outbox.Message, error) {
	if s == nil || s.pool == nil {
		return nil, fmt.Errorf("PostgreSQL store is not configured")
	}
	if workerID == "" || limit < 1 || limit > 1000 || duration < time.Millisecond || duration > 24*time.Hour {
		return nil, fmt.Errorf("invalid outbox worker, claim limit or lease duration")
	}
	// Same lock order as Dispatch: platform first, outbox second. Locking the
	// singleton also ensures a claim's epoch and recovery eligibility agree.
	const query = `WITH platform AS MATERIALIZED (
  SELECT recovery_epoch,recovery_mode FROM platform_state WHERE singleton_id=true FOR SHARE
 ), candidates AS (
  SELECT o.outbox_id,p.recovery_epoch FROM outbox_events o CROSS JOIN platform p
  WHERE (o.state='PENDING' OR (o.state='LEASED' AND o.lease_until<=clock_timestamp()))
   AND o.next_attempt_at<=clock_timestamp()
   AND o.risk_class IN ('OBSERVE','CONTROLLED_MUTATION','HIGH_RISK')
   AND p.recovery_mode IN ('NORMAL','RECOVERY_RECONCILIATION')
   AND (p.recovery_mode='NORMAL' OR o.risk_class='OBSERVE')
  ORDER BY o.outbox_id LIMIT $1 FOR UPDATE OF o SKIP LOCKED
 ), claimed AS (
  UPDATE outbox_events o SET state='LEASED',lease_owner=$2,
   lease_until=clock_timestamp()+($3 * interval '1 millisecond'),
   attempt_count=o.attempt_count+1,lease_recovery_epoch=c.recovery_epoch,last_error=NULL
  FROM candidates c WHERE o.outbox_id=c.outbox_id RETURNING o.*
 ) SELECT ` + outboxColumns + ` FROM claimed ORDER BY outbox_id`
	rows, err := s.pool.Query(ctx, query, limit, workerID, duration.Milliseconds())
	if err != nil {
		return nil, fmt.Errorf("claim outbox: %w", err)
	}
	defer rows.Close()
	var messages []outbox.Message
	for rows.Next() {
		m, err := scanOutbox(rows)
		if err != nil {
			return nil, fmt.Errorf("scan claimed outbox: %w", err)
		}
		messages = append(messages, m)
	}
	return messages, rows.Err()
}

// Dispatch serializes cooperative delivery with recovery transitions and other
// owners. The callback runs synchronously under the row locks; it may not change
// recovery/outbox state, detach work or perform non-idempotent external actions.
// DB rollback cannot roll back an external effect. Recipients must deduplicate
// by outbox_key and route irreversible actions through their reconciled ledger.
func (s *Store) Dispatch(ctx context.Context, lease outbox.Lease, risk action.RiskClass, deliver func(context.Context, outbox.Message) outbox.Resolution) (outbox.State, error) {
	if s == nil || s.pool == nil {
		return "", fmt.Errorf("PostgreSQL store is not configured")
	}
	if !lease.Valid() {
		return "", outbox.ErrLeaseLost
	}
	if !outbox.ValidRisk(risk) || deliver == nil {
		return "", outbox.ErrRiskClass
	}
	// Direct callers receive a hard upper context bound as well as the DB lease
	// bound. No goroutine is abandoned when a handler ignores cancellation.
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return "", err
	}
	defer rollbackOutbox(tx)
	var epoch uint64
	var mode string
	if err := tx.QueryRow(ctx, `SELECT recovery_epoch,recovery_mode FROM platform_state WHERE singleton_id=true FOR SHARE`).Scan(&epoch, &mode); err != nil {
		return "", fmt.Errorf("%w: read platform state: %w", outbox.ErrRecoveryBlocked, err)
	}
	if mode != "NORMAL" && mode != "RECOVERY_RECONCILIATION" {
		return "", outbox.ErrRecoveryBlocked
	}
	m, err := scanOutbox(tx.QueryRow(ctx, `SELECT `+outboxColumns+` FROM outbox_events WHERE outbox_id=$1 FOR UPDATE`, lease.ID))
	if errors.Is(err, pgx.ErrNoRows) {
		return "", outbox.ErrLeaseLost
	}
	if err != nil {
		return "", err
	}
	if m.State != outbox.Leased || m.Lease() != lease {
		return "", outbox.ErrLeaseLost
	}
	classified, err := outbox.Classify(m.Topic, m.RiskClass)
	if err != nil || classified != risk {
		return "", outbox.ErrRiskClass
	}
	if risk != action.Observe && (mode != "NORMAL" || m.LeaseRecoveryEpoch != epoch) {
		return "", outbox.ErrRecoveryBlocked
	}
	// Read DB wall time AFTER lock acquisition. now() is transaction-start time
	// and would incorrectly allow a lease which expired while waiting for a lock.
	started := time.Now()
	var remainingSeconds float64
	var hasEpoch bool
	if err := tx.QueryRow(ctx, `SELECT EXTRACT(EPOCH FROM lease_until-clock_timestamp())::double precision,lease_recovery_epoch IS NOT NULL FROM outbox_events WHERE outbox_id=$1`, lease.ID).Scan(&remainingSeconds, &hasEpoch); err != nil {
		return "", err
	}
	remaining := time.Duration(remainingSeconds*float64(time.Second)) - time.Since(started)
	if remaining <= 0 || !hasEpoch {
		return "", outbox.ErrLeaseLost
	}
	callCtx, stop := context.WithTimeout(ctx, remaining)
	defer stop()
	if err := callCtx.Err(); err != nil {
		return "", err
	}
	resolution := deliver(callCtx, m)
	if err := callCtx.Err(); err != nil {
		return "", fmt.Errorf("%w: %w", outbox.ErrOutcomeUnknown, err)
	}
	if err := resolution.Validate(); err != nil {
		return "", fmt.Errorf("%w: %w", outbox.ErrOutcomeUnknown, err)
	}
	if err := settleOutbox(callCtx, tx, lease, resolution); err != nil {
		return "", fmt.Errorf("%w: %w", outbox.ErrOutcomeUnknown, err)
	}
	// Result and audit are committed before the lease/recovery locks are released.
	record, err := auditInput("outbox.settled", "Outbox", m.Key, struct {
		Key     string       `json:"outbox_key"`
		Attempt uint64       `json:"attempt"`
		Epoch   uint64       `json:"recovery_epoch"`
		State   outbox.State `json:"state"`
	}{m.Key, m.AttemptCount, epoch, resolution.State})
	if err != nil {
		return "", fmt.Errorf("%w: %w", outbox.ErrOutcomeUnknown, err)
	}
	if _, err := appendAudit(callCtx, tx, record, time.Now().UTC()); err != nil {
		return "", fmt.Errorf("%w: %w", outbox.ErrOutcomeUnknown, err)
	}
	if err := tx.Commit(callCtx); err != nil {
		return "", fmt.Errorf("%w: commit dispatch: %w", outbox.ErrOutcomeUnknown, err)
	}
	return resolution.State, nil
}

func rollbackOutbox(tx pgx.Tx) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = tx.Rollback(ctx)
}

type outboxExecer interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

func settleOutbox(ctx context.Context, db outboxExecer, lease outbox.Lease, r outbox.Resolution) error {
	if !lease.Valid() {
		return outbox.ErrLeaseLost
	}
	if err := r.Validate(); err != nil {
		return err
	}
	// Clamp diagnostic storage; callbacks must not return secrets in errors.
	diagnostic := []rune(r.LastError)
	if len(diagnostic) > 2048 {
		diagnostic = diagnostic[:2048]
	}
	micros := r.RetryAfter.Microseconds()
	if r.RetryAfter > 0 && micros == 0 {
		micros = 1
	}
	const q = `UPDATE outbox_events SET state=$4,lease_owner=NULL,lease_until=NULL,
  next_attempt_at=CASE WHEN $4='PENDING' THEN clock_timestamp()+($5 * interval '1 microsecond') ELSE next_attempt_at END,
  last_error=NULLIF($6,''),dispatched_at=CASE WHEN $4='DISPATCHED' THEN clock_timestamp() ELSE dispatched_at END
  WHERE outbox_id=$1 AND state='LEASED' AND lease_owner=$2 AND attempt_count=$3
   AND lease_until>clock_timestamp()`
	tag, err := db.Exec(ctx, q, lease.ID, lease.Owner, lease.Attempt, string(r.State), micros, string(diagnostic))
	if err != nil {
		return fmt.Errorf("settle outbox: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return outbox.ErrLeaseLost
	}
	return nil
}

// These receipt-only methods never invoke a handler and require the same exact
// live lease as Dispatch. They do not authorize external side effects.
func (s *Store) MarkDispatched(ctx context.Context, lease outbox.Lease) error {
	if s == nil || s.pool == nil {
		return fmt.Errorf("PostgreSQL store is not configured")
	}
	return settleOutbox(ctx, s.pool, lease, outbox.Resolution{State: outbox.Dispatched})
}
func (s *Store) MarkRetry(ctx context.Context, lease outbox.Lease, delay time.Duration, lastError string) error {
	if s == nil || s.pool == nil {
		return fmt.Errorf("PostgreSQL store is not configured")
	}
	return settleOutbox(ctx, s.pool, lease, outbox.Resolution{State: outbox.Pending, RetryAfter: delay, LastError: lastError})
}
func (s *Store) MarkDeadLetter(ctx context.Context, lease outbox.Lease, lastError string) error {
	if s == nil || s.pool == nil {
		return fmt.Errorf("PostgreSQL store is not configured")
	}
	return settleOutbox(ctx, s.pool, lease, outbox.Resolution{State: outbox.DeadLetter, LastError: lastError})
}

func (s *Store) GetOutbox(ctx context.Context, key string) (outbox.Message, error) {
	if s == nil || s.pool == nil {
		return outbox.Message{}, fmt.Errorf("PostgreSQL store is not configured")
	}
	m, err := scanOutbox(s.pool.QueryRow(ctx, `SELECT `+outboxColumns+` FROM outbox_events WHERE outbox_key=$1`, key))
	if errors.Is(err, pgx.ErrNoRows) {
		return outbox.Message{}, outbox.ErrLeaseLost
	}
	return m, err
}
