package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/core"
	corestore "github.com/jiying2007/engineering-platform/internal/store"
)

func TestReviewClosureContextCancelsRealPostgresLockWithoutAuditResidue(t *testing.T) {
	s := newIsolatedIntegrationStore(t)
	seedRestoreAuthority(t, s)
	ctx, stop := context.WithTimeout(context.Background(), 10*time.Second)
	defer stop()

	reviewReport, err := s.GetReviewContext(ctx, "restore-review")
	if err != nil {
		t.Fatal(err)
	}
	closure, err := s.GetClosureContext(ctx, "restore-closure")
	if err != nil {
		t.Fatal(err)
	}
	work, err := s.GetWorkContext(ctx, "restore-work")
	if err != nil {
		t.Fatal(err)
	}

	// This restored work is already CLOSED; the synthetic duplicate attempts
	// deliberately exercise blocked SQL cancellation, not authorization to
	// review/close again. The full valid transition is covered separately.
	for _, tc := range []struct {
		name      string
		table     string
		id        string
		eventType string
		auditID   string
		attempt   func(context.Context) error
		read      func() error
	}{
		{
			name:      "review",
			table:     "verification_reports",
			id:        "cancelled-review",
			eventType: "review.created",
			auditID:   "cancelled-review",
			attempt: func(c context.Context) error {
				r := reviewReport
				r.ID = "cancelled-review"
				next := work
				next.State = core.WorkReviewing
				return s.CreateReviewAndUpdateWorkContext(c, r, work.Version, next)
			},
			read: func() error {
				_, e := s.GetReviewContext(ctx, "cancelled-review")
				return e
			},
		},
		{
			name:      "closure",
			table:     "review_reports",
			id:        "cancelled-closure",
			eventType: "work.closed",
			auditID:   "restore-work",
			attempt: func(c context.Context) error {
				r := closure
				r.ID = "cancelled-closure"
				next := work
				next.State = core.WorkClosed
				return s.CreateClosureAndUpdateWorkContext(c, r, work.Version, next)
			},
			read: func() error {
				_, e := s.GetClosureContext(ctx, "cancelled-closure")
				return e
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var baseline int
			if err := s.pool.QueryRow(ctx, "SELECT count(*) FROM audit_events WHERE event_type=$1 AND aggregate_id=$2", tc.eventType, tc.auditID).Scan(&baseline); err != nil {
				t.Fatal(err)
			}
			lock, err := s.pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer rollbackOutbox(lock)
			if _, err := lock.Exec(ctx, "LOCK TABLE "+tc.table+" IN ACCESS EXCLUSIVE MODE"); err != nil {
				t.Fatal(err)
			}
			caller, cancel := context.WithCancel(ctx)
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- tc.attempt(caller) }()
			select {
			case err := <-done:
				t.Fatalf("mutation bypassed locked authority table: %v", err)
			case <-time.After(80 * time.Millisecond):
			}
			cancel()
			select {
			case err := <-done:
				if err == nil || caller.Err() != context.Canceled {
					t.Fatalf("mutation ignored cancelled caller: %v", err)
				}
			case <-ctx.Done():
				t.Fatal("Core SQL did not honor cancelled caller before lock release")
			}
			if err := lock.Rollback(ctx); err != nil {
				t.Fatal(err)
			}
			if err := tc.read(); !errors.Is(err, corestore.ErrNotFound) {
				t.Fatalf("cancelled mutation persisted an authority record: %v", err)
			}
			var count int
			if err := s.pool.QueryRow(ctx, "SELECT count(*) FROM audit_events WHERE event_type=$1 AND aggregate_id=$2", tc.eventType, tc.auditID).Scan(&count); err != nil || count != baseline {
				t.Fatalf("cancelled mutation changed audit count: before=%d after=%d err=%v", baseline, count, err)
			}
		})
	}
	if _, err := s.GetReviewContext(ctx, "restore-review"); err != nil {
		t.Fatalf("historical independent review lost: %v", err)
	}
	if _, err := s.GetClosureContext(ctx, "restore-closure"); err != nil {
		t.Fatalf("historical closure lost: %v", err)
	}
}
