package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	corestore "github.com/jiying2007/engineering-platform/internal/store"
)

func TestEvidenceVerificationContextCancelsRealPostgresLockWithoutAuthorityLeak(t *testing.T) {
	s := newIsolatedIntegrationStore(t)
	seedRestoreAuthority(t, s)
	ctx, stop := context.WithTimeout(context.Background(), 10*time.Second)
	defer stop()

	t.Run("evidence", func(t *testing.T) {
		original, err := s.GetEvidenceContext(ctx, "restore-evidence")
		if err != nil {
			t.Fatal(err)
		}
		original.ID = "cancelled-evidence"
		lock, err := s.pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer rollbackOutbox(lock)
		if _, err := lock.Exec(ctx, "LOCK TABLE evidence IN ACCESS EXCLUSIVE MODE"); err != nil {
			t.Fatal(err)
		}
		caller, cancel := context.WithCancel(ctx)
		defer cancel()
		done := make(chan error, 1)
		go func() { done <- s.CreateEvidenceContext(caller, original) }()
		select {
		case err := <-done:
			t.Fatalf("Evidence creation passed locked table: %v", err)
		case <-time.After(80 * time.Millisecond):
		}
		cancel()
		select {
		case err := <-done:
			if err == nil || caller.Err() != context.Canceled {
				t.Fatalf("Evidence ignored canceled caller: %v", err)
			}
		case <-ctx.Done():
			t.Fatal("Evidence SQL did not honor deadline before lock release")
		}
		if err := lock.Rollback(ctx); err != nil {
			t.Fatal(err)
		}
		if _, err := s.GetEvidenceContext(ctx, original.ID); !errors.Is(err, corestore.ErrNotFound) {
			t.Fatalf("cancelled Evidence left a persisted record: %v", err)
		}
		var count int
		if err := s.pool.QueryRow(ctx,
			"SELECT count(*) FROM audit_events WHERE event_type='evidence.registered' AND aggregate_id=$1",
			original.ID,
		).Scan(&count); err != nil || count != 0 {
			t.Fatalf("canceled Evidence wrote audit: count=%d err=%v", count, err)
		}
	})

	t.Run("verification", func(t *testing.T) {
		original, err := s.GetVerificationContext(ctx, "restore-verification")
		if err != nil {
			t.Fatal(err)
		}
		original.ID = "cancelled-verification"
		original.CreatedAt = time.Now().UTC()
		lock, err := s.pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer rollbackOutbox(lock)
		if _, err := lock.Exec(ctx, "LOCK TABLE verification_reports IN ACCESS EXCLUSIVE MODE"); err != nil {
			t.Fatal(err)
		}
		caller, cancel := context.WithCancel(ctx)
		defer cancel()
		done := make(chan error, 1)
		go func() { done <- s.CreateVerificationContext(caller, original) }()
		select {
		case err := <-done:
			t.Fatalf("Verification creation passed locked table: %v", err)
		case <-time.After(80 * time.Millisecond):
		}
		cancel()
		select {
		case err := <-done:
			if err == nil || caller.Err() != context.Canceled {
				t.Fatalf("Verification ignored canceled caller: %v", err)
			}
		case <-ctx.Done():
			t.Fatal("Verification SQL did not honor deadline before lock release")
		}
		if err := lock.Rollback(ctx); err != nil {
			t.Fatal(err)
		}
		if _, err := s.GetVerificationContext(ctx, original.ID); !errors.Is(err, corestore.ErrNotFound) {
			t.Fatalf("cancelled Verification left a persisted report: %v", err)
		}
		var count int
		if err := s.pool.QueryRow(ctx,
			"SELECT count(*) FROM audit_events WHERE event_type='verification.created' AND aggregate_id=$1",
			original.ID,
		).Scan(&count); err != nil || count != 0 {
			t.Fatalf("canceled Verification wrote audit: count=%d err=%v", count, err)
		}
	})
	if _, err := s.GetEvidenceContext(ctx, "restore-evidence"); err != nil {
		t.Fatalf("historical Evidence readback lost: %v", err)
	}
	if _, err := s.GetVerificationContext(ctx, "restore-verification"); err != nil {
		t.Fatalf("historical Verification readback lost: %v", err)
	}
}
