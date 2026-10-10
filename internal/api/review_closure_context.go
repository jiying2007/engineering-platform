package api

import (
	"context"

	"github.com/jiying2007/engineering-platform/internal/core"
	"github.com/jiying2007/engineering-platform/internal/review"
)

// reviewClosureContextStore adds caller cancellation to the existing Core
// Review/Closure authority without changing review independence or state.
type reviewClosureContextStore interface {
	CreateReviewAndUpdateWorkContext(context.Context, review.Report, uint64, core.WorkItem) error
	GetReviewContext(context.Context, string) (review.Report, error)
	CreateClosureAndUpdateWorkContext(context.Context, core.ClosureReceipt, uint64, core.WorkItem) error
	GetClosureContext(context.Context, string) (core.ClosureReceipt, error)
}

func (s *Server) createReviewForRequest(ctx context.Context, report review.Report, version uint64, work core.WorkItem) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if typed, ok := s.store.(reviewClosureContextStore); ok {
		return typed.CreateReviewAndUpdateWorkContext(ctx, report, version, work)
	}
	return s.store.CreateReviewAndUpdateWork(report, version, work)
}

func (s *Server) getReviewForRequest(ctx context.Context, id string) (review.Report, error) {
	if err := ctx.Err(); err != nil {
		return review.Report{}, err
	}
	if typed, ok := s.store.(reviewClosureContextStore); ok {
		return typed.GetReviewContext(ctx, id)
	}
	return s.store.GetReview(id)
}

func (s *Server) createClosureForRequest(ctx context.Context, item core.ClosureReceipt, version uint64, work core.WorkItem) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if typed, ok := s.store.(reviewClosureContextStore); ok {
		return typed.CreateClosureAndUpdateWorkContext(ctx, item, version, work)
	}
	return s.store.CreateClosureAndUpdateWork(item, version, work)
}

func (s *Server) getClosureForRequest(ctx context.Context, id string) (core.ClosureReceipt, error) {
	if err := ctx.Err(); err != nil {
		return core.ClosureReceipt{}, err
	}
	if typed, ok := s.store.(reviewClosureContextStore); ok {
		return typed.GetClosureContext(ctx, id)
	}
	return s.store.GetClosure(id)
}
