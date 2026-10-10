package api

import (
	"context"

	"github.com/jiying2007/engineering-platform/internal/core"
	"github.com/jiying2007/engineering-platform/internal/run"
	"github.com/jiying2007/engineering-platform/internal/session"
)

// runControlContextStore reuses the same Core Run/Session/Work authority.
// The existing in-process Store API is retained for controlled internal
// callers; no independent execution owner, policy or state machine is added.
type runControlContextStore interface {
	UpdateExecutionContext(context.Context, string, uint64, run.Run, session.Session) error
	UpdateExecutionAndWorkContext(context.Context, string, uint64, run.Run, session.Session, uint64, core.WorkItem) error
}

func (s *Server) updateRunForRequest(ctx context.Context, id string, version uint64, value run.Run, sess session.Session) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if typed, ok := s.store.(runControlContextStore); ok {
		return typed.UpdateExecutionContext(ctx, id, version, value, sess)
	}
	return s.store.UpdateExecution(id, version, value, sess)
}

func (s *Server) completeRunForRequest(ctx context.Context, id string, runVersion uint64, value run.Run, sess session.Session, workVersion uint64, work core.WorkItem) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if typed, ok := s.store.(runControlContextStore); ok {
		return typed.UpdateExecutionAndWorkContext(ctx, id, runVersion, value, sess, workVersion, work)
	}
	return s.store.UpdateExecutionAndWork(id, runVersion, value, sess, workVersion, work)
}
