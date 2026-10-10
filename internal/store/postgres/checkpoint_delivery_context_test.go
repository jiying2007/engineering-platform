package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/core"
	"github.com/jiying2007/engineering-platform/internal/session"
	corestore "github.com/jiying2007/engineering-platform/internal/store"
)

func TestCheckpointContextCancelsDatabaseLockWithoutAuditOrReservation(t *testing.T) {
	s := newIsolatedIntegrationStore(t)
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	runID := setupPostgresActionRun(t, s, suffix)
	value, _, err := s.GetExecution(runID)
	if err != nil {
		t.Fatal(err)
	}
	cp := session.Checkpoint{
		ID: "cancel-checkpoint-" + suffix,
		RunID: runID,
		TaskContractDigest: value.TaskContractDigest,
		RunInputManifestDigest: value.RunInputManifestDigest,
		ExecutionEpoch: value.CurrentEpoch,
		SourceTreeDigest: "sha256:source-tree",
		Objective: "test caller cancellation",
		CreatedAt: time.Now().UTC(),
	}
	if _, err := cp.Digest(); err != nil {
		t.Fatal(err)
	}

	ctx, stop := context.WithTimeout(context.Background(), 10*time.Second)
	defer stop()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rollbackOutbox(tx)
	if _, err := tx.Exec(ctx, "LOCK TABLE checkpoints IN ACCESS EXCLUSIVE MODE"); err != nil {
		t.Fatal(err)
	}

	caller, cancel := context.WithCancel(ctx)
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, err := s.CreateCheckpointContext(caller, cp)
		result <- err
	}()
	select {
	case err := <-result:
		t.Fatalf("checkpoint bypassed held table lock: %v", err)
	case <-time.After(80 * time.Millisecond):
	}
	cancel()
	select {
	case err := <-result:
		if err == nil || caller.Err() != context.Canceled {
			t.Fatalf("checkpoint cancellation was ignored: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("checkpoint transaction did not stop under caller cancellation")
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.GetCheckpointContext(ctx, cp.ID); !errors.Is(err, corestore.ErrNotFound) {
		t.Fatalf("canceled checkpoint retained a record: %v", err)
	}
	var auditCount int
	if err := s.pool.QueryRow(ctx,
		"SELECT count(*) FROM audit_events WHERE event_type='checkpoint.created' AND aggregate_id=$1", cp.RunID,
	).Scan(&auditCount); err != nil || auditCount != 0 {
		t.Fatalf("checkpoint cancellation left audit residue: count=%d err=%v", auditCount, err)
	}
}

func TestDeliveryContextCancelsDatabaseLockWithoutAuditOrSubject(t *testing.T) {
	s := newIsolatedIntegrationStore(t)
	seedRestoreAuthority(t, s)
	item, err := s.GetDeliveryContext(context.Background(), "restore-delivery")
	if err != nil {
		t.Fatal(err)
	}
	item.ID = "cancelled-delivery"
	item.ResultCommit = strings.Repeat("d", 40)
	item.CreatedAt = time.Now().UTC()
	item.SubjectDigest, err = item.CalculateSubjectDigest()
	if err != nil {
		t.Fatal(err)
	}

	ctx, stop := context.WithTimeout(context.Background(), 10*time.Second)
	defer stop()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rollbackOutbox(tx)
	if _, err := tx.Exec(ctx, "LOCK TABLE delivery_receipts IN ACCESS EXCLUSIVE MODE"); err != nil {
		t.Fatal(err)
	}
	caller, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- s.CreateDeliveryContext(caller, item) }()
	select {
	case err := <-done:
		t.Fatalf("delivery bypassed held table lock: %v", err)
	case <-time.After(80 * time.Millisecond):
	}
	cancel()
	select {
	case err := <-done:
		if err == nil || caller.Err() != context.Canceled {
			t.Fatalf("delivery ignored caller cancellation: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("delivery lock wait did not stop")
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetDeliveryContext(ctx, item.ID); !errors.Is(err, corestore.ErrNotFound) {
		t.Fatalf("canceled delivery was committed: %v", err)
	}
	var count int
	if err := s.pool.QueryRow(ctx,
		"SELECT count(*) FROM audit_events WHERE event_type='delivery.created' AND aggregate_id=$1", item.ID,
	).Scan(&count); err != nil || count != 0 {
		t.Fatalf("canceled delivery wrote audit event: count=%d err=%v", count, err)
	}
	// Historical source Delivery is unchanged by the canceled request.
	if existing, err := s.GetDeliveryContext(ctx, "restore-delivery"); err != nil ||
		existing.SubjectDigest == item.SubjectDigest || existing.ID != "restore-delivery" {
		t.Fatalf("cancellation mutated immutable original delivery: %#v err=%v", existing, err)
	}
}

var _ = core.DeliveryReceipt{}
