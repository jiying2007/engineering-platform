package run

import (
	"errors"
	"fmt"
	"time"
)

var (
	ErrStaleEpoch = errors.New("stale execution epoch")
	ErrTerminal    = errors.New("run is terminal")
)

type State string

const (
	Created         State = "CREATED"
	Running         State = "RUNNING"
	Paused          State = "PAUSED"
	HumanControlled State = "HUMAN_CONTROLLED"
	Completed       State = "COMPLETED"
	Aborted         State = "ABORTED"
)

type Attempt struct {
	ID        string    `json:"attempt_id"`
	Epoch     uint64    `json:"execution_epoch"`
	StartedAt time.Time `json:"started_at"`
	EndedAt   time.Time `json:"ended_at,omitempty"`
}

type Run struct {
	ID               string `json:"run_id"`
	TaskContractDigest string `json:"task_contract_digest"`
	State            State  `json:"state"`
	CurrentEpoch     uint64 `json:"current_epoch"`
	CurrentAttemptID string `json:"current_attempt_id,omitempty"`
	ControlOwner     string `json:"control_owner"`
}

func New(id, taskDigest string) *Run {
	return &Run{ID: id, TaskContractDigest: taskDigest, State: Created, ControlOwner: "RUNTIME"}
}

func (r *Run) StartAttempt(id string, now time.Time) (Attempt, error) {
	if r.State == Completed || r.State == Aborted {
		return Attempt{}, ErrTerminal
	}
	r.CurrentEpoch++
	r.CurrentAttemptID = id
	r.State = Running
	r.ControlOwner = "RUNTIME"
	return Attempt{ID: id, Epoch: r.CurrentEpoch, StartedAt: now}, nil
}

func (r *Run) CheckEpoch(epoch uint64) error {
	if epoch != r.CurrentEpoch {
		return fmt.Errorf("%w: got %d want %d", ErrStaleEpoch, epoch, r.CurrentEpoch)
	}
	if r.State == Completed || r.State == Aborted {
		return ErrTerminal
	}
	return nil
}

func (r *Run) Pause(epoch uint64) error {
	if err := r.CheckEpoch(epoch); err != nil {
		return err
	}
	r.State = Paused
	return nil
}

func (r *Run) Resume(epoch uint64) error {
	if err := r.CheckEpoch(epoch); err != nil {
		return err
	}
	r.State = Running
	return nil
}

func (r *Run) Takeover(epoch uint64) (uint64, error) {
	if err := r.CheckEpoch(epoch); err != nil {
		return r.CurrentEpoch, err
	}
	r.CurrentEpoch++
	r.State = HumanControlled
	r.ControlOwner = "HUMAN"
	return r.CurrentEpoch, nil
}

func (r *Run) Abort(epoch uint64) error {
	if err := r.CheckEpoch(epoch); err != nil {
		return err
	}
	r.State = Aborted
	return nil
}
