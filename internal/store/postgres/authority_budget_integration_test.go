package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jiying2007/engineering-platform/internal/action"
	"github.com/jiying2007/engineering-platform/internal/core"
	"github.com/jiying2007/engineering-platform/internal/gateway"
)

// The initiating Context deliberately has NO deadline. Hold the only pool
// connection until every waiter has returned: database statement/lock ceilings
// cannot help. Test the actual production fallback, not a test-only timer knob.
// Concurrent probes share the same held connection so this costs one 20s budget,
// not seven sequential waits. The 5s watchdog margin is not a production SLO.
func TestPostgresAuthorityBudgetNoDeadlinePoolWait(t *testing.T) {
	original := newIsolatedIntegrationStore(t)
	config := original.pool.Config()
	config.MaxConns, config.MinConns = 1, 0
	setup, stopSetup := context.WithTimeout(context.Background(), 10*time.Second)
	defer stopSetup()
	pool, err := pgxpool.NewWithConfig(setup, config)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	held, err := pool.Acquire(setup)
	if err != nil {
		t.Fatal(err)
	}
	defer held.Release()
	scoped := New(pool)
	provider := &poolWaitProvider{}
	authority := gateway.NewAuthority(scoped)
	service := action.NewService(authority, authority, provider, scoped)
	cases := []struct {
		name string
		call func(context.Context) error
	}{
		{"work-read", func(ctx context.Context) error { _, e := scoped.GetWorkContext(ctx, "default-wait"); return e }},
		{"work-write", func(ctx context.Context) error {
			return scoped.CreateWorkContext(ctx, core.WorkItem{ID: "default-wait", Title: "no deadline", HumanOwner: "local-test", State: core.WorkDraft, CreatedAt: time.Now().UTC()})
		}},
		{"recovery-read", func(ctx context.Context) error { _, e := scoped.GetRecoveryContext(ctx); return e }},
		{"recovery-begin", func(ctx context.Context) error { _, e := scoped.BeginRecoveryContext(ctx, 0); return e }},
		{"action-read", func(ctx context.Context) error { _, e := scoped.GetContext(ctx, "default-wait"); return e }},
		{"idempotency-read", func(ctx context.Context) error {
			_, e := scoped.GetByIdempotencyKeyContext(ctx, "default-wait")
			return e
		}},
		{"action-admission", func(ctx context.Context) error {
			_, e := service.Execute(ctx, action.Request{ID: "default-wait", RunID: "no-run", ExecutionEpoch: 1, Action: "ci.dispatch", RiskClass: action.ControlledMutation, Capability: "ci.dispatch", IdempotencyKey: "default-wait", ParametersDigest: "sha256:pool-probe", RequestedBy: "local-test"})
			return e
		}},
	}
	caller, stopCaller := context.WithCancel(context.Background())
	defer stopCaller()
	type outcome struct {
		name    string
		err     error
		elapsed time.Duration
	}
	results := make(chan outcome, len(cases))
	for _, tc := range cases {
		go func() {
			start := time.Now()
			err := tc.call(caller)
			results <- outcome{tc.name, err, time.Since(start)}
		}()
	}
	watchdog := time.NewTimer(maxStatementTimeout + 5*time.Second)
	defer watchdog.Stop()
	for remaining := len(cases); remaining > 0; remaining-- {
		select {
		case got := <-results:
			if !errors.Is(got.err, context.DeadlineExceeded) || caller.Err() != nil {
				t.Errorf("unbounded caller has no finite independent Store wait: %s: %v caller=%v", got.name, got.err, caller.Err())
			}
			t.Logf("authority_pool_wait case=%s elapsed_ms=%d fallback_ms=%d caller_deadline=false connection_still_held=true", got.name, got.elapsed.Milliseconds(), maxStatementTimeout.Milliseconds())
		case <-watchdog.C:
			// Release on a broken baseline only after recording the failure, then
			// join every probe. Do not trap pool.Close or leak a diagnostic task.
			t.Errorf("no-deadline authority pool wait exceeded fallback with connection still held (%d outstanding)", remaining)
			stopCaller()
			held.Release()
			for ; remaining > 0; remaining-- {
				select {
				case <-results:
				case <-time.After(5 * time.Second):
					t.Fatal("failed fixture did not stop")
				}
			}
			return
		}
	}
	if pool.Stat().AcquiredConns() != 1 || provider.calls != 0 {
		t.Fatalf("probe lost its held connection or dispatched: acquired=%d provider=%d", pool.Stat().AcquiredConns(), provider.calls)
	}
	verify, stopVerify := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopVerify()
	var work, audit, operations int
	if err := original.pool.QueryRow(verify, "SELECT (SELECT count(*) FROM work_items),(SELECT count(*) FROM audit_events),(SELECT count(*) FROM external_operations)").Scan(&work, &audit, &operations); err != nil {
		t.Fatal(err)
	}
	if work != 0 || audit != 0 || operations != 0 {
		t.Fatalf("expired no-deadline request left authority: %d/%d/%d", work, audit, operations)
	}
	held.Release()
	if err := scoped.CreateWorkContext(verify, core.WorkItem{ID: "after-default-wait", Title: "pool reusable", HumanOwner: "local-test", State: core.WorkDraft, CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatalf("fallback poisoned the pool: %v", err)
	}
}

func TestPostgresAuthorityBudgetCoversWholeMutation(t *testing.T) {
	s := newIsolatedIntegrationStore(t)
	type key struct{}
	parent, cancel := context.WithCancel(context.WithValue(context.Background(), key{}, "exact-caller"))
	defer cancel()
	input, err := auditInput("probe.cancelled", "WorkItem", "budget-probe", struct{ ID string }{"budget-probe"})
	if err != nil {
		t.Fatal(err)
	}
	var observed context.Context
	_, err = s.Mutate(parent, Mutation{Audit: input, Apply: func(ctx context.Context, tx pgx.Tx) error {
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > maxStatementTimeout || ctx.Value(key{}) != "exact-caller" {
			return errors.New("mutation phase lacks its caller-bound whole-operation deadline")
		}
		observed = ctx
		if _, err := tx.Exec(ctx, "SELECT 1"); err != nil {
			return err
		}
		if after, _ := ctx.Deadline(); after != deadline {
			return errors.New("transaction phase reset its deadline")
		}
		cancel()
		return ctx.Err()
	}})
	if observed == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("mutation did not preserve the budget through SQL and rollback: %v", err)
	}
	verify, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()
	var count int
	if err := s.pool.QueryRow(verify, "SELECT count(*) FROM audit_events").Scan(&count); err != nil || count != 0 {
		t.Fatalf("cancelled transaction retained an audit: %d %v", count, err)
	}
}
