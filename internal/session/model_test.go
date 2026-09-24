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

func TestCheckpointDigestExcludesBusinessIDAndTimestamp(t *testing.T) {
	base := Checkpoint{
		ID:                     "cp-1",
		RunID:                  "run-1",
		TaskContractDigest:     "sha256:task",
		RunInputManifestDigest: "sha256:input",
		ExecutionEpoch:         2,
		SourceTreeDigest:       "sha256:tree",
		DiffDigest:             "sha256:diff",
		Objective:              "finish driver fix",
		Completed:              []string{"analysis"},
		Pending:                []string{"test"},
		Questions:              []string{"which fixture"},
		LastEventSequence:      7,
		ExternalOperationCursor:"op-3",
		CreatedAt:              time.Unix(1, 0),
	}
	other := base
	other.ID = "cp-2"
	other.CreatedAt = time.Unix(999, 0)

	d1, err := base.Digest()
	if err != nil {
		t.Fatal(err)
	}
	d2, err := other.Digest()
	if err != nil {
		t.Fatal(err)
	}
	if d1 != d2 {
		t.Fatalf("checkpoint content digest must ignore business id/time: %s != %s", d1, d2)
	}
}
