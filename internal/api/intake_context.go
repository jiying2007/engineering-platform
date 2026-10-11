package api

import (
	"context"

	"github.com/jiying2007/engineering-platform/internal/core"
	"github.com/jiying2007/engineering-platform/internal/verification"
)

// Every operation below is required by the original Core Store contract;
// an HTTP request cannot silently fall back to unscoped ledger methods.

func (s *Server) createWorkForRequest(ctx context.Context, item core.WorkItem) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.store.CreateWorkContext(ctx, item)
}

func (s *Server) getWorkForRequest(ctx context.Context, id string) (core.WorkItem, error) {
	if err := ctx.Err(); err != nil {
		return core.WorkItem{}, err
	}
	return s.store.GetWorkContext(ctx, id)
}

func (s *Server) freezeTaskForRequest(ctx context.Context, task core.TaskContract, plan verification.Plan, version uint64, work core.WorkItem) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.store.CreateTaskAndUpdateWorkContext(ctx, task, plan, version, work)
}

func (s *Server) getTaskForRequest(ctx context.Context, id string) (core.TaskContract, error) {
	if err := ctx.Err(); err != nil {
		return core.TaskContract{}, err
	}
	return s.store.GetTaskContext(ctx, id)
}
