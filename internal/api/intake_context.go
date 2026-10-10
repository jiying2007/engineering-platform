package api

import (
	"context"

	"github.com/jiying2007/engineering-platform/internal/core"
	"github.com/jiying2007/engineering-platform/internal/verification"
)

// coreIntakeContextStore is a transport capability on the existing Core Store,
// not a second source of Work/Task authority. PostgreSQL provides request-bound
// methods. In-memory test stores retain the historical Store interface.
type coreIntakeContextStore interface {
	CreateWorkContext(context.Context, core.WorkItem) error
	GetWorkContext(context.Context, string) (core.WorkItem, error)
	UpdateWorkContext(context.Context, string, uint64, core.WorkItem) error
	CreateTaskAndUpdateWorkContext(context.Context, core.TaskContract, verification.Plan, uint64, core.WorkItem) error
	GetTaskContext(context.Context, string) (core.TaskContract, error)
}

func (s *Server) createWorkForRequest(ctx context.Context, item core.WorkItem) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if typed, ok := s.store.(coreIntakeContextStore); ok {
		return typed.CreateWorkContext(ctx, item)
	}
	return s.store.CreateWork(item)
}

func (s *Server) getWorkForRequest(ctx context.Context, id string) (core.WorkItem, error) {
	if err := ctx.Err(); err != nil {
		return core.WorkItem{}, err
	}
	if typed, ok := s.store.(coreIntakeContextStore); ok {
		return typed.GetWorkContext(ctx, id)
	}
	return s.store.GetWork(id)
}

func (s *Server) freezeTaskForRequest(ctx context.Context, task core.TaskContract, plan verification.Plan, version uint64, work core.WorkItem) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if typed, ok := s.store.(coreIntakeContextStore); ok {
		return typed.CreateTaskAndUpdateWorkContext(ctx, task, plan, version, work)
	}
	return s.store.CreateTaskAndUpdateWork(task, plan, version, work)
}

func (s *Server) getTaskForRequest(ctx context.Context, id string) (core.TaskContract, error) {
	if err := ctx.Err(); err != nil {
		return core.TaskContract{}, err
	}
	if typed, ok := s.store.(coreIntakeContextStore); ok {
		return typed.GetTaskContext(ctx, id)
	}
	return s.store.GetTask(id)
}
