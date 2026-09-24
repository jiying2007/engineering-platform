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
	ErrLeaseLost       = errors.New("outbox lease lost")
	ErrNoHandler       = errors.New("outbox topic has no handler")
	ErrRiskClass       = errors.New("outbox risk classification is missing or mismatched")
	ErrRecoveryBlocked = errors.New("outbox dispatch blocked by recovery authority")
	// An ambiguous side effect is NOT a retryable delivery failure. The recipient
	// must reconcile its Action ledger before an operator authorizes any replay.
	ErrOutcomeUnknown = errors.New("outbox recipient outcome requires reconciliation")
)

type Message struct {
	ID                 int64            `json:"outbox_id"`
	Key                string           `json:"outbox_key"`
	Topic              string           `json:"topic"`
	AggregateType      string           `json:"aggregate_type,omitempty"`
	AggregateID        string           `json:"aggregate_id,omitempty"`
	Payload            json.RawMessage  `json:"payload"`
	RiskClass          action.RiskClass `json:"risk_class"`
	State              State            `json:"state"`
	AttemptCount       uint64           `json:"attempt_count"`
	LeaseOwner         string           `json:"lease_owner,omitempty"`
	LeaseUntil         time.Time        `json:"lease_until,omitempty"`
	LeaseRecoveryEpoch uint64           `json:"lease_recovery_epoch"`
	NextAttemptAt      time.Time        `json:"next_attempt_at"`
	LastError          string           `json:"last_error,omitempty"`
	CreatedAt          time.Time        `json:"created_at"`
	DispatchedAt       time.Time        `json:"dispatched_at,omitempty"`
}

// Attempt is a monotonic lease generation, NOT just an owner name. It must
// never be reset when the same worker reclaims a message.
type Lease struct {
	ID      int64
	Owner   string
	Attempt uint64
}

func (m Message) Lease() Lease { return Lease{ID: m.ID, Owner: m.LeaseOwner, Attempt: m.AttemptCount} }

func (l Lease) Valid() bool { return l.ID > 0 && l.Owner != "" && l.Attempt > 0 }

type Resolution struct {
	State      State
	RetryAfter time.Duration
	LastError  string
}

func (r Resolution) Validate() error {
	switch r.State {
	case Dispatched, DeadLetter:
		if r.RetryAfter != 0 {
			return fmt.Errorf("terminal resolution cannot carry retry delay")
		}
	case Pending:
		if r.RetryAfter <= 0 || r.RetryAfter > 24*time.Hour {
			return fmt.Errorf("retry delay must be within (0,24h]")
		}
	default:
		return fmt.Errorf("invalid outbox resolution state")
	}
	return nil
}

type Repository interface {
	Claim(context.Context, string, int, time.Duration) ([]Message, error)
	// Dispatch must acquire current recovery authority AND the exact live lease,
	// reload the persisted message, invoke deliver synchronously, and settle it
	// before releasing authority. Checking recovery and then unlocking is unsafe.
	Dispatch(context.Context, Lease, action.RiskClass, func(context.Context, Message) Resolution) (State, error)
	MarkDeadLetter(context.Context, Lease, string) error
}

// Handlers perform bounded, idempotent delivery keyed by Message.Key. They must
// honor cancellation and must NOT mutate recovery/outbox authority or detach
// background work. Irreversible effects belong to a reconciled Action Gateway,
// not a raw outbox callback. Registration is trusted host configuration.
type Handler interface {
	Handle(context.Context, Message) error
}
type HandlerFunc func(context.Context, Message) error

func (f HandlerFunc) Handle(ctx context.Context, message Message) error { return f(ctx, message) }

type Registration struct {
	RiskClass action.RiskClass
	Handler   Handler
}

// Configure once before concurrent use; DispatchBatch does not mutate defaults.
type Dispatcher struct {
	Repository     Repository
	Handlers       map[string]Registration
	LeaseDuration  time.Duration
	HandlerTimeout time.Duration
	RetryDelay     time.Duration
	MaxRetryDelay  time.Duration
	MaxAttempts    uint64
}

type Summary struct {
	Claimed      int `json:"claimed"`
	Dispatched   int `json:"dispatched"`
	Retried      int `json:"retried"`
	DeadLettered int `json:"dead_lettered"`
	Deferred     int `json:"deferred"`
}

func (d *Dispatcher) DispatchBatch(ctx context.Context, workerID string, limit int) (Summary, error) {
	if d == nil || d.Repository == nil {
		return Summary{}, fmt.Errorf("outbox repository is required")
	}
	if workerID == "" || limit <= 0 || limit > 1000 {
		return Summary{}, fmt.Errorf("worker and claim limit in [1,1000] are required")
	}
	lease, timeout, delay, maxDelay, maxAttempts := d.LeaseDuration, d.HandlerTimeout, d.RetryDelay, d.MaxRetryDelay, d.MaxAttempts
	if lease == 0 {
		lease = 30 * time.Second
	}
	if timeout == 0 {
		timeout = lease / 2
	}
	if delay == 0 {
		delay = 5 * time.Second
	}
	if maxDelay == 0 {
		maxDelay = time.Minute
	}
	if maxAttempts == 0 {
		maxAttempts = 5
	}
	if lease < time.Millisecond || lease > 24*time.Hour || timeout <= 0 || timeout >= lease || delay <= 0 || maxDelay < delay || maxDelay > 24*time.Hour {
		return Summary{}, fmt.Errorf("invalid outbox lease, timeout or retry bounds")
	}
	for topic, entry := range d.Handlers {
		if topic == "" || entry.Handler == nil {
			return Summary{}, fmt.Errorf("handler and topic are required")
		}
		if _, err := Classify(topic, entry.RiskClass); err != nil || !ValidRisk(entry.RiskClass) {
			return Summary{}, ErrRiskClass
		}
	}
	if err := ctx.Err(); err != nil {
		return Summary{}, err
	}
	messages, err := d.Repository.Claim(ctx, workerID, limit, lease)
	if err != nil {
		return Summary{}, err
	}
	summary := Summary{Claimed: len(messages)}
	for _, message := range messages {
		if err := ctx.Err(); err != nil {
			return summary, err
		}
		entry, ok := d.Handlers[message.Topic]
		if !ok {
			if err := d.Repository.MarkDeadLetter(ctx, message.Lease(), ErrNoHandler.Error()); err != nil {
				if errors.Is(err, ErrLeaseLost) {
					summary.Deferred++
					continue
				}
				return summary, err
			}
			summary.DeadLettered++
			continue
		}
		dispatchCtx, cancel := context.WithTimeout(ctx, timeout)
		state, err := d.Repository.Dispatch(dispatchCtx, message.Lease(), entry.RiskClass, func(callCtx context.Context, current Message) Resolution {
			// Dispatch reloads identity inside its transaction. Never invoke a handler
			// chosen from stale/caller-modified topic metadata.
			if current.Topic != message.Topic {
				return Resolution{State: DeadLetter, LastError: ErrRiskClass.Error()}
			}
			if current.AttemptCount > maxAttempts {
				return Resolution{State: DeadLetter, LastError: "attempt budget exhausted"}
			}
			handleErr := entry.Handler.Handle(callCtx, current)
			if handleErr == nil {
				return Resolution{State: Dispatched}
			}
			if errors.Is(handleErr, ErrOutcomeUnknown) {
				return Resolution{State: DeadLetter, LastError: ErrOutcomeUnknown.Error()}
			}
			if current.AttemptCount >= maxAttempts {
				return Resolution{State: DeadLetter, LastError: handleErr.Error()}
			}
			return Resolution{State: Pending, RetryAfter: retryDelay(delay, maxDelay, current.AttemptCount), LastError: handleErr.Error()}
		})
		cancel()
		if err != nil {
			if errors.Is(err, ErrOutcomeUnknown) {
				return summary, err
			}
			// Authority denial is NOT a handler failure, never ACK or retry it using a
			// stale generation. Its lease will expire and a fresh claim can re-evaluate.
			if errors.Is(err, ErrLeaseLost) || errors.Is(err, ErrRecoveryBlocked) || errors.Is(err, ErrRiskClass) {
				summary.Deferred++
				continue
			}
			return summary, err
		}
		switch state {
		case Dispatched:
			summary.Dispatched++
		case Pending:
			summary.Retried++
		case DeadLetter:
			summary.DeadLettered++
		default:
			return summary, fmt.Errorf("repository returned invalid settled state %q", state)
		}
	}
	return summary, nil
}

func retryDelay(base, maximum time.Duration, attempt uint64) time.Duration {
	for i := uint64(1); i < attempt && base < maximum; i++ {
		if base > maximum/2 {
			return maximum
		}
		base *= 2
	}
	if base > maximum {
		return maximum
	}
	return base
}
