package api

import (
	"context"

	"github.com/jiying2007/engineering-platform/internal/core"
	"github.com/jiying2007/engineering-platform/internal/run"
	"github.com/jiying2007/engineering-platform/internal/session"
)

// Every operation below is required by the original Core Store contract;
// an HTTP request cannot silently fall back to unscoped ledger methods.

func (s *Server) getFrozenTaskForRequest(ctx context.Context, digest string) (core.TaskContract, error) {
	if err := ctx.Err(); err != nil {
		return core.TaskContract{}, err
	}
	return s.store.GetTaskByDigestContext(ctx, digest)
}

func (s *Server) startRunForRequest(ctx context.Context, value run.Run, attempt run.Attempt, sess session.Session, input core.RunInputManifest, version uint64, work core.WorkItem) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.store.CreateExecutionAndUpdateWorkContext(ctx, value, attempt, sess, input, version, work)
}

func (s *Server) getRunForRequest(ctx context.Context, id string) (run.Run, session.Session, error) {
	if err := ctx.Err(); err != nil {
		return run.Run{}, session.Session{}, err
	}
	return s.store.GetExecutionContext(ctx, id)
}

func (s *Server) getRunInputForRequest(ctx context.Context, digest string) (core.RunInputManifest, error) {
	if err := ctx.Err(); err != nil {
		return core.RunInputManifest{}, err
	}
	return s.store.GetRunInputByDigestContext(ctx, digest)
}
