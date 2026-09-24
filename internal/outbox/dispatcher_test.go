package outbox

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/action"
)

type memoryRepo struct {
	mu       sync.Mutex
	messages []Message
}

func (r *memoryRepo) Claim(_ context.Context, workerID string, limit int, lease time.Duration) ([]Message, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Unix(100, 0)
	var out []Message
	for i := range r.messages {
		if len(out) >= limit {
			break
		}
		m := &r.messages[i]
		if m.State != Pending || m.NextAttemptAt.After(now) {
			continue
		}
		m.State = Leased
		m.LeaseOwner = workerID
		m.LeaseUntil = now.Add(lease)
		m.AttemptCount++
		out = append(out, *m)
	}
	return out, nil
}

func (r *memoryRepo) MarkDispatched(_ context.Context, id int64, workerID string) error {
	return r.update(id, workerID, func(m *Message) {
		m.State = Dispatched
	})
}

func (r *memoryRepo) MarkRetry(_ context.Context, id int64, workerID string, next time.Time, lastError string) error {
	return r.update(id, workerID, func(m *Message) {
		m.State = Pending
		m.NextAttemptAt = next
		m.LastError = lastError
		m.LeaseOwner = ""
		m.LeaseUntil = time.Time{}
	})
}

func (r *memoryRepo) MarkDeadLetter(_ context.Context, id int64, workerID, lastError string) error {
	return r.update(id, workerID, func(m *Message) {
		m.State = DeadLetter
		m.LastError = lastError
	})
}

func (r *memoryRepo) update(id int64, workerID string, fn func(*Message)) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.messages {
		if r.messages[i].ID != id {
			continue
		}
		if r.messages[i].State != Leased || r.messages[i].LeaseOwner != workerID {
			return ErrLeaseLost
		}
		fn(&r.messages[i])
		return nil
	}
	return errors.New("not found")
}

func TestDispatcherSuccess(t *testing.T) {
	repo := &memoryRepo{messages: []Message{{
		ID: 1, Key: "run:1", Topic: "run.started", RiskClass: action.Observe,
		State: Pending, NextAttemptAt: time.Unix(0, 0),
	}}}
	calls := 0
	d := Dispatcher{
		Repository: repo,
		Handlers: map[string]Handler{
			"run.started": HandlerFunc(func(context.Context, Message) error {
				calls++
				return nil
			}),
		},
		Now: func() time.Time { return time.Unix(100, 0) },
	}
	summary, err := d.DispatchBatch(context.Background(), "worker-1", 10)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || summary.Dispatched != 1 || repo.messages[0].State != Dispatched {
		t.Fatalf("unexpected dispatch: calls=%d summary=%#v message=%#v", calls, summary, repo.messages[0])
	}
}

func TestDispatcherRetriesThenDeadLetters(t *testing.T) {
	repo := &memoryRepo{messages: []Message{{
		ID: 1, Key: "run:1", Topic: "run.started", RiskClass: action.Observe,
		State: Pending, NextAttemptAt: time.Unix(0, 0), AttemptCount: 1,
	}}}
	d := Dispatcher{
		Repository: repo,
		Handlers: map[string]Handler{
			"run.started": HandlerFunc(func(context.Context, Message) error {
				return errors.New("provider unavailable")
			}),
		},
		MaxAttempts: 2,
		RetryDelay:  10 * time.Second,
		Now:         func() time.Time { return time.Unix(100, 0) },
	}
	summary, err := d.DispatchBatch(context.Background(), "worker-1", 10)
	if err != nil {
		t.Fatal(err)
	}
	if summary.DeadLettered != 1 || repo.messages[0].State != DeadLetter {
		t.Fatalf("expected dead letter, summary=%#v message=%#v", summary, repo.messages[0])
	}
}

func TestMissingHandlerRetries(t *testing.T) {
	repo := &memoryRepo{messages: []Message{{
		ID: 1, Key: "unknown", Topic: "unknown.topic", RiskClass: action.Observe,
		State: Pending, NextAttemptAt: time.Unix(0, 0),
	}}}
	d := Dispatcher{
		Repository: repo,
		Handlers:   map[string]Handler{},
		Now:        func() time.Time { return time.Unix(100, 0) },
	}
	summary, err := d.DispatchBatch(context.Background(), "worker-1", 10)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Retried != 1 || repo.messages[0].State != Pending || repo.messages[0].LastError == "" {
		t.Fatalf("expected retry for missing handler: summary=%#v message=%#v", summary, repo.messages[0])
	}
}
