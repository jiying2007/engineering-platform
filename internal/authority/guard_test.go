package authority

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/action"
	"github.com/jiying2007/engineering-platform/internal/recovery"
	"github.com/jiying2007/engineering-platform/internal/run"
	"github.com/jiying2007/engineering-platform/internal/session"
)

type fakeSource struct {
	value run.Run
	sess  session.Session
	rec   recovery.Manager
	err   error
}

func (f fakeSource) GetExecution(string) (run.Run, session.Session, error) {
	return f.value, f.sess, f.err
}

func (f fakeSource) GetRecovery() (recovery.Manager, error) {
	if f.err != nil {
		return recovery.Manager{}, f.err
	}
	return f.rec, nil
}

func TestGuardChecksRunAndSessionEpoch(t *testing.T) {
	value := run.New("run-1", "sha256:task", "sha256:input")
	attempt, err := value.StartAttempt("attempt-1", time.Unix(1, 0))
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(value.ID, attempt.Epoch)
	g := New(fakeSource{value: *value, sess: *sess, rec: *recovery.New()})
	if err := g.CheckRunEpoch(context.Background(), "run-1", attempt.Epoch); err != nil {
		t.Fatal(err)
	}

	staleSession := *sess
	staleSession.ExecutionEpoch++
	g = New(fakeSource{value: *value, sess: staleSession, rec: *recovery.New()})
	if err := g.CheckRunEpoch(context.Background(), "run-1", attempt.Epoch); !errors.Is(err, ErrRunSessionEpochMismatch) {
		t.Fatalf("expected run/session epoch mismatch, got %v", err)
	}
}

func TestRecoveryAllowsObserveButBlocksMutation(t *testing.T) {
	rec := *recovery.New()
	epoch := rec.Begin()
	g := New(fakeSource{rec: rec})

	if err := g.CheckRecoveryEpoch(context.Background(), epoch, action.Observe); err != nil {
		t.Fatalf("observe should remain available during recovery: %v", err)
	}
	if err := g.CheckRecoveryEpoch(context.Background(), epoch, action.ControlledMutation); !errors.Is(err, recovery.ErrRecoveryMode) {
		t.Fatalf("expected controlled mutation blocked during recovery, got %v", err)
	}
	if err := g.CheckRecoveryEpoch(context.Background(), epoch, action.HighRisk); !errors.Is(err, recovery.ErrRecoveryMode) {
		t.Fatalf("expected high-risk action blocked during recovery, got %v", err)
	}
}

func TestRecoveryRejectsStaleEpochEvenForObserve(t *testing.T) {
	rec := *recovery.New()
	epoch := rec.Begin()
	g := New(fakeSource{rec: rec})
	if err := g.CheckRecoveryEpoch(context.Background(), epoch-1, action.Observe); !errors.Is(err, recovery.ErrStaleEpoch) {
		t.Fatalf("expected stale recovery epoch, got %v", err)
	}
}

func TestRecoveryReadFailureFailsClosed(t *testing.T) {
	expected := errors.New("database unavailable")
	g := New(fakeSource{err: expected})
	if err := g.CheckRecoveryEpoch(context.Background(), 0, action.Observe); !errors.Is(err, expected) {
		t.Fatalf("expected recovery source error to propagate, got %v", err)
	}
}
