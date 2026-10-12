package postgres

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/action"
)

func actionIdentityAuditCount(t *testing.T, s *Store, id string) int {
	t.Helper()
	var count int
	if err := s.pool.QueryRow(context.Background(), "SELECT count(*) FROM audit_events WHERE aggregate_type='ExternalOperation' AND aggregate_id=$1", id).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

// This regression runs on real PostgreSQL whenever POSTGRES_TEST_URL is set.
// A well-formed different Run must not authorize dispatch of the original row.
func TestPostgresActionRejectsBorrowedRunAuthority(t *testing.T) {
	s, original := actionAdmissionFixture(t)
	ctx := context.Background()
	if err := s.CreateContext(ctx, original); err != nil {
		t.Fatal(err)
	}
	alternate := setupPostgresActionRun(t, s, fmt.Sprintf("alternate-%d", time.Now().UnixNano()))
	if _, err := s.pool.Exec(ctx, "UPDATE runs SET state='PAUSED' WHERE run_id=$1", original.RunID); err != nil {
		t.Fatal(err)
	}
	next := original
	next.RunID = alternate
	next.State = action.Dispatched
	before := actionIdentityAuditCount(t, s, original.ID)
	if err := s.UpdateContext(ctx, next); !errors.Is(err, action.ErrInvalidTransition) {
		t.Fatalf("borrowed Run authorized original dispatch: %v", err)
	}
	read, err := s.GetContext(ctx, original.ID)
	if err != nil || read.State != action.Planned || read.RunID != original.RunID || read.IdempotencyKey != original.IdempotencyKey {
		t.Fatalf("reserved row changed: %+v %v", read, err)
	}
	if actionIdentityAuditCount(t, s, original.ID) != before {
		t.Fatal("rejected identity appended audit")
	}
}

func TestPostgresActionSettlementBindsEveryOriginalIdentityField(t *testing.T) {
	s, original := actionAdmissionFixture(t)
	ctx := context.Background()
	if err := s.CreateContext(ctx, original); err != nil {
		t.Fatal(err)
	}
	original.State = action.Dispatched
	if err := s.UpdateContext(ctx, original); err != nil {
		t.Fatal(err)
	}
	mutations := []struct {
		name   string
		change func(*action.Operation)
	}{
		{"run", func(o *action.Operation) { o.RunID = "different" }},
		{"execution-epoch", func(o *action.Operation) { o.ExecutionEpoch++ }},
		{"recovery-epoch", func(o *action.Operation) { o.RecoveryEpoch++ }},
		{"action", func(o *action.Operation) { o.Action = "different" }},
		{"risk", func(o *action.Operation) { o.RiskClass = action.Observe }},
		{"capability", func(o *action.Operation) { o.Capability = "different" }},
		{"idempotency", func(o *action.Operation) { o.IdempotencyKey = "different" }},
		{"request-digest", func(o *action.Operation) { o.RequestDigest = "sha256:different" }},
	}
	for _, tc := range mutations {
		t.Run(tc.name, func(t *testing.T) {
			before := actionIdentityAuditCount(t, s, original.ID)
			next := original
			next.State = action.Confirmed
			next.ExternalRef = "unbound"
			tc.change(&next)
			if err := s.UpdateContext(ctx, next); !errors.Is(err, action.ErrInvalidTransition) {
				t.Fatalf("altered settlement accepted: %v", err)
			}
			read, err := s.GetByIdempotencyKeyContext(ctx, original.IdempotencyKey)
			if err != nil || read.State != action.Dispatched || read.ExternalRef != "" || read.RunID != original.RunID || read.RequestDigest != original.RequestDigest {
				t.Fatalf("altered identity changed row: %+v %v", read, err)
			}
			if actionIdentityAuditCount(t, s, original.ID) != before {
				t.Fatal("rejected settlement appended audit")
			}
		})
	}
	// A later Recovery epoch is NOT a reason to lose a real original settlement.
	recovery, err := s.BeginRecoveryContext(ctx, original.RecoveryEpoch)
	if err != nil {
		t.Fatal(err)
	}
	if recovery.Epoch <= original.RecoveryEpoch {
		t.Fatal("recovery did not advance")
	}
	next := original
	next.State = action.Confirmed
	next.ExternalRef = "original-provider-result"
	next.ObservedState = "original-observation"
	if err := s.UpdateContext(ctx, next); err != nil {
		t.Fatalf("original settlement after Recovery refused: %v", err)
	}
	read, err := s.GetContext(ctx, original.ID)
	if err != nil || read.State != action.Confirmed || read.RecoveryEpoch != original.RecoveryEpoch || read.ExternalRef != next.ExternalRef {
		t.Fatalf("settlement rewrote original identity: %+v %v", read, err)
	}
	if actionIdentityAuditCount(t, s, original.ID) != 3 {
		t.Fatal("expected plan/dispatch/settlement audit exactly once")
	}
	if err := s.UpdateContext(ctx, next); !errors.Is(err, action.ErrInvalidTransition) {
		t.Fatal("duplicate settlement changed terminal row")
	}
}
