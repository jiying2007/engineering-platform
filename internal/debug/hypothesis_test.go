package debug

import (
	"errors"
	"testing"
	"time"
)

func TestConfirmationRequiresEvidence(t *testing.T) {
	r := NewRegistry()
	r.Add(Hypothesis{ID: "h1", Statement: "DMA ownership bug"})
	if err := r.Confirm("h1", nil, time.Unix(1, 0)); !errors.Is(err, ErrEvidenceRequired) {
		t.Fatalf("expected evidence requirement, got %v", err)
	}
	if err := r.Confirm("h1", []string{"evidence-1"}, time.Unix(2, 0)); err != nil {
		t.Fatal(err)
	}
	h, _ := r.Get("h1")
	if h.Status != Confirmed {
		t.Fatalf("expected confirmed, got %s", h.Status)
	}
}
