package audit

import (
	"fmt"
	"time"

	"github.com/jiying2007/engineering-platform/internal/canonical"
)

type Event struct {
	Sequence       uint64    `json:"sequence"`
	Type           string    `json:"type"`
	PayloadDigest  string    `json:"payload_digest"`
	PreviousDigest string    `json:"previous_digest,omitempty"`
	Digest         string    `json:"digest"`
	CreatedAt      time.Time `json:"created_at"`
}

type unsignedEvent struct {
	Sequence       uint64 `json:"sequence"`
	Type           string `json:"type"`
	PayloadDigest  string `json:"payload_digest"`
	PreviousDigest string `json:"previous_digest,omitempty"`
}

type Journal struct {
	events []Event
}

func (j *Journal) Append(eventType, payloadDigest string, now time.Time) (Event, error) {
	var prev string
	if len(j.events) > 0 {
		prev = j.events[len(j.events)-1].Digest
	}
	u := unsignedEvent{
		Sequence:       uint64(len(j.events) + 1),
		Type:           eventType,
		PayloadDigest:  payloadDigest,
		PreviousDigest: prev,
	}
	digest, err := canonical.Digest(u)
	if err != nil {
		return Event{}, err
	}
	event := Event{
		Sequence:       u.Sequence,
		Type:           u.Type,
		PayloadDigest:  u.PayloadDigest,
		PreviousDigest: u.PreviousDigest,
		Digest:         digest,
		CreatedAt:      now,
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
		u := unsignedEvent{
			Sequence:       event.Sequence,
			Type:           event.Type,
			PayloadDigest:  event.PayloadDigest,
			PreviousDigest: event.PreviousDigest,
		}
		digest, err := canonical.Digest(u)
		if err != nil {
			return err
		}
		if digest != event.Digest {
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
