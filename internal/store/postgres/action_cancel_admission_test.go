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

func (s *cancellationAdmissionStore) Create(op action.Operation) error {
	close(s.entered)
	return s.Store.Create(op)
}

func TestPostgresActionCancellationDuringLockedAdmissionLeavesOnlyPlannedReservation(t *testing.T) {
	s := newIsolatedIntegrationStore(t)
	runID := setupPostgresActionRun(t, s, fmt.Sprintf("cancel-%d", time.Now().UnixNano()))
	ctx, stop := context.WithTimeout(context.Background(), 10*time.Second)
	defer stop()

	// Force Create to block on the actual PostgreSQL platform-state lock.
	// The Action Service currently uses a legacy context-free repository, so
	// the lock may outlive the caller's cancellation until it is released.
	// It must NEVER initiate an external Provider call after that wait.
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
		ID: fmt.Sprintf("cancelled-postgres-action-%d", time.Now().UnixNano()),
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
	// Cancellation is observable before the blocked SQL reservation is
	// permitted to commit, but the synchronous DB call is still in flight.
	cancel()
	if err := blocker.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case result := <-done:
		if !errors.Is(result.err, context.Canceled) ||
			result.receipt.OperationID != "" || provider.dispatchCalls != 0 {
			t.Fatalf("cancelled request reached external effect: %#v err=%v calls=%d",
				result.receipt, result.err, provider.dispatchCalls)
		}
	case <-ctx.Done():
		t.Fatal("PostgreSQL action did not terminate after lock release")
	}
	stored, err := s.Get(request.ID)
	if err != nil || stored.State != action.Planned {
		t.Fatalf("cancelled request did not retain safe pre-dispatch reservation: %#v err=%v", stored, err)
	}
	again, err := service.Execute(context.Background(), request)
	if err != nil || again.Result != string(action.Planned) || provider.dispatchCalls != 0 {
		t.Fatalf("original idempotency request replayed cancelled publication: %#v err=%v calls=%d",
			again, err, provider.dispatchCalls)
	}
}
