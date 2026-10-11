package store

import (
	"context"
	"errors"
	"testing"

	"github.com/jiying2007/engineering-platform/internal/recovery"
)

// A non-scoped Recovery backend cannot be admitted as Core Store.
var _ Store = (*Memory)(nil)

func TestMemoryRecoveryLifecycleRequiredContextRejectsCancelledMutations(t *testing.T) {
	core := NewMemory()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := core.GetRecoveryContext(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled recovery read escaped caller context: %v", err)
	}
	if _, err := core.BeginRecoveryContext(ctx, 0); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled Recovery Begin escaped caller context: %v", err)
	}
	state, err := core.GetRecovery()
	if err != nil || state.Epoch != 0 || state.Mode != recovery.Normal {
		t.Fatalf("cancelled begin mutated durable epoch: state=%+v err=%v", state, err)
	}

	state, err = core.BeginRecoveryContext(context.Background(), 0)
	if err != nil || state.Epoch != 1 || state.Mode != recovery.RecoveryReconciliation {
		t.Fatalf("valid scoped begin rejected: state=%+v err=%v", state, err)
	}
	if _, err := core.CompleteRecoveryContext(ctx, 1, true); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled Recovery Complete escaped caller context: %v", err)
	}
	state, err = core.GetRecovery()
	if err != nil || state.Epoch != 1 || state.Mode != recovery.RecoveryReconciliation {
		t.Fatalf("cancelled complete advanced authority: state=%+v err=%v", state, err)
	}
	state, err = core.CompleteRecoveryContext(context.Background(), 1, true)
	if err != nil || state.Mode != recovery.Normal {
		t.Fatalf("valid scoped complete rejected: state=%+v err=%v", state, err)
	}
}
