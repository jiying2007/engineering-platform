package action

import (
	"errors"
	"time"
)

type State string

const (
	Planned     State = "PLANNED"
	Dispatched  State = "DISPATCHED"
	Confirmed   State = "CONFIRMED"
	Unknown     State = "UNKNOWN"
	Reconciling State = "RECONCILING"
	SafeToRetry State = "SAFE_TO_RETRY"
	Manual      State = "MANUAL"
)

var ErrInvalidTransition = errors.New("invalid external operation transition")

type Operation struct {
	ID             string    `json:"operation_id"`
	RunID          string    `json:"run_id"`
	Action         string    `json:"action"`
	IdempotencyKey string    `json:"idempotency_key"`
	RequestDigest  string    `json:"request_digest,omitempty"`
	State          State     `json:"state"`
	UpdatedAt      time.Time `json:"updated_at"`
	ExternalRef    string    `json:"external_ref,omitempty"`
	ObservedState  string    `json:"observed_state,omitempty"`
}

func New(id, runID, actionName, idempotencyKey string, now time.Time) *Operation {
	return NewWithRequestDigest(id, runID, actionName, idempotencyKey, "", now)
}

func NewWithRequestDigest(id, runID, actionName, idempotencyKey, requestDigest string, now time.Time) *Operation {
	return &Operation{
		ID:             id,
		RunID:          runID,
		Action:         actionName,
		IdempotencyKey: idempotencyKey,
		RequestDigest:  requestDigest,
		State:          Planned,
		UpdatedAt:      now,
	}
}

func (o *Operation) Transition(to State, now time.Time) error {
	valid := false
	switch o.State {
	case Planned:
		valid = to == Dispatched
	case Dispatched:
		valid = to == Confirmed || to == Unknown
	case Unknown:
		valid = to == Reconciling
	case Reconciling:
		valid = to == Confirmed || to == SafeToRetry || to == Manual
	case SafeToRetry:
		valid = to == Dispatched
	}
	if !valid {
		return ErrInvalidTransition
	}
	o.State = to
	o.UpdatedAt = now
	return nil
}
