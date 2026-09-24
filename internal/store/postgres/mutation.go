package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/jiying2007/engineering-platform/internal/action"
	"github.com/jiying2007/engineering-platform/internal/audit"
	"github.com/jiying2007/engineering-platform/internal/outbox"
)

type OutboxMessage struct {
	Key           string
	Topic         string
	AggregateType string
	AggregateID   string
	RiskClass     action.RiskClass
	Payload       any
}

type Mutation struct {
	Apply  func(context.Context, pgx.Tx) error
	Audit  audit.Input
	Outbox []OutboxMessage
}

type MutationResult struct {
	AuditEvent audit.Event
	OutboxIDs  []int64
}

func (s *Store) Mutate(ctx context.Context, mutation Mutation) (MutationResult, error) {
	if s == nil || s.pool == nil {
		return MutationResult{}, fmt.Errorf("PostgreSQL store is not configured")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return MutationResult{}, fmt.Errorf("begin authority transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if mutation.Apply != nil {
		if err := mutation.Apply(ctx, tx); err != nil {
			return MutationResult{}, fmt.Errorf("apply business mutation: %w", err)
		}
	}
	event, err := appendAudit(ctx, tx, mutation.Audit, time.Now().UTC())
	if err != nil {
		return MutationResult{}, err
	}
	outboxIDs := make([]int64, 0, len(mutation.Outbox))
	for _, message := range mutation.Outbox {
		id, err := insertOutbox(ctx, tx, message)
		if err != nil {
			return MutationResult{}, err
		}
		outboxIDs = append(outboxIDs, id)
	}
	if err := tx.Commit(ctx); err != nil {
		return MutationResult{}, fmt.Errorf("commit authority transaction: %w", err)
	}
	return MutationResult{AuditEvent: event, OutboxIDs: outboxIDs}, nil
}

func appendAudit(ctx context.Context, tx pgx.Tx, input audit.Input, now time.Time) (audit.Event, error) {
	const selectHead = "SELECT last_sequence, last_digest FROM audit_journal_state WHERE singleton_id = true FOR UPDATE"
	var lastSequence uint64
	var lastDigest string
	if err := tx.QueryRow(ctx, selectHead).Scan(&lastSequence, &lastDigest); err != nil {
		return audit.Event{}, fmt.Errorf("lock audit journal head: %w", err)
	}
	event, err := audit.Build(lastSequence+1, lastDigest, input, now)
	if err != nil {
		return audit.Event{}, fmt.Errorf("build audit event: %w", err)
	}
	const insertEvent = "INSERT INTO audit_events (sequence,event_type,aggregate_type,aggregate_id,payload_digest,previous_digest,event_digest,correlation_id,causation_id,created_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)"
	if _, err := tx.Exec(ctx, insertEvent, event.Sequence, event.Type, nullIfEmpty(event.AggregateType), nullIfEmpty(event.AggregateID), event.PayloadDigest, nullIfEmpty(event.PreviousDigest), event.Digest, nullIfEmpty(event.CorrelationID), nullIfEmpty(event.CausationID), event.CreatedAt); err != nil {
		return audit.Event{}, fmt.Errorf("insert audit event: %w", err)
	}
	const updateHead = "UPDATE audit_journal_state SET last_sequence=$1,last_digest=$2 WHERE singleton_id=true"
	tag, err := tx.Exec(ctx, updateHead, event.Sequence, event.Digest)
	if err != nil {
		return audit.Event{}, fmt.Errorf("advance audit journal head: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return audit.Event{}, fmt.Errorf("advance audit journal head: expected one row, got %d", tag.RowsAffected())
	}
	return event, nil
}

func insertOutbox(ctx context.Context, tx pgx.Tx, message OutboxMessage) (int64, error) {
	if message.Key == "" {
		return 0, fmt.Errorf("outbox key is required")
	}
	if message.Topic == "" {
		return 0, fmt.Errorf("outbox topic is required")
	}
	payload, err := json.Marshal(message.Payload)
	if err != nil {
		return 0, fmt.Errorf("marshal outbox payload: %w", err)
	}
	riskClass, err := outbox.Classify(message.Topic, message.RiskClass)
	if err != nil {
		return 0, err
	}
	const insertMessage = "INSERT INTO outbox_events (outbox_key,topic,aggregate_type,aggregate_id,risk_class,payload_json) VALUES ($1,$2,$3,$4,$5,$6::jsonb) RETURNING outbox_id"
	var id int64
	if err := tx.QueryRow(ctx, insertMessage, message.Key, message.Topic, nullIfEmpty(message.AggregateType), nullIfEmpty(message.AggregateID), string(riskClass), string(payload)).Scan(&id); err != nil {
		return 0, fmt.Errorf("insert outbox message %q: %w", message.Key, err)
	}
	return id, nil
}

func nullIfEmpty(value string) any {
	if value == "" {
		return nil
	}
	return value
}
