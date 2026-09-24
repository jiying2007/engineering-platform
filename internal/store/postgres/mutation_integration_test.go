package postgres

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/jiying2007/engineering-platform/internal/audit"
)

func integrationStore(t *testing.T) *Store {
	t.Helper()
	url := os.Getenv("POSTGRES_TEST_URL")
	if url == "" {
		t.Skip("POSTGRES_TEST_URL is not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)

	s, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)

	if _, err := s.pool.Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public", pgx.QueryExecModeSimpleProtocol); err != nil {
		t.Fatalf("reset PostgreSQL schema: %v", err)
	}
	if err := s.ApplyCoreMigration(ctx); err != nil {
		t.Fatal(err)
	}
	return s
}

func insertWorkMutation(id string, outbox []OutboxMessage) Mutation {
	return Mutation{
		Apply: func(ctx context.Context, tx pgx.Tx) error {
			const insertWork = "INSERT INTO work_items (work_item_id,title,human_owner,state,version,created_at) VALUES ($1,$2,$3,'DRAFT',1,$4)"
			_, err := tx.Exec(ctx, insertWork, id, "PostgreSQL test work", "test", time.Unix(1, 0).UTC())
			return err
		},
		Audit: audit.Input{
			Type:          "work.created",
			AggregateType: "WorkItem",
			AggregateID:   id,
			PayloadDigest: "sha256:" + id,
			CorrelationID: "test-" + id,
		},
		Outbox: outbox,
	}
}

func TestMutationCommitsBusinessAuditAndOutboxAtomically(t *testing.T) {
	s := integrationStore(t)
	ctx := context.Background()

	result, err := s.Mutate(ctx, insertWorkMutation("work-1", []OutboxMessage{{
		Key:           "work-1.created",
		Topic:         "work.created",
		AggregateType: "WorkItem",
		AggregateID:   "work-1",
		Payload:       map[string]any{"work_item_id": "work-1"},
	}}))
	if err != nil {
		t.Fatal(err)
	}
	if result.AuditEvent.Sequence != 1 || len(result.OutboxIDs) != 1 {
		t.Fatalf("unexpected mutation result: %#v", result)
	}

	var workCount int
	if err := s.pool.QueryRow(ctx, "SELECT count(*) FROM work_items WHERE work_item_id='work-1'").Scan(&workCount); err != nil {
		t.Fatal(err)
	}
	if workCount != 1 {
		t.Fatalf("expected committed work row, got %d", workCount)
	}

	var event audit.Event
	const loadEvent = "SELECT sequence,event_type,COALESCE(aggregate_type,''),COALESCE(aggregate_id,''),payload_digest,COALESCE(correlation_id,''),COALESCE(causation_id,''),COALESCE(previous_digest,''),event_digest,created_at FROM audit_events WHERE sequence=1"
	if err := s.pool.QueryRow(ctx, loadEvent).Scan(
		&event.Sequence,
		&event.Type,
		&event.AggregateType,
		&event.AggregateID,
		&event.PayloadDigest,
		&event.CorrelationID,
		&event.CausationID,
		&event.PreviousDigest,
		&event.Digest,
		&event.CreatedAt,
	); err != nil {
		t.Fatal(err)
	}
	if err := audit.Verify([]audit.Event{event}); err != nil {
		t.Fatalf("persisted audit event did not verify: %v", err)
	}

	var headSequence uint64
	var headDigest string
	if err := s.pool.QueryRow(ctx, "SELECT last_sequence,last_digest FROM audit_journal_state WHERE singleton_id=true").Scan(&headSequence, &headDigest); err != nil {
		t.Fatal(err)
	}
	if headSequence != 1 || headDigest != event.Digest {
		t.Fatalf("unexpected audit head sequence=%d digest=%s", headSequence, headDigest)
	}

	var outboxCount int
	if err := s.pool.QueryRow(ctx, "SELECT count(*) FROM outbox_events WHERE outbox_key='work-1.created'").Scan(&outboxCount); err != nil {
		t.Fatal(err)
	}
	if outboxCount != 1 {
		t.Fatalf("expected committed outbox row, got %d", outboxCount)
	}
}

func TestRollbackDoesNotConsumeAuditSequence(t *testing.T) {
	s := integrationStore(t)
	ctx := context.Background()

	first, err := s.Mutate(ctx, insertWorkMutation("work-1", nil))
	if err != nil {
		t.Fatal(err)
	}
	if first.AuditEvent.Sequence != 1 {
		t.Fatalf("expected first sequence 1, got %d", first.AuditEvent.Sequence)
	}

	_, err = s.Mutate(ctx, insertWorkMutation("work-rollback", []OutboxMessage{
		{
			Key:     "duplicate-key",
			Topic:   "test",
			Payload: map[string]any{"n": 1},
		},
		{
			Key:     "duplicate-key",
			Topic:   "test",
			Payload: map[string]any{"n": 2},
		},
	}))
	if err == nil {
		t.Fatal("expected duplicate outbox key to roll back mutation")
	}

	var workCount int
	if err := s.pool.QueryRow(ctx, "SELECT count(*) FROM work_items WHERE work_item_id='work-rollback'").Scan(&workCount); err != nil {
		t.Fatal(err)
	}
	if workCount != 0 {
		t.Fatalf("business row survived rolled-back authority transaction: %d", workCount)
	}

	var auditCount int
	if err := s.pool.QueryRow(ctx, "SELECT count(*) FROM audit_events").Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if auditCount != 1 {
		t.Fatalf("audit row survived rollback; expected 1 prior row, got %d", auditCount)
	}

	var duplicateOutboxCount int
	if err := s.pool.QueryRow(ctx, "SELECT count(*) FROM outbox_events WHERE outbox_key='duplicate-key'").Scan(&duplicateOutboxCount); err != nil {
		t.Fatal(err)
	}
	if duplicateOutboxCount != 0 {
		t.Fatalf("outbox row survived rollback: %d", duplicateOutboxCount)
	}

	var headSequence uint64
	if err := s.pool.QueryRow(ctx, "SELECT last_sequence FROM audit_journal_state WHERE singleton_id=true").Scan(&headSequence); err != nil {
		t.Fatal(err)
	}
	if headSequence != 1 {
		t.Fatalf("rolled-back transaction consumed logical audit sequence: %d", headSequence)
	}

	next, err := s.Mutate(ctx, insertWorkMutation("work-2", nil))
	if err != nil {
		t.Fatal(err)
	}
	if next.AuditEvent.Sequence != 2 {
		t.Fatalf("expected next committed audit sequence 2, got %d", next.AuditEvent.Sequence)
	}
}

func TestMutationRollsBackWhenAuditCannotBeBuilt(t *testing.T) {
	s := integrationStore(t)
	ctx := context.Background()

	mutation := insertWorkMutation("work-invalid-audit", nil)
	mutation.Audit.Type = ""
	_, err := s.Mutate(ctx, mutation)
	if err == nil {
		t.Fatal("expected invalid audit event to fail mutation")
	}

	var count int
	if err := s.pool.QueryRow(ctx, "SELECT count(*) FROM work_items WHERE work_item_id='work-invalid-audit'").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("business state committed without audit: %d", count)
	}
}

func TestConcurrentMutationsProduceContiguousAuditChain(t *testing.T) {
	s := integrationStore(t)
	ctx := context.Background()

	const workers = 8
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		i := i
		go func() {
			id := fmt.Sprintf("work-%02d", i)
			_, err := s.Mutate(ctx, insertWorkMutation(id, nil))
			errs <- err
		}()
	}

	for i := 0; i < workers; i++ {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}

	const queryEvents = "SELECT sequence,event_type,COALESCE(aggregate_type,''),COALESCE(aggregate_id,''),payload_digest,COALESCE(correlation_id,''),COALESCE(causation_id,''),COALESCE(previous_digest,''),event_digest,created_at FROM audit_events ORDER BY sequence"
	rows, err := s.pool.Query(ctx, queryEvents)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	var events []audit.Event
	for rows.Next() {
		var event audit.Event
		if err := rows.Scan(
			&event.Sequence,
			&event.Type,
			&event.AggregateType,
			&event.AggregateID,
			&event.PayloadDigest,
			&event.CorrelationID,
			&event.CausationID,
			&event.PreviousDigest,
			&event.Digest,
			&event.CreatedAt,
		); err != nil {
			t.Fatal(err)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(events) != workers {
		t.Fatalf("expected %d events, got %d", workers, len(events))
	}
	if err := audit.Verify(events); err != nil {
		t.Fatalf("concurrent audit chain is invalid: %v", err)
	}
}

func TestMutationApplyErrorRollsBack(t *testing.T) {
	s := integrationStore(t)
	ctx := context.Background()

	_, err := s.Mutate(ctx, Mutation{
		Apply: func(ctx context.Context, tx pgx.Tx) error {
			const insertWork = "INSERT INTO work_items (work_item_id,title,human_owner,state,version,created_at) VALUES ('work-apply-error','x','test','DRAFT',1,$1)"
			if _, err := tx.Exec(ctx, insertWork, time.Unix(1, 0).UTC()); err != nil {
				return err
			}
			return errors.New("synthetic business failure")
		},
		Audit: audit.Input{
			Type:          "work.created",
			AggregateType: "WorkItem",
			AggregateID:   "work-apply-error",
			PayloadDigest: "sha256:work-apply-error",
		},
	})
	if err == nil {
		t.Fatal("expected synthetic business failure")
	}

	var count int
	if err := s.pool.QueryRow(ctx, "SELECT count(*) FROM work_items WHERE work_item_id='work-apply-error'").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("business mutation survived rollback: %d", count)
	}
}
