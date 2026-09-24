package run

import (
	"errors"
	"testing"
	"time"
)

func TestStaleEpochRejectedAfterRestart(t *testing.T) {
	r := New("run-1", "sha256:task")
	a1, err := r.StartAttempt("a1", time.Unix(1, 0))
	if err != nil {
		t.Fatal(err)
	}
	a2, err := r.StartAttempt("a2", time.Unix(2, 0))
	if err != nil {
		t.Fatal(err)
	}
	if a2.Epoch <= a1.Epoch {
		t.Fatal("expected epoch to increase")
	}
	if err := r.Pause(a1.Epoch); !errors.Is(err, ErrStaleEpoch) {
		t.Fatalf("expected stale epoch, got %v", err)
	}
	if err := r.Pause(a2.Epoch); err != nil {
		t.Fatalf("current epoch should mutate run: %v", err)
	}
}

func TestTakeoverRevokesRuntimeEpoch(t *testing.T) {
	r := New("run-1", "sha256:task")
	a, _ := r.StartAttempt("a1", time.Unix(1, 0))
	newEpoch, err := r.Takeover(a.Epoch)
	if err != nil {
		t.Fatal(err)
	}
	if r.ControlOwner != "HUMAN" {
		t.Fatalf("expected human control, got %s", r.ControlOwner)
	}
	if err := r.Resume(a.Epoch); !errors.Is(err, ErrStaleEpoch) {
		t.Fatalf("old runtime epoch should be stale, got %v", err)
	}
	if newEpoch != r.CurrentEpoch {
		t.Fatal("takeover epoch mismatch")
	}
}
