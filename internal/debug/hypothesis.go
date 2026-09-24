package debug

import (
	"errors"
	"time"
)

var (
	ErrHypothesisNotFound = errors.New("hypothesis not found")
	ErrEvidenceRequired   = errors.New("confirmation requires evidence")
)

type Status string

const (
	Proposed     Status = "PROPOSED"
	Supported    Status = "SUPPORTED"
	Rejected     Status = "REJECTED"
	Confirmed    Status = "CONFIRMED"
	Inconclusive Status = "INCONCLUSIVE"
)

type Hypothesis struct {
	ID                    string    `json:"hypothesis_id"`
	Statement             string    `json:"statement"`
	ObservedSupport       []string  `json:"observed_support,omitempty"`
	InferredSupport       []string  `json:"inferred_support,omitempty"`
	ContradictingEvidence []string  `json:"contradicting_evidence,omitempty"`
	ProposedExperiment    string    `json:"proposed_experiment,omitempty"`
	EvidenceRefs          []string  `json:"evidence_refs,omitempty"`
	Status                Status    `json:"status"`
	UpdatedAt             time.Time `json:"updated_at"`
}

type Registry struct {
	items map[string]Hypothesis
}

func NewRegistry() *Registry {
	return &Registry{items: map[string]Hypothesis{}}
}

func (r *Registry) Add(h Hypothesis) {
	if h.Status == "" {
		h.Status = Proposed
	}
	r.items[h.ID] = h
}

func (r *Registry) Confirm(id string, evidenceRefs []string, now time.Time) error {
	h, ok := r.items[id]
	if !ok {
		return ErrHypothesisNotFound
	}
	if len(evidenceRefs) == 0 {
		return ErrEvidenceRequired
	}
	h.EvidenceRefs = append([]string(nil), evidenceRefs...)
	h.Status = Confirmed
	h.UpdatedAt = now
	r.items[id] = h
	return nil
}

func (r *Registry) SetStatus(id string, status Status, now time.Time) error {
	h, ok := r.items[id]
	if !ok {
		return ErrHypothesisNotFound
	}
	if status == Confirmed && len(h.EvidenceRefs) == 0 {
		return ErrEvidenceRequired
	}
	h.Status = status
	h.UpdatedAt = now
	r.items[id] = h
	return nil
}

func (r *Registry) Get(id string) (Hypothesis, bool) {
	h, ok := r.items[id]
	return h, ok
}
