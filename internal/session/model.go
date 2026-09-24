package session

import (
	"errors"
	"time"
)

var (
	ErrStaleEpoch       = errors.New("stale session epoch")
	ErrSequence         = errors.New("steering sequence must increase")
	ErrRuntimeNotOwner  = errors.New("runtime no longer owns session writes")
)

type ControlOwner string

const (
	Runtime ControlOwner = "RUNTIME"
	Human   ControlOwner = "HUMAN"
)

type SteeringCommand struct {
	ID            string    `json:"steering_command_id"`
	RunID         string    `json:"run_id"`
	ExecutionEpoch uint64    `json:"execution_epoch"`
	Sequence      uint64    `json:"sequence"`
	Actor         string    `json:"actor"`
	ContentDigest string    `json:"content_digest"`
	CreatedAt     time.Time `json:"created_at"`
}

type Checkpoint struct {
	ID                     string    `json:"checkpoint_id"`
	RunID                  string    `json:"run_id"`
	ExecutionEpoch         uint64    `json:"execution_epoch"`
	SourceTreeDigest       string    `json:"source_tree_digest"`
	DiffDigest             string    `json:"diff_digest,omitempty"`
	Objective               string    `json:"objective,omitempty"`
	Completed               []string  `json:"completed,omitempty"`
	Pending                 []string  `json:"pending,omitempty"`
	Questions               []string  `json:"questions,omitempty"`
	LastEventSequence       uint64    `json:"last_event_sequence"`
	ExternalOperationCursor string    `json:"external_operation_cursor,omitempty"`
	CreatedAt               time.Time `json:"created_at"`
}

type Session struct {
	RunID          string       `json:"run_id"`
	ExecutionEpoch uint64       `json:"execution_epoch"`
	Owner          ControlOwner `json:"control_owner"`
	LastSequence   uint64       `json:"last_steering_sequence"`
	Paused         bool         `json:"paused"`
}

func New(runID string, epoch uint64) *Session {
	return &Session{RunID: runID, ExecutionEpoch: epoch, Owner: Runtime}
}

func (s *Session) ApplySteering(cmd SteeringCommand) error {
	if cmd.ExecutionEpoch != s.ExecutionEpoch {
		return ErrStaleEpoch
	}
	if s.Owner != Runtime {
		return ErrRuntimeNotOwner
	}
	if cmd.Sequence <= s.LastSequence {
		return ErrSequence
	}
	s.LastSequence = cmd.Sequence
	return nil
}

func (s *Session) Pause(epoch uint64) error {
	if epoch != s.ExecutionEpoch {
		return ErrStaleEpoch
	}
	s.Paused = true
	return nil
}

func (s *Session) Resume(epoch uint64) error {
	if epoch != s.ExecutionEpoch {
		return ErrStaleEpoch
	}
	s.Paused = false
	return nil
}

func (s *Session) Takeover(epoch uint64) (uint64, error) {
	if epoch != s.ExecutionEpoch {
		return s.ExecutionEpoch, ErrStaleEpoch
	}
	s.ExecutionEpoch++
	s.Owner = Human
	return s.ExecutionEpoch, nil
}
