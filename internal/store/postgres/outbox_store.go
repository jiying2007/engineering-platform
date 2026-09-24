package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jiying2007/engineering-platform/internal/action"
	"github.com/jiying2007/engineering-platform/internal/outbox"
)

var _ outbox.Repository = (*Store)(nil)

func (s *Store) Claim(ctx context.Context, workerID string, limit int, leaseDuration time.Duration) ([]outbox.Message, error) {
	if s == nil || s.pool == nil {
		return nil, fmt.Errorf("PostgreSQL store is not configured")
	}
	if workerID == "" {
		return nil, fmt.Errorf("worker id is required")
	}
	if limit <= 0 {
		return nil, fmt.Errorf("claim limit must be positive")
	}
	if leaseDuration <= 0 {
		return nil, fmt.Errorf("lease duration must be positive")
	}

	const q = `
WITH candidates AS (
    SELECT o.outbox_id
    FROM outbox_events o
    WHERE (
            o.state = 'PENDING'
            OR (o.state = 'LEASED' AND o.lease_until < now())
          )
      AND o.next_attempt_at <= now()
      AND EXISTS (
            SELECT 1
            FROM platform_state ps
            WHERE ps.singleton_id = true
              AND (
                    ps.recovery_mode = 'NORMAL'
                    OR o.risk_class = 'OBSERVE'
                  )
          )
    ORDER BY o.outbox_id
    FOR UPDATE SKIP LOCKED
    LIMIT $1
)
UPDATE outbox_events o
SET state = 'LEASED',
    lease_owner = $2,
    lease_until = now() + ($3 * interval '1 millisecond'),
    attempt_count = attempt_count + 1,
    last_error = NULL
FROM candidates c
WHERE o.outbox_id = c.outbox_id
RETURNING
    o.outbox_id,
    o.outbox_key,
    o.topic,
    COALESCE(o.aggregate_type,''),
    COALESCE(o.aggregate_id,''),
    o.risk_class,
    o.payload_json,
    o.state,
    o.attempt_count,
    COALESCE(o.lease_owner,''),
    o.lease_until,
    o.next_attempt_at,
    COALESCE(o.last_error,''),
    o.created_at,
    o.dispatched_at`

	rows, err := s.pool.Query(ctx, q, limit, workerID, leaseDuration.Milliseconds())
	if err != nil {
		return nil, fmt.Errorf("claim outbox: %w", err)
	}
	defer rows.Close()

	var messages []outbox.Message
	for rows.Next() {
		var item outbox.Message
		var riskClass string
		var state string
		var payload []byte
		var leaseUntil pgtype.Timestamptz
		var dispatchedAt pgtype.Timestamptz
		if err := rows.Scan(
			&item.ID,
			&item.Key,
			&item.Topic,
			&item.AggregateType,
			&item.AggregateID,
			&riskClass,
			&payload,
			&state,
			&item.AttemptCount,
			&item.LeaseOwner,
			&leaseUntil,
			&item.NextAttemptAt,
			&item.LastError,
			&item.CreatedAt,
			&dispatchedAt,
		); err != nil {
			return nil, fmt.Errorf("scan claimed outbox: %w", err)
		}
		item.RiskClass = action.RiskClass(riskClass)
		item.State = outbox.State(state)
		item.Payload = append(json.RawMessage(nil), payload...)
		if leaseUntil.Valid {
			item.LeaseUntil = leaseUntil.Time
		}
		if dispatchedAt.Valid {
			item.DispatchedAt = dispatchedAt.Time
		}
		messages = append(messages, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("claim outbox rows: %w", err)
	}
	return messages, nil
}

func (s *Store) MarkDispatched(ctx context.Context, id int64, workerID string) error {
	const q = `
UPDATE outbox_events
SET state='DISPATCHED',
    lease_owner=NULL,
    lease_until=NULL,
    last_error=NULL,
    dispatched_at=now()
WHERE outbox_id=$1
  AND state='LEASED'
  AND lease_owner=$2`
	tag, err := s.pool.Exec(ctx, q, id, workerID)
	if err != nil {
		return fmt.Errorf("mark outbox dispatched: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return outbox.ErrLeaseLost
	}
	return nil
}

func (s *Store) MarkRetry(ctx context.Context, id int64, workerID string, next time.Time, lastError string) error {
	const q = `
UPDATE outbox_events
SET state='PENDING',
    lease_owner=NULL,
    lease_until=NULL,
    next_attempt_at=$3,
    last_error=$4
WHERE outbox_id=$1
  AND state='LEASED'
  AND lease_owner=$2`
	tag, err := s.pool.Exec(ctx, q, id, workerID, next, lastError)
	if err != nil {
		return fmt.Errorf("mark outbox retry: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return outbox.ErrLeaseLost
	}
	return nil
}

func (s *Store) MarkDeadLetter(ctx context.Context, id int64, workerID, lastError string) error {
	const q = `
UPDATE outbox_events
SET state='DEAD_LETTER',
    lease_owner=NULL,
    lease_until=NULL,
    last_error=$3
WHERE outbox_id=$1
  AND state='LEASED'
  AND lease_owner=$2`
	tag, err := s.pool.Exec(ctx, q, id, workerID, lastError)
	if err != nil {
		return fmt.Errorf("mark outbox dead letter: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return outbox.ErrLeaseLost
	}
	return nil
}

func (s *Store) GetOutbox(ctx context.Context, key string) (outbox.Message, error) {
	const q = `
SELECT outbox_id,outbox_key,topic,COALESCE(aggregate_type,''),COALESCE(aggregate_id,''),
       risk_class,payload_json,state,attempt_count,COALESCE(lease_owner,''),lease_until,
       next_attempt_at,COALESCE(last_error,''),created_at,dispatched_at
FROM outbox_events
WHERE outbox_key=$1`

	var item outbox.Message
	var riskClass, state string
	var payload []byte
	var leaseUntil, dispatchedAt pgtype.Timestamptz
	if err := s.pool.QueryRow(ctx, q, key).Scan(
		&item.ID,
		&item.Key,
		&item.Topic,
		&item.AggregateType,
		&item.AggregateID,
		&riskClass,
		&payload,
		&state,
		&item.AttemptCount,
		&item.LeaseOwner,
		&leaseUntil,
		&item.NextAttemptAt,
		&item.LastError,
		&item.CreatedAt,
		&dispatchedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return outbox.Message{}, outbox.ErrLeaseLost
		}
		return outbox.Message{}, fmt.Errorf("read outbox: %w", err)
	}
	item.RiskClass = action.RiskClass(riskClass)
	item.State = outbox.State(state)
	item.Payload = append(json.RawMessage(nil), payload...)
	if leaseUntil.Valid {
		item.LeaseUntil = leaseUntil.Time
	}
	if dispatchedAt.Valid {
		item.DispatchedAt = dispatchedAt.Time
	}
	return item, nil
}
