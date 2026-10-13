package githubpublish

import (
	"context"
	"time"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/processscope"
)

const maxPublicationGitProofs = 32

// GitQuiescence is an original Publisher observation, not a portable credential.
// Core trusts only the configured mTLS Publisher, never operator-supplied JSON.
// It proves local namespace-init reap, NOT cancellation of an accepted remote call.
type GitQuiescence struct {
	Version int                  `json:"version"`
	Proofs  []processscope.Proof `json:"proofs"`
	Digest  string               `json:"digest"`
}

func (q GitQuiescence) validate(start, returned time.Time) error {
	if q.Version != 1 || len(q.Proofs) == 0 || len(q.Proofs) > maxPublicationGitProofs {
		return ErrDispatchEvidenceUnavailable
	}
	digest, err := canonical.Digest(q.Proofs)
	if err != nil || digest != q.Digest {
		return ErrDispatchEvidenceUnavailable
	}
	previous := start
	seen := make(map[string]bool, len(q.Proofs))
	for _, p := range q.Proofs {
		if !p.Quiescent() || p.StartedAt.Before(previous) || p.ReapedAt.After(returned) {
			return ErrDispatchEvidenceUnavailable
		}
		d, _ := canonical.Digest(p)
		if seen[d] {
			return ErrDispatchEvidenceUnavailable
		}
		seen[d] = true
		previous = p.ReapedAt
	}
	return nil
}

type completionProofKey struct{}
type completionProofLog struct {
	proofs  []processscope.Proof
	invalid bool
}

// The service creates this collector for ONE original synchronous Publish call.
// githubRemote's nested cleanup collector cannot replace it. There is no global
// proof cache and no observation-path collector that can upgrade old witnesses.
func recordPublicationReap(ctx context.Context, p processscope.Proof) error {
	log, _ := ctx.Value(completionProofKey{}).(*completionProofLog)
	if log == nil {
		return nil
	} // Direct legacy pilots cannot mint service witnesses.
	if !p.Quiescent() || len(log.proofs) >= maxPublicationGitProofs {
		log.invalid = true
		return ErrDispatchEvidenceUnavailable
	}
	log.proofs = append(log.proofs, p)
	return nil
}
func (l *completionProofLog) completed(start DispatchStart, receipt PublicationReceipt, at time.Time) (DispatchCompletion, error) {
	var zero DispatchCompletion
	if l == nil || l.invalid || start.Version != 2 {
		return zero, ErrDispatchEvidenceUnavailable
	}
	digest, err := canonical.Digest(start)
	if err != nil {
		return zero, err
	}
	q := &GitQuiescence{Version: 1, Proofs: append([]processscope.Proof(nil), l.proofs...)}
	q.Digest, err = canonical.Digest(q.Proofs)
	if err != nil {
		return zero, err
	}
	result := DispatchCompletion{Version: 2, StartDigest: digest, Receipt: receipt, ReturnedAt: at, GitQuiescence: q}
	if result.ValidateForRecovery(start) != nil {
		return zero, ErrDispatchEvidenceUnavailable
	}
	return result, nil
}

// ValidateForRecovery is stronger than historical byte readback. Never upgrade a
// v1 completion merely because the CURRENT binary happens to isolate Git. Already
// retained Core receipts keep structural readback; no history is rewritten.
func (c DispatchCompletion) ValidateForRecovery(s DispatchStart) error {
	if c.Version != 2 || s.Version != 2 {
		return ErrDispatchEvidenceUnavailable
	}
	return c.Validate(s)
}
func (e PublicationEvidence) ValidateForRecovery() error {
	if e.Validate() != nil {
		return ErrDispatchEvidenceUnavailable
	}
	return e.Completion.ValidateForRecovery(e.Start)
}
