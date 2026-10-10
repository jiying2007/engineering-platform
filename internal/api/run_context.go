package api

import (
	"context"

	"github.com/jiying2007/engineering-platform/internal/core"
	"github.com/jiying2007/engineering-platform/internal/run"
	"github.com/jiying2007/engineering-platform/internal/session"
)

// runContextStore provides caller-bound access to the same Core ledger.
// It is an optional transport capability on the existing store, not another
// Run/Worker/Outbox authority or a new execution implementation.
type runContextStore interface {
	GetTaskByDigestContext(context.Context, string) (core.TaskContract, error)
	CreateExecutionAndUpdateWorkContext(context.Context, run.Run, run.Attempt, session.Session, core.RunInputManifest, uint64, core.WorkItem) error
	GetExecutionContext(context.Context, string) (run.Run, session.Session, error)
	GetRunInputByDigestContext(context.Context, string) (core.RunInputManifest, error)
}

func (s *Server) getFrozenTaskForRequest(ctx context.Context, digest string) (core.TaskContract, error) {
	if err := ctx.Err(); err != nil {
		return core.TaskContract{}, err
	}
	if typed, ok := s.store.(runContextStore); ok {
		return typed.GetTaskByDigestContext(ctx, digest)
	}
	return s.store.GetTaskByDigest(digest)
}

func (s *Server) startRunForRequest(ctx context.Context, value run.Run, attempt run.Attempt, sess session.Session, input core.RunInputManifest, version uint64, work core.WorkItem) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if typed, ok := s.store.(runContextStore); ok {
		return typed.CreateExecutionAndUpdateWorkContext(ctx, value, attempt, sess, input, version, work)
	}
	return s.store.CreateExecutionAndUpdateWork(value, attempt, sess, input, version, work)
}

func (s *Server) getRunForRequest(ctx context.Context, id string) (run.Run, session.Session, error) {
	if err := ctx.Err(); err != nil {
		return run.Run{}, session.Session{}, err
	}
	if typed, ok := s.store.(runContextStore); ok {
		return typed.GetExecutionContext(ctx, id)
	}
	return s.store.GetExecution(id)
}

func (s *Server) getRunInputForRequest(ctx context.Context, digest string) (core.RunInputManifest, error) {
	if err := ctx.Err(); err != nil {
		return core.RunInputManifest{}, err
	}
	if typed, ok := s.store.(runContextStore); ok {
		return typed.GetRunInputByDigestContext(ctx, digest)
	}
	return s.store.GetRunInputByDigest(digest)
}
