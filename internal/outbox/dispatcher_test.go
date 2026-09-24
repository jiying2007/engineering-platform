package outbox

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/action"
)

// This fake tests dispatcher policy, not PostgreSQL lock semantics. Integration
// tests separately exercise row locks, wall-clock expiry and schema upgrades.
type memoryRepo struct {
	mu             sync.Mutex
	messages       []Message
	recovering     bool
	epoch          uint64
	beforeDispatch func()
}

func (r *memoryRepo) Claim(ctx context.Context, worker string, limit int, duration time.Duration) ([]Message, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	now := time.Now()
	var result []Message
	for i := range r.messages {
		m := &r.messages[i]
		if len(result) == limit {
			break
		}
		if (m.State != Pending && !(m.State == Leased && !m.LeaseUntil.After(now))) || m.NextAttemptAt.After(now) {
			continue
		}
		if r.recovering && m.RiskClass != action.Observe {
			continue
		}
		m.State = Leased
		m.LeaseOwner = worker
		m.LeaseUntil = now.Add(duration)
		m.LeaseRecoveryEpoch = r.epoch
		m.AttemptCount++
		result = append(result, *m)
	}
	return result, nil
}
func (r *memoryRepo) Dispatch(ctx context.Context, lease Lease, risk action.RiskClass, deliver func(context.Context, Message) Resolution) (State, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.beforeDispatch != nil {
		r.beforeDispatch()
	}
	for i := range r.messages {
		m := &r.messages[i]
		if m.ID != lease.ID {
			continue
		}
		if m.State != Leased || m.Lease() != lease || !m.LeaseUntil.After(time.Now()) {
			return "", ErrLeaseLost
		}
		if risk != m.RiskClass {
			return "", ErrRiskClass
		}
		if risk != action.Observe && (r.recovering || r.epoch != m.LeaseRecoveryEpoch) {
			return "", ErrRecoveryBlocked
		}
		resolution := deliver(ctx, *m)
		if err := ctx.Err(); err != nil {
			return "", fmt.Errorf("%w: %w", ErrOutcomeUnknown, err)
		}
		if !m.LeaseUntil.After(time.Now()) {
			return "", ErrLeaseLost
		}
		if err := resolution.Validate(); err != nil {
			return "", err
		}
		m.State = resolution.State
		m.LastError = resolution.LastError
		m.NextAttemptAt = time.Now().Add(resolution.RetryAfter)
		m.LeaseOwner = ""
		m.LeaseUntil = time.Time{}
		return m.State, nil
	}
	return "", ErrLeaseLost
}
func (r *memoryRepo) MarkDeadLetter(ctx context.Context, lease Lease, lastError string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	for i := range r.messages {
		m := &r.messages[i]
		if m.ID == lease.ID && m.State == Leased && m.Lease() == lease && m.LeaseUntil.After(time.Now()) {
			m.State = DeadLetter
			m.LastError = lastError
			m.LeaseOwner = ""
			m.LeaseUntil = time.Time{}
			return nil
		}
	}
	return ErrLeaseLost
}
func pending(id int64, risk action.RiskClass) Message {
	return Message{ID: id, Key: fmt.Sprintf("message-%d", id), Topic: "test.delivery", RiskClass: risk, State: Pending}
}
func dispatcher(r *memoryRepo, risk action.RiskClass, handler HandlerFunc) Dispatcher {
	return Dispatcher{Repository: r, Handlers: map[string]Registration{"test.delivery": {RiskClass: risk, Handler: handler}}}
}

func TestDispatcherSuccess(t *testing.T) {
	repo := &memoryRepo{messages: []Message{pending(1, action.Observe)}}
	calls := 0
	d := dispatcher(repo, action.Observe, func(_ context.Context, m Message) error {
		calls++
		if m.Key != "message-1" {
			t.Fatal("unstable idempotency key")
		}
		return nil
	})
	result, err := d.DispatchBatch(context.Background(), "worker", 10)
	if err != nil || calls != 1 || result.Dispatched != 1 || repo.messages[0].State != Dispatched {
		t.Fatalf("calls=%d result=%+v err=%v", calls, result, err)
	}
}
func TestDispatcherRetriesThenDeadLetters(t *testing.T) {
	repo := &memoryRepo{messages: []Message{pending(1, action.Observe)}}
	d := dispatcher(repo, action.Observe, func(context.Context, Message) error { return errors.New("unavailable") })
	d.MaxAttempts = 2
	d.RetryDelay = time.Millisecond
	first, err := d.DispatchBatch(context.Background(), "worker", 10)
	if err != nil || first.Retried != 1 || repo.messages[0].LastError == "" {
		t.Fatalf("%+v %v", first, err)
	}
	repo.messages[0].NextAttemptAt = time.Time{}
	second, err := d.DispatchBatch(context.Background(), "worker", 10)
	if err != nil || second.DeadLettered != 1 || repo.messages[0].AttemptCount != 2 {
		t.Fatalf("%+v %v", second, err)
	}
}
func TestMissingHandlerIsQuarantined(t *testing.T) {
	repo := &memoryRepo{messages: []Message{pending(1, action.Observe)}}
	d := Dispatcher{Repository: repo}
	result, err := d.DispatchBatch(context.Background(), "worker", 10)
	if err != nil || result.DeadLettered != 1 || repo.messages[0].LastError != ErrNoHandler.Error() {
		t.Fatalf("%+v %v", result, err)
	}
}
func TestRecoveryChangesAfterClaimBlockMutationButAllowObserve(t *testing.T) {
	for _, risk := range []action.RiskClass{action.Observe, action.ControlledMutation, action.HighRisk} {
		t.Run(string(risk), func(t *testing.T) {
			repo := &memoryRepo{messages: []Message{pending(1, risk)}}
			repo.beforeDispatch = func() { repo.recovering = true; repo.epoch++ }
			calls := 0
			d := dispatcher(repo, risk, func(context.Context, Message) error { calls++; return nil })
			result, err := d.DispatchBatch(context.Background(), "worker", 10)
			if err != nil {
				t.Fatal(err)
			}
			if risk == action.Observe {
				if calls != 1 || result.Dispatched != 1 {
					t.Fatalf("%+v calls=%d", result, calls)
				}
			} else if calls != 0 || result.Deferred != 1 || repo.messages[0].State != Leased {
				t.Fatalf("%+v calls=%d", result, calls)
			}
		})
	}
}
func TestOldEpochCannotResumeEvenAfterRecoveryCompletes(t *testing.T) {
	repo := &memoryRepo{messages: []Message{pending(1, action.ControlledMutation)}}
	repo.beforeDispatch = func() { repo.epoch++ }
	d := dispatcher(repo, action.ControlledMutation, func(context.Context, Message) error { t.Fatal("old epoch dispatched"); return nil })
	result, err := d.DispatchBatch(context.Background(), "worker", 10)
	if err != nil || result.Deferred != 1 {
		t.Fatalf("%+v %v", result, err)
	}
}
func TestMislabeledMessageCannotInvokeRegisteredHandler(t *testing.T) {
	repo := &memoryRepo{messages: []Message{pending(1, action.Observe)}}
	d := dispatcher(repo, action.HighRisk, func(context.Context, Message) error { t.Fatal("risk downgrade dispatched"); return nil })
	result, err := d.DispatchBatch(context.Background(), "worker", 10)
	if err != nil || result.Deferred != 1 {
		t.Fatalf("%+v %v", result, err)
	}
}
func TestUnknownOutcomeNeverRetries(t *testing.T) {
	repo := &memoryRepo{messages: []Message{pending(1, action.HighRisk)}}
	d := dispatcher(repo, action.HighRisk, func(context.Context, Message) error { return fmt.Errorf("provider: %w", ErrOutcomeUnknown) })
	result, err := d.DispatchBatch(context.Background(), "worker", 10)
	if err != nil || result.DeadLettered != 1 || result.Retried != 0 || repo.messages[0].LastError != ErrOutcomeUnknown.Error() {
		t.Fatalf("%+v %v", result, err)
	}
}
func TestExpiredLeaseBeforeHandlerIsDeferred(t *testing.T) {
	repo := &memoryRepo{messages: []Message{pending(1, action.Observe)}}
	repo.beforeDispatch = func() { repo.messages[0].LeaseUntil = time.Now().Add(-time.Second) }
	d := dispatcher(repo, action.Observe, func(context.Context, Message) error { t.Fatal("expired owner executed"); return nil })
	result, err := d.DispatchBatch(context.Background(), "worker", 10)
	if err != nil || result.Deferred != 1 {
		t.Fatalf("%+v %v", result, err)
	}
}
func TestCancelledHandlerCannotBeAcknowledged(t *testing.T) {
	repo := &memoryRepo{messages: []Message{pending(1, action.Observe)}}
	d := dispatcher(repo, action.Observe, func(ctx context.Context, _ Message) error { <-ctx.Done(); return nil })
	d.HandlerTimeout = time.Millisecond
	result, err := d.DispatchBatch(context.Background(), "worker", 10)
	if !errors.Is(err, context.DeadlineExceeded) || !errors.Is(err, ErrOutcomeUnknown) || result.Dispatched != 0 || repo.messages[0].State != Leased {
		t.Fatalf("%+v %v", result, err)
	}
}
func TestConcurrentBatchesDoNotMutateDispatcherDefaults(t *testing.T) {
	repo := &memoryRepo{}
	for i := int64(1); i <= 64; i++ {
		repo.messages = append(repo.messages, pending(i, action.Observe))
	}
	var calls atomic.Int64
	d := dispatcher(repo, action.Observe, func(context.Context, Message) error { calls.Add(1); return nil })
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if _, err := d.DispatchBatch(context.Background(), fmt.Sprintf("worker-%d", i), 8); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	if calls.Load() != 64 || d.LeaseDuration != 0 || d.HandlerTimeout != 0 || d.RetryDelay != 0 || d.MaxAttempts != 0 {
		t.Fatalf("calls=%d dispatcher mutated: %+v", calls.Load(), d)
	}
}
func TestAttemptBudgetSurvivesLeaseReclaims(t *testing.T) {
	m := pending(1, action.Observe)
	m.AttemptCount = 10
	repo := &memoryRepo{messages: []Message{m}}
	d := dispatcher(repo, action.Observe, func(context.Context, Message) error { t.Fatal("exhausted budget invoked"); return nil })
	result, err := d.DispatchBatch(context.Background(), "worker", 1)
	if err != nil || result.DeadLettered != 1 {
		t.Fatalf("%+v %v", result, err)
	}
}
func TestRiskClassificationFailsClosed(t *testing.T) {
	for _, risk := range []action.RiskClass{"", "UNKNOWN"} {
		if _, err := Classify("new.topic", risk); !errors.Is(err, ErrRiskClass) {
			t.Fatalf("unclassified topic accepted: %v", err)
		}
	}
	if got, err := Classify("run.started", ""); err != nil || got != action.ControlledMutation {
		t.Fatalf("%s %v", got, err)
	}
	if _, err := Classify("run.started", action.Observe); !errors.Is(err, ErrRiskClass) {
		t.Fatal("core downgrade accepted")
	}
	repo := &memoryRepo{messages: []Message{pending(1, action.Observe)}}
	d := dispatcher(repo, "", func(context.Context, Message) error { return nil })
	if _, err := d.DispatchBatch(context.Background(), "worker", 1); !errors.Is(err, ErrRiskClass) || repo.messages[0].AttemptCount != 0 {
		t.Fatal("unclassified handler claimed work")
	}
}
func TestBackoffAndResolutionBounds(t *testing.T) {
	for _, tc := range []struct {
		attempt uint64
		want    time.Duration
	}{{1, time.Second}, {2, 2 * time.Second}, {3, 4 * time.Second}, {100000, 5 * time.Second}} {
		if got := retryDelay(time.Second, 5*time.Second, tc.attempt); got != tc.want {
			t.Fatalf("attempt=%d got=%s", tc.attempt, got)
		}
	}
	for _, r := range []Resolution{{State: Leased}, {State: Pending}, {State: Pending, RetryAfter: 25 * time.Hour}, {State: Dispatched, RetryAfter: time.Second}} {
		if r.Validate() == nil {
			t.Fatalf("invalid resolution accepted: %+v", r)
		}
	}
	if (Lease{ID: 1, Owner: "worker"}).Valid() {
		t.Fatal("missing lease generation accepted")
	}
}
