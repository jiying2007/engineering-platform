package store

import (
	"errors"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/core"
	"github.com/jiying2007/engineering-platform/internal/run"
	"github.com/jiying2007/engineering-platform/internal/session"
)

func TestTaskRevisionsAreImmutableAndMonotonic(t *testing.T) {
	s := NewMemory()
	r1 := core.TaskContract{
		ID:                 "task-1",
		WorkItemID:         "work-1",
		TaskType:           "FEATURE",
		Repository:         "repo",
		BaseCommit:         "0123456789abcdef0123456789abcdef01234567",
		AcceptanceCriteria: []string{"A"},
		Revision:           1,
	}
	if err := s.CreateTask(r1); err != nil {
		t.Fatal(err)
	}
	d1, _ := r1.Digest()

	r2 := r1
	r2.Revision = 2
	r2.AcceptanceCriteria = []string{"A", "B"}
	if err := s.CreateTask(r2); err != nil {
		t.Fatal(err)
	}
	d2, _ := r2.Digest()
	if d1 == d2 {
		t.Fatal("expected revision digest to change")
	}

	latest, err := s.GetTask("task-1")
	if err != nil {
		t.Fatal(err)
	}
	if latest.Revision != 2 {
		t.Fatalf("expected latest revision 2, got %d", latest.Revision)
	}
	old, err := s.GetTaskByDigest(d1)
	if err != nil {
		t.Fatal(err)
	}
	if old.Revision != 1 {
		t.Fatalf("historical digest should still resolve revision 1, got %d", old.Revision)
	}

	r4 := r2
	r4.Revision = 4
	if err := s.CreateTask(r4); !errors.Is(err, ErrConflict) {
		t.Fatalf("expected revision gap conflict, got %v", err)
	}
}

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
