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

type State interface {
	GetExecution(string) (run.Run, session.Session, error)
	GetTaskByDigest(string) (core.TaskContract, error)
	GetRecovery() (recovery.Manager, error)
}

type Authority struct {
	state State
}

func NewAuthority(state State) *Authority {
	return &Authority{state: state}
}

func (a *Authority) Authorize(_ context.Context, req action.Request) error {
	if a == nil || a.state == nil {
		return fmt.Errorf("gateway authority state is not configured")
	}
	if req.Action == "" || req.Capability == "" {
		return ErrActionNotAllowed
	}
	switch req.RiskClass {
	case action.Observe, action.ControlledMutation, action.HighRisk:
	default:
		return ErrInvalidRiskClass
	}

	value, _, err := a.state.GetExecution(req.RunID)
	if err != nil {
		return err
	}
	task, err := a.state.GetTaskByDigest(value.TaskContractDigest)
	if err != nil {
		return err
	}
	for _, allowed := range task.AllowedActions {
		if allowed == req.Action {
			return nil
		}
	}
	return fmt.Errorf("%w: %s", ErrActionNotAllowed, req.Action)
}

func (a *Authority) CheckRunEpoch(_ context.Context, runID string, epoch uint64) error {
	if a == nil || a.state == nil {
		return fmt.Errorf("gateway authority state is not configured")
	}
	value, _, err := a.state.GetExecution(runID)
	if err != nil {
		return err
	}
	return value.CheckEpoch(epoch)
}

func (a *Authority) CheckRecoveryEpoch(_ context.Context, epoch uint64, risk action.RiskClass) error {
	if a == nil || a.state == nil {
		return fmt.Errorf("gateway authority state is not configured")
	}
	state, err := a.state.GetRecovery()
	if err != nil {
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
