package api

import (
	"context"

	"github.com/jiying2007/engineering-platform/internal/core"
	"github.com/jiying2007/engineering-platform/internal/run"
	"github.com/jiying2007/engineering-platform/internal/session"
)

// Every operation below is required by the original Core Store contract;
// an HTTP request cannot silently fall back to unscoped ledger methods.

func (s *Server) updateRunForRequest(ctx context.Context, id string, version uint64, value run.Run, sess session.Session) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.store.UpdateExecutionContext(ctx, id, version, value, sess)
}

func (s *Server) completeRunForRequest(ctx context.Context, id string, runVersion uint64, value run.Run, sess session.Session, workVersion uint64, work core.WorkItem) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.store.UpdateExecutionAndWorkContext(ctx, id, runVersion, value, sess, workVersion, work)
}
