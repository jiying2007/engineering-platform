package postgres

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/action"
	"github.com/jiying2007/engineering-platform/internal/outbox"
	"github.com/jiying2007/engineering-platform/internal/recovery"
)

func TestPostgresOutboxRecoveryGateAndLeaseRecovery(t *testing.T) {
	url := os.Getenv("POSTGRES_TEST_URL")
	if url == "" {
		t.Skip("POSTGRES_TEST_URL is not set")
	}

	ctx := context.Background()
	s, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.ApplyCoreMigration(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx, "DELETE FROM outbox_events"); err != nil {
		t.Fatal(err)
	}

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	observeKey := "outbox-observe-" + suffix
	mutateKey := "outbox-mutate-" + suffix

	if err := insertOutboxFixture(ctx, s, OutboxMessage{
		Key:           observeKey,
		Topic:         "test.observe",
		AggregateType: "Test",
		AggregateID:   suffix,
		RiskClass:     action.Observe,
		Payload:       map[string]any{"kind": "observe"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := insertOutboxFixture(ctx, s, OutboxMessage{
		Key:           mutateKey,
		Topic:         "test.mutate",
		AggregateType: "Test",
		AggregateID:   suffix,
		RiskClass:     action.ControlledMutation,
		Payload:       map[string]any{"kind": "mutate"},
	}); err != nil {
		t.Fatal(err)
	}

	recoveryState, err := s.GetRecovery()
	if err != nil {
		t.Fatal(err)
	}
	if recoveryState.Mode != recovery.Normal {
		t.Fatalf("expected NORMAL recovery state, got %#v", recoveryState)
	}
	recoveryState, err = s.BeginRecovery(recoveryState.Epoch)
	if err != nil {
		t.Fatal(err)
	}

	claimed, err := s.Claim(ctx, "worker-a", 10, 100*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if len(claimed) != 1 || claimed[0].Key != observeKey {
		t.Fatalf("recovery mode must claim only OBSERVE messages: %#v", claimed)
	}
	if err := s.MarkDispatched(ctx, claimed[0].ID, "worker-a"); err != nil {
		t.Fatal(err)
	}

	if _, err := s.CompleteRecovery(recoveryState.Epoch, true); err != nil {
		t.Fatal(err)
	}

	claimed, err = s.Claim(ctx, "worker-a", 10, 20*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if len(claimed) != 1 || claimed[0].Key != mutateKey || claimed[0].AttemptCount != 1 {
		t.Fatalf("expected controlled mutation after recovery, got %#v", claimed)
	}
	firstID := claimed[0].ID

	secondWorker, err := s.Claim(ctx, "worker-b", 10, 20*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if len(secondWorker) != 0 {
		t.Fatalf("active lease must exclude other workers: %#v", secondWorker)
	}

	time.Sleep(35 * time.Millisecond)
	reclaimed, err := s.Claim(ctx, "worker-b", 10, 100*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if len(reclaimed) != 1 || reclaimed[0].ID != firstID || reclaimed[0].AttemptCount != 2 {
		t.Fatalf("expired lease should be reclaimed with incremented attempt count: %#v", reclaimed)
	}
	if err := s.MarkDispatched(ctx, reclaimed[0].ID, "worker-b"); err != nil {
		t.Fatal(err)
	}

	stored, err := s.GetOutbox(ctx, mutateKey)
	if err != nil {
		t.Fatal(err)
	}
	if stored.State != outbox.Dispatched || stored.AttemptCount != 2 || stored.DispatchedAt.IsZero() {
		t.Fatalf("unexpected final outbox state: %#v", stored)
	}
}

func TestPostgresOutboxDispatcherRetryAndDeadLetter(t *testing.T) {
	url := os.Getenv("POSTGRES_TEST_URL")
	if url == "" {
		t.Skip("POSTGRES_TEST_URL is not set")
	}

	ctx := context.Background()
	s, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.ApplyCoreMigration(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx, "DELETE FROM outbox_events"); err != nil {
		t.Fatal(err)
	}

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	key := "outbox-dispatcher-" + suffix
	if err := insertOutboxFixture(ctx, s, OutboxMessage{
		Key:           key,
		Topic:         "test.fail",
		AggregateType: "Test",
		AggregateID:   suffix,
		RiskClass:     action.Observe,
		Payload:       map[string]any{"kind": "fail"},
	}); err != nil {
		t.Fatal(err)
	}

	dispatcher := outbox.Dispatcher{
		Repository: s,
		Handlers: map[string]outbox.Handler{
			"test.fail": outbox.HandlerFunc(func(context.Context, outbox.Message) error {
				return fmt.Errorf("simulated downstream failure")
			}),
		},
		LeaseDuration: 100 * time.Millisecond,
		RetryDelay:    time.Nanosecond,
		MaxAttempts:   2,
		Now:           func() time.Time { return time.Now().UTC() },
	}

	first, err := dispatcher.DispatchBatch(ctx, "worker-a", 10)
	if err != nil {
		t.Fatal(err)
	}
	if first.Retried != 1 {
		t.Fatalf("expected one retry, got %#v", first)
	}

	time.Sleep(time.Millisecond)
	second, err := dispatcher.DispatchBatch(ctx, "worker-a", 10)
	if err != nil {
		t.Fatal(err)
	}
	if second.DeadLettered != 1 {
		t.Fatalf("expected dead letter on second attempt, got %#v", second)
	}
	stored, err := s.GetOutbox(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	if stored.State != outbox.DeadLetter || stored.LastError == "" || stored.AttemptCount != 2 {
		t.Fatalf("unexpected dead-letter row: %#v", stored)
	}
}

func insertOutboxFixture(ctx context.Context, s *Store, message OutboxMessage) error {
	auditRecord, err := auditInput(
		"outbox.fixture",
		"Test",
		message.AggregateID,
		map[string]any{"key": message.Key, "topic": message.Topic},
	)
	if err != nil {
		return err
	}
	_, err = s.Mutate(ctx, Mutation{
		Audit:  auditRecord,
		Outbox: []OutboxMessage{message},
	})
	return err
}
