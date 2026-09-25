package postgres

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/jiying2007/engineering-platform/internal/recovery"
	corestore "github.com/jiying2007/engineering-platform/internal/store"
)

func TestPostgresRecoveryLifecycle(t *testing.T) {
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

	if _, err := s.pool.Exec(ctx, "UPDATE platform_state SET recovery_epoch=0,recovery_mode='NORMAL',updated_at=now() WHERE singleton_id=true"); err != nil {
		t.Fatal(err)
	}

	state, err := s.GetRecovery()
	if err != nil {
		t.Fatal(err)
	}
	if state.Epoch != 0 || state.Mode != recovery.Normal {
		t.Fatalf("unexpected initial recovery state: %#v", state)
	}

	active, err := s.BeginRecovery(0)
	if err != nil {
		t.Fatal(err)
	}
	if active.Epoch != 1 || active.Mode != recovery.RecoveryReconciliation {
		t.Fatalf("unexpected active recovery state: %#v", active)
	}

	if _, err := s.BeginRecovery(0); !errors.Is(err, corestore.ErrConflict) {
		t.Fatalf("expected stale/duplicate begin conflict, got %v", err)
	}

	if _, err := s.CompleteRecovery(1, false); !errors.Is(err, recovery.ErrReconciliationRequired) {
		t.Fatalf("expected reconciliation requirement, got %v", err)
	}

	stillActive, err := s.GetRecovery()
	if err != nil {
		t.Fatal(err)
	}
	if stillActive.Mode != recovery.RecoveryReconciliation {
		t.Fatalf("failed completion must keep recovery active: %#v", stillActive)
	}

	if _, err := s.CompleteRecovery(1, true); !errors.Is(err, recovery.ErrReconciliationRequired) {
		t.Fatalf("completion without durable proof accepted: %v", err)
	}
	proof, err := s.CreateRecoveryProof(ctx, 1, "urn:engineering-platform:operator:reconciler")
	if err != nil {
		t.Fatal(err)
	}
	if proof.RecoveryEpoch != 1 || !proof.Facts.Clear() {
		t.Fatalf("unexpected recovery proof: %#v", proof)
	}
	if err := s.AuthorizeCompletion(ctx, proof.Reconciler, 1); err == nil {
		t.Fatal("same reconciler identity authorized completion")
	}
	if err := s.AuthorizeCompletion(ctx, "urn:engineering-platform:operator:completer", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CompleteRecovery(0, true); !errors.Is(err, corestore.ErrConflict) {
		t.Fatalf("expected stale recovery epoch conflict, got %v", err)
	}

	normal, err := s.CompleteRecovery(1, true)
	if err != nil {
		t.Fatal(err)
	}
	if normal.Epoch != 1 || normal.Mode != recovery.Normal {
		t.Fatalf("unexpected completed recovery state: %#v", normal)
	}
}
