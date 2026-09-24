package audit

import (
	"fmt"
	"time"

	"github.com/jiying2007/engineering-platform/internal/canonical"
)

type Event struct {
	Sequence       uint64    `json:"sequence"`
	Type           string    `json:"type"`
	AggregateType  string    `json:"aggregate_type,omitempty"`
	AggregateID    string    `json:"aggregate_id,omitempty"`
	PayloadDigest  string    `json:"payload_digest"`
	CorrelationID  string    `json:"correlation_id,omitempty"`
	CausationID    string    `json:"causation_id,omitempty"`
	PreviousDigest string    `json:"previous_digest,omitempty"`
	Digest         string    `json:"digest"`
	CreatedAt      time.Time `json:"created_at"`
}

type eventContent struct {
	Sequence       uint64 `json:"sequence"`
	Type           string `json:"type"`
	AggregateType  string `json:"aggregate_type,omitempty"`
	AggregateID    string `json:"aggregate_id,omitempty"`
	PayloadDigest  string `json:"payload_digest"`
	CorrelationID  string `json:"correlation_id,omitempty"`
	CausationID    string `json:"causation_id,omitempty"`
	PreviousDigest string `json:"previous_digest,omitempty"`
}

type Input struct {
	Type          string
	AggregateType string
	AggregateID   string
	PayloadDigest string
	CorrelationID string
	CausationID   string
}

func Build(sequence uint64, previousDigest string, input Input, now time.Time) (Event, error) {
	if sequence == 0 {
		return Event{}, fmt.Errorf("audit sequence must be greater than zero")
	}
	if input.Type == "" {
		return Event{}, fmt.Errorf("audit event type is required")
	}
	if input.PayloadDigest == "" {
		return Event{}, fmt.Errorf("audit payload digest is required")
	}
	content := eventContent{
		Sequence:       sequence,
		Type:           input.Type,
		AggregateType:  input.AggregateType,
		AggregateID:    input.AggregateID,
		PayloadDigest:  input.PayloadDigest,
		CorrelationID:  input.CorrelationID,
		CausationID:    input.CausationID,
		PreviousDigest: previousDigest,
	}
	digest, err := canonical.Digest(content)
	if err != nil {
		return Event{}, err
	}
	return Event{
		Sequence:       content.Sequence,
		Type:           content.Type,
		AggregateType:  content.AggregateType,
		AggregateID:    content.AggregateID,
		PayloadDigest:  content.PayloadDigest,
		CorrelationID:  content.CorrelationID,
		CausationID:    content.CausationID,
		PreviousDigest: content.PreviousDigest,
		Digest:         digest,
		CreatedAt:      now,
	}, nil
}

type Journal struct {
	events []Event
}

func (j *Journal) Append(eventType, payloadDigest string, now time.Time) (Event, error) {
	return j.AppendInput(Input{
		Type:          eventType,
		PayloadDigest: payloadDigest,
	}, now)
}

func (j *Journal) AppendInput(input Input, now time.Time) (Event, error) {
	var prev string
	if len(j.events) > 0 {
		prev = j.events[len(j.events)-1].Digest
	}
	event, err := Build(uint64(len(j.events)+1), prev, input, now)
	if err != nil {
		return Event{}, err
	}
	j.events = append(j.events, event)
	return event, nil
}

func (j *Journal) Events() []Event {
	out := make([]Event, len(j.events))
	copy(out, j.events)
	return out
}

func Verify(events []Event) error {
	var prev string
	for i, event := range events {
		expectedSequence := uint64(i + 1)
		if event.Sequence != expectedSequence {
			return fmt.Errorf("audit sequence mismatch: got %d want %d", event.Sequence, expectedSequence)
		}
		if event.PreviousDigest != prev {
			return fmt.Errorf("audit previous digest mismatch at sequence %d", event.Sequence)
		}
		rebuilt, err := Build(event.Sequence, event.PreviousDigest, Input{
			Type:          event.Type,
			AggregateType: event.AggregateType,
			AggregateID:   event.AggregateID,
			PayloadDigest: event.PayloadDigest,
			CorrelationID: event.CorrelationID,
			CausationID:   event.CausationID,
		}, event.CreatedAt)
		if err != nil {
			return err
		}
		if rebuilt.Digest != event.Digest {
			return fmt.Errorf("audit digest mismatch at sequence %d", event.Sequence)
		}
		prev = event.Digest
	}
	return nil
}

type Checkpoint struct {
	FirstSequence uint64    `json:"first_sequence"`
	LastSequence  uint64    `json:"last_sequence"`
	EventCount    uint64    `json:"event_count"`
	RootDigest    string    `json:"root_digest"`
	CreatedAt     time.Time `json:"created_at"`
}

func NewCheckpoint(events []Event, now time.Time) (Checkpoint, error) {
	if len(events) == 0 {
		return Checkpoint{}, fmt.Errorf("cannot checkpoint empty audit stream")
	}
	if err := Verify(events); err != nil {
		return Checkpoint{}, err
	}
	return Checkpoint{
		FirstSequence: events[0].Sequence,
		LastSequence:  events[len(events)-1].Sequence,
		EventCount:    uint64(len(events)),
		RootDigest:    events[len(events)-1].Digest,
		CreatedAt:     now,
	}, nil
}
