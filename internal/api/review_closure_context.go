package api

import (
	"context"

	"github.com/jiying2007/engineering-platform/internal/core"
	"github.com/jiying2007/engineering-platform/internal/review"
)

// Every operation below is required by the original Core Store contract;
// an HTTP request cannot silently fall back to unscoped ledger methods.

func (s *Server) createReviewForRequest(ctx context.Context, report review.Report, version uint64, work core.WorkItem) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.store.CreateReviewAndUpdateWorkContext(ctx, report, version, work)
}

func (s *Server) getReviewForRequest(ctx context.Context, id string) (review.Report, error) {
	if err := ctx.Err(); err != nil {
		return review.Report{}, err
	}
	return s.store.GetReviewContext(ctx, id)
}

func (s *Server) createClosureForRequest(ctx context.Context, item core.ClosureReceipt, version uint64, work core.WorkItem) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.store.CreateClosureAndUpdateWorkContext(ctx, item, version, work)
}

func (s *Server) getClosureForRequest(ctx context.Context, id string) (core.ClosureReceipt, error) {
	if err := ctx.Err(); err != nil {
		return core.ClosureReceipt{}, err
	}
	return s.store.GetClosureContext(ctx, id)
}
