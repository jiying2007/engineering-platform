package postgres

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jiying2007/engineering-platform/internal/action"
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
func insertWorkMutation(id string, messages []OutboxMessage) Mutation {
	return Mutation{
		Apply: func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx, "INSERT INTO work_items (work_item_id,title,human_owner,state,version,created_at) VALUES ($1,$2,$3,'DRAFT',1,$4)", id, "PostgreSQL test work", "test", time.Unix(1, 0).UTC())
			return err
		},
		Audit:  audit.Input{Type: "work.created", AggregateType: "WorkItem", AggregateID: id, PayloadDigest: "sha256:" + id, CorrelationID: "test-" + id},
		Outbox: messages,
	}
}
func readAuditEvents(t *testing.T, s *Store) []audit.Event {
	t.Helper()
	rows, err := s.pool.Query(context.Background(), `SELECT sequence,event_type,COALESCE(aggregate_type,''),COALESCE(aggregate_id,''),payload_digest,COALESCE(correlation_id,''),COALESCE(causation_id,''),COALESCE(previous_digest,''),event_digest,created_at FROM audit_events ORDER BY sequence`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var events []audit.Event
	for rows.Next() {
		var e audit.Event
		if err := rows.Scan(&e.Sequence, &e.Type, &e.AggregateType, &e.AggregateID, &e.PayloadDigest, &e.CorrelationID, &e.CausationID, &e.PreviousDigest, &e.Digest, &e.CreatedAt); err != nil {
			t.Fatal(err)
		}
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return events
}
func assertCount(t *testing.T, s *Store, query string, want int) {
	t.Helper()
	var count int
	if err := s.pool.QueryRow(context.Background(), query).Scan(&count); err != nil || count != want {
		t.Fatalf("%s: count=%d want=%d err=%v", query, count, want, err)
	}
}
func TestMutationCommitsBusinessAuditAndOutboxAtomically(t *testing.T) {
	s := integrationStore(t)
	result, err := s.Mutate(context.Background(), insertWorkMutation("work-1", []OutboxMessage{{Key: "work-1.created", Topic: "work.created", RiskClass: action.Observe, AggregateType: "WorkItem", AggregateID: "work-1", Payload: map[string]any{"work_item_id": "work-1"}}}))
	if err != nil {
		t.Fatal(err)
	}
	if result.AuditEvent.Sequence != 1 || len(result.OutboxIDs) != 1 {
		t.Fatalf("%+v", result)
	}
	assertCount(t, s, "SELECT count(*) FROM work_items WHERE work_item_id='work-1'", 1)
	assertCount(t, s, "SELECT count(*) FROM outbox_events WHERE outbox_key='work-1.created'", 1)
	events := readAuditEvents(t, s)
	if len(events) != 1 {
		t.Fatalf("events=%d", len(events))
	}
	if err := audit.Verify(events); err != nil {
		t.Fatal(err)
	}
	var sequence uint64
	var digest string
	if err := s.pool.QueryRow(context.Background(), "SELECT last_sequence,last_digest FROM audit_journal_state WHERE singleton_id=true").Scan(&sequence, &digest); err != nil {
		t.Fatal(err)
	}
	if sequence != 1 || digest != events[0].Digest {
		t.Fatalf("audit head=%d %s", sequence, digest)
	}
}
func TestRollbackDoesNotConsumeAuditSequence(t *testing.T) {
	s := integrationStore(t)
	ctx := context.Background()
	first, err := s.Mutate(ctx, insertWorkMutation("work-1", nil))
	if err != nil || first.AuditEvent.Sequence != 1 {
		t.Fatalf("%+v %v", first, err)
	}
	_, err = s.Mutate(ctx, insertWorkMutation("work-rollback", []OutboxMessage{
		{Key: "duplicate-key", Topic: "test", RiskClass: action.Observe, Payload: map[string]any{"n": 1}},
		{Key: "duplicate-key", Topic: "test", RiskClass: action.Observe, Payload: map[string]any{"n": 2}},
	}))
	// Prove uniqueness caused the failure; missing risk must not make this test
	// pass before the intended database constraint is exercised.
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		t.Fatalf("expected duplicate outbox constraint, got %v", err)
	}
	assertCount(t, s, "SELECT count(*) FROM work_items WHERE work_item_id='work-rollback'", 0)
	assertCount(t, s, "SELECT count(*) FROM audit_events", 1)
	assertCount(t, s, "SELECT count(*) FROM outbox_events WHERE outbox_key='duplicate-key'", 0)
	assertCount(t, s, "SELECT last_sequence FROM audit_journal_state WHERE singleton_id=true", 1)
	next, err := s.Mutate(ctx, insertWorkMutation("work-2", nil))
	if err != nil || next.AuditEvent.Sequence != 2 {
		t.Fatalf("sequence consumed: %+v %v", next, err)
	}
}
func TestMutationRollsBackWhenAuditCannotBeBuilt(t *testing.T) {
	s := integrationStore(t)
	m := insertWorkMutation("work-invalid-audit", nil)
	m.Audit.Type = ""
	if _, err := s.Mutate(context.Background(), m); err == nil {
		t.Fatal("invalid audit accepted")
	}
	assertCount(t, s, "SELECT count(*) FROM work_items WHERE work_item_id='work-invalid-audit'", 0)
}
func TestConcurrentMutationsProduceContiguousAuditChain(t *testing.T) {
	s := integrationStore(t)
	const workers = 8
	results := make(chan error, workers)
	for i := 0; i < workers; i++ {
		go func(i int) {
			_, err := s.Mutate(context.Background(), insertWorkMutation(fmt.Sprintf("work-%02d", i), nil))
			results <- err
		}(i)
	}
	for i := 0; i < workers; i++ {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	events := readAuditEvents(t, s)
	if len(events) != workers {
		t.Fatalf("audit count=%d", len(events))
	}
	if err := audit.Verify(events); err != nil {
		t.Fatal(err)
	}
}
func TestMutationApplyErrorRollsBack(t *testing.T) {
	s := integrationStore(t)
	m := insertWorkMutation("work-apply-error", nil)
	apply := m.Apply
	m.Apply = func(ctx context.Context, tx pgx.Tx) error {
		if err := apply(ctx, tx); err != nil {
			return err
		}
		return errors.New("synthetic business failure")
	}
	if _, err := s.Mutate(context.Background(), m); err == nil {
		t.Fatal("apply error accepted")
	}
	assertCount(t, s, "SELECT count(*) FROM work_items WHERE work_item_id='work-apply-error'", 0)
}
