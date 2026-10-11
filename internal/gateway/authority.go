package gateway

import (
	"context"
	"errors"
	"fmt"

	"github.com/jiying2007/engineering-platform/internal/action"
	"github.com/jiying2007/engineering-platform/internal/core"
	"github.com/jiying2007/engineering-platform/internal/recovery"
	"github.com/jiying2007/engineering-platform/internal/run"
	"github.com/jiying2007/engineering-platform/internal/session"
)

var (
	ErrActionNotAllowed = errors.New("action is not allowed by frozen task contract")
	ErrInvalidRiskClass = errors.New("invalid action risk class")
)

// State is the same Core authority, with mandatory caller-bound methods
// for all three pre-dispatch reads. A context-free fallback could outlive
// an authenticated HTTP cancellation before an Action reservation.
type State interface {
	GetExecutionContext(context.Context, string) (run.Run, session.Session, error)
	GetTaskByDigestContext(context.Context, string) (core.TaskContract, error)
	GetRecoveryContext(context.Context) (recovery.Manager, error)
}

type Authority struct {
	state State
}

func NewAuthority(state State) *Authority {
	return &Authority{state: state}
}

func (a *Authority) execution(ctx context.Context, id string) (run.Run, session.Session, error) {
	if err := ctx.Err(); err != nil {
		return run.Run{}, session.Session{}, err
	}
	return a.state.GetExecutionContext(ctx, id)
}

func (a *Authority) frozenTask(ctx context.Context, digest string) (core.TaskContract, error) {
	if err := ctx.Err(); err != nil {
		return core.TaskContract{}, err
	}
	return a.state.GetTaskByDigestContext(ctx, digest)
}

func (a *Authority) recovery(ctx context.Context) (recovery.Manager, error) {
	if err := ctx.Err(); err != nil {
		return recovery.Manager{}, err
	}
	return a.state.GetRecoveryContext(ctx)
}

func (a *Authority) Authorize(ctx context.Context, req action.Request) error {
	if a == nil || a.state == nil {
		return fmt.Errorf("gateway authority state is not configured")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if req.Action == "" || req.Capability == "" {
		return ErrActionNotAllowed
	}
	switch req.RiskClass {
	case action.Observe, action.ControlledMutation, action.HighRisk:
	default:
		return ErrInvalidRiskClass
	}
	value, _, err := a.execution(ctx, req.RunID)
	if err != nil {
		return err
	}
	task, err := a.frozenTask(ctx, value.TaskContractDigest)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	for _, allowed := range task.AllowedActions {
		if allowed == req.Action {
			return nil
		}
	}
	return fmt.Errorf("%w: %s", ErrActionNotAllowed, req.Action)
}

func (a *Authority) CheckRunEpoch(ctx context.Context, runID string, epoch uint64) error {
	if a == nil || a.state == nil {
		return fmt.Errorf("gateway authority state is not configured")
	}
	value, _, err := a.execution(ctx, runID)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return value.CheckEpoch(epoch)
}

func (a *Authority) CheckRecoveryEpoch(ctx context.Context, epoch uint64, risk action.RiskClass) error {
	if a == nil || a.state == nil {
		return fmt.Errorf("gateway authority state is not configured")
	}
	state, err := a.recovery(ctx)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := state.CheckEpoch(epoch); err != nil {
		return err
	}
	switch risk {
	case action.Observe:
		return nil
	case action.ControlledMutation, action.HighRisk:
		return state.AuthorizeIrreversible(epoch)
	default:
		return ErrInvalidRiskClass
	}
}
