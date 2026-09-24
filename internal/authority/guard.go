package authority

import (
	"context"
	"errors"

	"github.com/jiying2007/engineering-platform/internal/action"
	"github.com/jiying2007/engineering-platform/internal/recovery"
	"github.com/jiying2007/engineering-platform/internal/run"
	"github.com/jiying2007/engineering-platform/internal/session"
)

var ErrRunSessionEpochMismatch = errors.New("run/session execution epoch mismatch")

type Source interface {
	GetExecution(string) (run.Run, session.Session, error)
	GetRecovery() (recovery.Manager, error)
}

type Guard struct {
	source Source
}

func New(source Source) *Guard {
	return &Guard{source: source}
}

func (g *Guard) CheckRunEpoch(_ context.Context, runID string, epoch uint64) error {
	value, sess, err := g.source.GetExecution(runID)
	if err != nil {
		return err
	}
	if err := value.CheckEpoch(epoch); err != nil {
		return err
	}
	if sess.ExecutionEpoch != epoch {
		return ErrRunSessionEpochMismatch
	}
	return nil
}

func (g *Guard) CheckRecoveryEpoch(_ context.Context, epoch uint64, risk action.RiskClass) error {
	state, err := g.source.GetRecovery()
	if err != nil {
		return err
	}
	if err := state.CheckEpoch(epoch); err != nil {
		return err
	}
	if risk != action.Observe && state.Mode == recovery.RecoveryReconciliation {
		return recovery.ErrRecoveryMode
	}
	return nil
}
