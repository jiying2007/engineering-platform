package core

import (
	"errors"
	"testing"
)

func TestWorkStateMachine(t *testing.T) {
	w := WorkItem{ID: "work-1", State: WorkDraft}
	for _, state := range []WorkState{WorkReady, WorkExecuting, WorkVerifying, WorkClosed} {
		if err := w.Transition(state); err != nil {
			t.Fatalf("transition to %s failed: %v", state, err)
		}
	}
	if err := w.Transition(WorkExecuting); !errors.Is(err, ErrInvalidWorkTransition) {
		t.Fatalf("closed work should not reopen, got %v", err)
	}
}
