package store

import (
	"errors"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/run"
	"github.com/jiying2007/engineering-platform/internal/session"
)

func TestExecutionUpdateUsesOptimisticConcurrency(t *testing.T) {
	s := NewMemory()
	r := run.New("run-1", "sha256:task")
	attempt, err := r.StartAttempt("attempt-1", time.Unix(1, 0))
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(r.ID, attempt.Epoch)
	if err := s.CreateExecution(*r, *sess); err != nil {
		t.Fatal(err)
	}

	first, firstSession, err := s.GetExecution("run-1")
	if err != nil {
		t.Fatal(err)
	}
	stale, staleSession, err := s.GetExecution("run-1")
	if err != nil {
		t.Fatal(err)
	}

	if err := first.Pause(first.CurrentEpoch); err != nil {
		t.Fatal(err)
	}
	if err := firstSession.Pause(first.CurrentEpoch); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateExecution(first.ID, first.Version, first, firstSession); err != nil {
		t.Fatal(err)
	}

	if err := stale.Resume(stale.CurrentEpoch); err != nil {
		t.Fatal(err)
	}
	if err := staleSession.Resume(stale.CurrentEpoch); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateExecution(stale.ID, stale.Version, stale, staleSession); !errors.Is(err, ErrConflict) {
		t.Fatalf("expected optimistic concurrency conflict, got %v", err)
	}
}
