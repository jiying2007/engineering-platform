package action

import (
	"errors"
	"testing"
	"time"
)

func TestUnknownMustReconcileBeforeRetry(t *testing.T) {
	now := time.Unix(1, 0)
	o := New("op-1", "run-1", "ci.dispatch", "idem-1", now)
	if err := o.Transition(Dispatched, now); err != nil {
		t.Fatal(err)
	}
	if err := o.Transition(Unknown, now); err != nil {
		t.Fatal(err)
	}
	if err := o.Transition(Dispatched, now); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("blind retry should be rejected, got %v", err)
	}
	if err := o.Transition(Reconciling, now); err != nil {
		t.Fatal(err)
	}
	if err := o.Transition(SafeToRetry, now); err != nil {
		t.Fatal(err)
	}
	if err := o.Transition(Dispatched, now); err != nil {
		t.Fatalf("safe retry should dispatch: %v", err)
	}
}
