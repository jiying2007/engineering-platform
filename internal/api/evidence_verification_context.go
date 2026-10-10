package api

import (
	"context"

	"github.com/jiying2007/engineering-platform/internal/core"
	"github.com/jiying2007/engineering-platform/internal/verification"
)

// evidenceVerificationContextStore is a caller-bound capability of the
// existing Core authority. The verification plan and delivery/evidence
// inputs remain immutable; no new Evidence or Review authority is created.
type evidenceVerificationContextStore interface {
	GetVerificationPlanByDigestContext(context.Context, string) (verification.Plan, error)
	CreateEvidenceContext(context.Context, core.EvidenceRef) error
	GetEvidenceContext(context.Context, string) (core.EvidenceRef, error)
	CreateVerificationContext(context.Context, verification.Report) error
	GetVerificationContext(context.Context, string) (verification.Report, error)
}

func (s *Server) getVerificationPlanForRequest(ctx context.Context, digest string) (verification.Plan, error) {
	if err := ctx.Err(); err != nil {
		return verification.Plan{}, err
	}
	if typed, ok := s.store.(evidenceVerificationContextStore); ok {
		return typed.GetVerificationPlanByDigestContext(ctx, digest)
	}
	return s.store.GetVerificationPlanByDigest(digest)
}

func (s *Server) registerEvidenceForRequest(ctx context.Context, item core.EvidenceRef) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if typed, ok := s.store.(evidenceVerificationContextStore); ok {
		return typed.CreateEvidenceContext(ctx, item)
	}
	return s.store.CreateEvidence(item)
}

func (s *Server) getEvidenceForRequest(ctx context.Context, id string) (core.EvidenceRef, error) {
	if err := ctx.Err(); err != nil {
		return core.EvidenceRef{}, err
	}
	if typed, ok := s.store.(evidenceVerificationContextStore); ok {
		return typed.GetEvidenceContext(ctx, id)
	}
	return s.store.GetEvidence(id)
}

func (s *Server) createVerificationForRequest(ctx context.Context, item verification.Report) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if typed, ok := s.store.(evidenceVerificationContextStore); ok {
		return typed.CreateVerificationContext(ctx, item)
	}
	return s.store.CreateVerification(item)
}

func (s *Server) getVerificationForRequest(ctx context.Context, id string) (verification.Report, error) {
	if err := ctx.Err(); err != nil {
		return verification.Report{}, err
	}
	if typed, ok := s.store.(evidenceVerificationContextStore); ok {
		return typed.GetVerificationContext(ctx, id)
	}
	return s.store.GetVerification(id)
}
