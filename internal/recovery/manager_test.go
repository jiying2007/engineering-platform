package recovery

import (
	"errors"
	"testing"
)

func TestRecoveryBlocksIrreversibleActionsUntilReconciled(t *testing.T) {
	m := New()
	epoch := m.Begin()
	if err := m.AuthorizeIrreversible(epoch); !errors.Is(err, ErrRecoveryMode) {
		t.Fatalf("expected recovery guard, got %v", err)
	}
	if err := m.Complete(epoch, false); !errors.Is(err, ErrRecoveryMode) {
		t.Fatalf("expected reconciliation requirement, got %v", err)
	}
	if err := m.Complete(epoch, true); err != nil {
		t.Fatal(err)
	}
	if err := m.AuthorizeIrreversible(epoch); err != nil {
		t.Fatalf("expected action allowed after reconciliation: %v", err)
	}
}

func TestOldRecoveryEpochIsRejected(t *testing.T) {
	m := New()
	old := m.Begin()
	_ = m.Complete(old, true)
	current := m.Begin()
	if current == old {
		t.Fatal("expected new recovery epoch")
	}
	if err := m.CheckEpoch(old); !errors.Is(err, ErrStaleEpoch) {
		t.Fatalf("expected stale epoch, got %v", err)
	}
}
