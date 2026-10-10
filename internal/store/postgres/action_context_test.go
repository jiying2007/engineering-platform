package postgres

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/action"
	"github.com/jiying2007/engineering-platform/internal/gateway"
)

type postgresCancelAfterEffectProvider struct {
	cancel context.CancelFunc
	calls  int
}

func (p *postgresCancelAfterEffectProvider) Dispatch(context.Context, action.Request) (action.DispatchResult, error) {
	p.calls++
	p.cancel()
	return action.DispatchResult{
		Outcome: action.DispatchConfirmed, ExternalRef: "exact-completed-external-effect",
		ObservedState: "remote-success",
	}, nil
}
func (p *postgresCancelAfterEffectProvider) Reconcile(context.Context, action.Operation) (action.ReconcileResult, error) {
	return action.ReconcileResult{}, fmt.Errorf("unexpected Reconcile call")
}

func TestPostgresActionIndependentBoundedSettlementAfterCallerCancellation(t *testing.T) {
	s := newIsolatedIntegrationStore(t)
	suffix := fmt.Sprintf("settle-cancel-%d", time.Now().UnixNano())
	runID := setupPostgresActionRun(t, s, suffix)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	provider := &postgresCancelAfterEffectProvider{cancel: cancel}
	authority := gateway.NewAuthority(s)
	service := action.NewService(authority, authority, provider, s)
	request := action.Request{
		ID: "action-" + suffix,
		RunID: runID, ExecutionEpoch: 1, RecoveryEpoch: 0,
		Action: "ci.dispatch", RiskClass: action.ControlledMutation,
		Capability: "ci.dispatch", IdempotencyKey: "once-" + suffix,
		ParametersDigest: "sha256:exact-request", RequestedBy: "runtime",
	}
	receipt, err := service.Execute(ctx, request)
	if err != nil || receipt.Result != string(action.Confirmed) ||
		receipt.ExternalRef != "exact-completed-external-effect" ||
		provider.calls != 1 {
		t.Fatalf("cancelled caller erased real Provider completion: receipt=%#v err=%v calls=%d",
			receipt, err, provider.calls)
	}
	stored, err := s.Get(request.ID)
	if err != nil || stored.State != action.Confirmed ||
		stored.ExternalRef != receipt.ExternalRef {
		t.Fatalf("post-effect settlement was not durable: %#v err=%v", stored, err)
	}
	var auditCount int
	if err := s.pool.QueryRow(context.Background(),
		"SELECT count(*) FROM audit_events WHERE aggregate_type='ExternalOperation' AND aggregate_id=$1",
		request.ID,
	).Scan(&auditCount); err != nil || auditCount != 3 {
		t.Fatalf("Action settlement must retain PLANNED/DISPATCHED/CONFIRMED audit: count=%d err=%v",
			auditCount, err)
	}
}
