package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jiying2007/engineering-platform/internal/action"
)

type State string

const (
	Pending    State = "PENDING"
	Leased     State = "LEASED"
	Dispatched State = "DISPATCHED"
	DeadLetter State = "DEAD_LETTER"
)

var (
	ErrLeaseLost = errors.New("outbox lease lost")
	ErrNoHandler = errors.New("outbox topic has no handler")
)

type Message struct {
	ID            int64            `json:"outbox_id"`
	Key           string           `json:"outbox_key"`
	Topic         string           `json:"topic"`
	AggregateType string           `json:"aggregate_type,omitempty"`
	AggregateID   string           `json:"aggregate_id,omitempty"`
	Payload       json.RawMessage  `json:"payload"`
	RiskClass     action.RiskClass `json:"risk_class"`
	State         State            `json:"state"`
	AttemptCount  uint64           `json:"attempt_count"`
	LeaseOwner    string           `json:"lease_owner,omitempty"`
	LeaseUntil    time.Time        `json:"lease_until,omitempty"`
	NextAttemptAt time.Time        `json:"next_attempt_at"`
	LastError     string           `json:"last_error,omitempty"`
	CreatedAt     time.Time        `json:"created_at"`
	DispatchedAt  time.Time        `json:"dispatched_at,omitempty"`
}

type Repository interface {
	Claim(context.Context, string, int, time.Duration) ([]Message, error)
	MarkDispatched(context.Context, int64, string) error
	MarkRetry(context.Context, int64, string, time.Time, string) error
	MarkDeadLetter(context.Context, int64, string, string) error
}

type Handler interface {
	Handle(context.Context, Message) error
}

type HandlerFunc func(context.Context, Message) error

func (f HandlerFunc) Handle(ctx context.Context, message Message) error {
	return f(ctx, message)
}

type Dispatcher struct {
	Repository    Repository
	Handlers      map[string]Handler
	LeaseDuration time.Duration
	RetryDelay    time.Duration
	MaxAttempts   uint64
	Now           func() time.Time
}

type Summary struct {
	Claimed      int `json:"claimed"`
	Dispatched   int `json:"dispatched"`
	Retried      int `json:"retried"`
	DeadLettered int `json:"dead_lettered"`
}

func (d *Dispatcher) DispatchBatch(ctx context.Context, workerID string, limit int) (Summary, error) {
	if d == nil || d.Repository == nil {
		return Summary{}, fmt.Errorf("outbox repository is required")
	}
	if workerID == "" {
		return Summary{}, fmt.Errorf("worker id is required")
	}
	if limit <= 0 {
		return Summary{}, fmt.Errorf("claim limit must be positive")
	}
	if d.LeaseDuration <= 0 {
		d.LeaseDuration = 30 * time.Second
	}
	if d.RetryDelay <= 0 {
		d.RetryDelay = 5 * time.Second
	}
	if d.MaxAttempts == 0 {
		d.MaxAttempts = 5
	}
	if d.Now == nil {
		d.Now = func() time.Time { return time.Now().UTC() }
	}

	messages, err := d.Repository.Claim(ctx, workerID, limit, d.LeaseDuration)
	if err != nil {
		return Summary{}, err
	}
	summary := Summary{Claimed: len(messages)}
	for _, message := range messages {
		handler := d.Handlers[message.Topic]
		var handleErr error
		if handler == nil {
			handleErr = fmt.Errorf("%w: %s", ErrNoHandler, message.Topic)
		} else {
			handleErr = handler.Handle(ctx, message)
		}

		if handleErr == nil {
			if err := d.Repository.MarkDispatched(ctx, message.ID, workerID); err != nil {
				return summary, err
			}
			summary.Dispatched++
			continue
		}

		if message.AttemptCount >= d.MaxAttempts {
			if err := d.Repository.MarkDeadLetter(ctx, message.ID, workerID, handleErr.Error()); err != nil {
				return summary, err
			}
			summary.DeadLettered++
			continue
		}
		next := d.Now().Add(d.RetryDelay)
		if err := d.Repository.MarkRetry(ctx, message.ID, workerID, next, handleErr.Error()); err != nil {
			return summary, err
		}
		summary.Retried++
	}
	return summary, nil
}
