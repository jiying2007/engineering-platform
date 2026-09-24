package session

import (
	"errors"
	"testing"
)

func TestSteeringIsOrderedAndEpochBound(t *testing.T) {
	s := New("run-1", 1)
	if err := s.ApplySteering(SteeringCommand{ExecutionEpoch: 1, Sequence: 1}); err != nil {
		t.Fatal(err)
	}
	if err := s.ApplySteering(SteeringCommand{ExecutionEpoch: 1, Sequence: 1}); !errors.Is(err, ErrSequence) {
		t.Fatalf("expected duplicate sequence rejection, got %v", err)
	}
	if err := s.ApplySteering(SteeringCommand{ExecutionEpoch: 2, Sequence: 2}); !errors.Is(err, ErrStaleEpoch) {
		t.Fatalf("expected stale epoch rejection, got %v", err)
	}
}

func TestTakeoverRevokesRuntimeSteering(t *testing.T) {
	s := New("run-1", 1)
	newEpoch, err := s.Takeover(1)
	if err != nil {
		t.Fatal(err)
	}
	if s.Owner != Human {
		t.Fatalf("expected human owner, got %s", s.Owner)
	}
	if err := s.ApplySteering(SteeringCommand{ExecutionEpoch: newEpoch, Sequence: 1}); !errors.Is(err, ErrRuntimeNotOwner) {
		t.Fatalf("expected runtime write revocation, got %v", err)
	}
}
