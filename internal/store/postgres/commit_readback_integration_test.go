package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/recovery"
	corestore "github.com/jiying2007/engineering-platform/internal/store"
)

// Simulates only the ACK-loss boundary *after* a real durable transaction.
// This is not a live network fault-injection or production recovery proof.
func TestSimulatedLostAcknowledgementAfterCommittedRecoveryReadsBackOneEpoch(t *testing.T) {
	s := newIsolatedIntegrationStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	committed, err := s.BeginRecoveryContext(ctx, 0)
	if err != nil || committed.Epoch != 1 || committed.Mode != recovery.RecoveryReconciliation {
		t.Fatalf("real PostgreSQL Recovery BEGIN failed: state=%+v err=%v", committed, err)
	}
	ackError := classifyCommitError(errors.New("simulated lost acknowledgement after real COMMIT"))
	if !errors.Is(ackError, corestore.ErrCommitOutcomeUnknown) {
		t.Fatalf("lost ACK was treated as known rollback: %v", ackError)
	}
	observed, err := s.GetRecoveryContext(ctx)
	if err != nil || observed != committed {
		t.Fatalf("authoritative exact-epoch readback lost durable transition: state=%+v err=%v", observed, err)
	}
	var count int
	if err := s.pool.QueryRow(ctx,
		"SELECT count(*) FROM audit_events WHERE event_type='recovery.started' AND aggregate_id='singleton'",
	).Scan(&count); err != nil || count != 1 {
		t.Fatalf("durable audit history does not match exact Recovery epoch: count=%d err=%v", count, err)
	}
	if _, err := s.BeginRecoveryContext(ctx, 0); !errors.Is(err, corestore.ErrConflict) {
		t.Fatalf("repeating unknown-ACK Recovery BEGIN caused a second epoch: %v", err)
	}
}
