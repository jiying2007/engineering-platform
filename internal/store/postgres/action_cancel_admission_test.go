package postgres

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/action"
	"github.com/jiying2007/engineering-platform/internal/gateway"
)

type cancellationAdmissionStore struct {
	*Store
	entered chan struct{}
}

func (s *cancellationAdmissionStore) CreateContext(ctx context.Context, op action.Operation) error {
	close(s.entered)
	return s.Store.CreateContext(ctx, op)
}

func TestPostgresActionCancellationInterruptsLockedAdmissionWithoutDispatch(t *testing.T) {
	s := newIsolatedIntegrationStore(t)
	runID := setupPostgresActionRun(t, s, fmt.Sprintf("cancel-%d", time.Now().UnixNano()))
	ctx, stop := context.WithTimeout(context.Background(), 10*time.Second)
	defer stop()

	// Force the caller-bound CreateContext to block on the PostgreSQL
	// platform-state lock. Cancellation must interrupt the waiting SQL while
	// the blocker is still held, not merely suppress the Provider after a
	// delayed, context-free reservation commit.
	blocker, err := s.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rollbackOutbox(blocker)
	if _, err = blocker.Exec(ctx,
		"UPDATE platform_state SET recovery_epoch=recovery_epoch WHERE singleton_id=true"); err != nil {
		t.Fatal(err)
	}
	repository := &cancellationAdmissionStore{Store: s, entered: make(chan struct{})}
	provider := &postgresActionProvider{
		dispatch: action.DispatchResult{
			Outcome: action.DispatchConfirmed, ExternalRef: "must-never-publish",
		},
	}
	authority := gateway.NewAuthority(s)
	service := action.NewService(authority, authority, provider, repository)
	request := action.Request{
		ID:    fmt.Sprintf("cancelled-postgres-action-%d", time.Now().UnixNano()),
		RunID: runID, ExecutionEpoch: 1, RecoveryEpoch: 0,
		Action: "ci.dispatch", RiskClass: action.ControlledMutation,
		Capability: "ci.dispatch", IdempotencyKey: "frozen-once",
		ParametersDigest: "sha256:frozen", RequestedBy: "runtime",
	}
	callCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	type outcome struct {
		receipt action.Receipt
		err     error
	}
	done := make(chan outcome, 1)
	go func() {
		receipt, err := service.Execute(callCtx, request)
		done <- outcome{receipt: receipt, err: err}
	}()
	select {
	case <-repository.entered:
	case <-ctx.Done():
		t.Fatal("Action never entered PostgreSQL admission")
	}
	// A cancelled Core action must return while the blocker still owns the
	// lock, proving that the request Context reached the actual PostgreSQL
	// statement rather than waiting for a server-side lock timeout.
	cancel()
	select {
	case result := <-done:
		if !errors.Is(result.err, context.Canceled) ||
			result.receipt.OperationID != "" || provider.dispatchCalls != 0 {
			t.Fatalf("cancelled request reached external effect: %#v err=%v calls=%d",
				result.receipt, result.err, provider.dispatchCalls)
		}
	case <-ctx.Done():
		t.Fatal("PostgreSQL action did not honor caller cancellation")
	}
	if err := blocker.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if stored, err := s.Get(request.ID); !errors.Is(err, action.ErrOperationAbsent) {
		t.Fatalf("cancelled SQL created a false reservation: %#v err=%v", stored, err)
	}
	if stored, err := s.GetByIdempotencyKey(request.IdempotencyKey); !errors.Is(err, action.ErrOperationAbsent) {
		t.Fatalf("cancelled SQL retained an unexpected idempotency key: %#v err=%v", stored, err)
	}
}
