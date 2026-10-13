package gateway

import (
	"context"
	"errors"
	"testing"

	"github.com/jiying2007/engineering-platform/internal/action"
	"github.com/jiying2007/engineering-platform/internal/core"
	"github.com/jiying2007/engineering-platform/internal/recovery"
	"github.com/jiying2007/engineering-platform/internal/run"
	"github.com/jiying2007/engineering-platform/internal/session"
	"github.com/jiying2007/engineering-platform/internal/store"
)

// Compile-time acceptance: a Core state that provides only unscoped
// GetExecution/GetTaskByDigest/GetRecovery cannot be injected into Gateway.
var _ State = (*store.Memory)(nil)

type contextAuthorityProbe struct {
	*store.Memory
	executionReads int
	taskReads      int
	recoveryReads  int
}

func (p *contextAuthorityProbe) GetExecutionContext(ctx context.Context, id string) (run.Run, session.Session, error) {
	p.executionReads++
	if err := ctx.Err(); err != nil {
		return run.Run{}, session.Session{}, err
	}
	return p.Memory.GetExecutionContext(context.Background(), id)
}

func (p *contextAuthorityProbe) GetTaskByDigestContext(ctx context.Context, digest string) (core.TaskContract, error) {
	p.taskReads++
	if err := ctx.Err(); err != nil {
		return core.TaskContract{}, err
	}
	return p.Memory.GetTaskByDigestContext(context.Background(), digest)
}

func (p *contextAuthorityProbe) GetRecoveryContext(ctx context.Context) (recovery.Manager, error) {
	p.recoveryReads++
	if err := ctx.Err(); err != nil {
		return recovery.Manager{}, err
	}
	return p.Memory.GetRecoveryContext(context.Background())
}

func TestGatewayUsesCallerContextForAllThreePreDispatchChecks(t *testing.T) {
	_, memory, value := setupAuthority(t, []string{"ci.dispatch"})
	probe := &contextAuthorityProbe{Memory: memory}
	auth := NewAuthority(probe)
	req := action.Request{RunID: value.ID, Action: "ci.dispatch", Capability: "ci.dispatch", RiskClass: action.ControlledMutation}
	if err := auth.Authorize(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if err := auth.CheckRunEpoch(context.Background(), value.ID, value.CurrentEpoch); err != nil {
		t.Fatal(err)
	}
	if err := auth.CheckRecoveryEpoch(context.Background(), 0, action.ControlledMutation); err != nil {
		t.Fatal(err)
	}
	if probe.executionReads != 2 || probe.taskReads != 1 || probe.recoveryReads != 1 {
		t.Fatalf("gateway bypassed original contextual Core: %+v", probe)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, fn := range []func() error{
		func() error { return auth.Authorize(ctx, req) },
		func() error { return auth.CheckRunEpoch(ctx, value.ID, value.CurrentEpoch) },
		func() error { return auth.CheckRecoveryEpoch(ctx, 0, action.ControlledMutation) },
	} {
		if err := fn(); !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled gateway check was not rejected: %v", err)
		}
	}
	if probe.executionReads != 2 || probe.taskReads != 1 || probe.recoveryReads != 1 {
		t.Fatalf("cancelled preflight performed Core reads: %+v", probe)
	}
}
