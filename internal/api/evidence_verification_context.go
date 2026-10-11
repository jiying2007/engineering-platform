package api

import (
	"context"

	"github.com/jiying2007/engineering-platform/internal/core"
	"github.com/jiying2007/engineering-platform/internal/verification"
)

// Every operation below is required by the original Core Store contract;
// an HTTP request cannot silently fall back to unscoped ledger methods.

func (s *Server) getVerificationPlanForRequest(ctx context.Context, digest string) (verification.Plan, error) {
	if err := ctx.Err(); err != nil {
		return verification.Plan{}, err
	}
	return s.store.GetVerificationPlanByDigestContext(ctx, digest)
}

func (s *Server) registerEvidenceForRequest(ctx context.Context, item core.EvidenceRef) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.store.CreateEvidenceContext(ctx, item)
}

func (s *Server) getEvidenceForRequest(ctx context.Context, id string) (core.EvidenceRef, error) {
	if err := ctx.Err(); err != nil {
		return core.EvidenceRef{}, err
	}
	return s.store.GetEvidenceContext(ctx, id)
}

func (s *Server) createVerificationForRequest(ctx context.Context, item verification.Report) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.store.CreateVerificationContext(ctx, item)
}

func (s *Server) getVerificationForRequest(ctx context.Context, id string) (verification.Report, error) {
	if err := ctx.Err(); err != nil {
		return verification.Report{}, err
	}
	return s.store.GetVerificationContext(ctx, id)
}
