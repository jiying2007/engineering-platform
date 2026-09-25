package recovery

import (
	"fmt"
	"strings"
	"time"

	"github.com/jiying2007/engineering-platform/internal/canonical"
)

const ProofKind = "RECOVERY_RECONCILIATION_V1"

type Facts struct {
	ExternalUnresolved   uint64 `json:"external_unresolved"`
	MutationOutboxLeases uint64 `json:"mutation_outbox_leases"`
	WorkerLeases         uint64 `json:"worker_leases"`
	OfflineUnresolved    uint64 `json:"offline_unresolved"`
}

func (f Facts) Clear() bool {
	return f.ExternalUnresolved == 0 &&
		f.MutationOutboxLeases == 0 &&
		f.WorkerLeases == 0 &&
		f.OfflineUnresolved == 0
}

type Proof struct {
	Kind          string    `json:"kind"`
	RecoveryEpoch uint64    `json:"recovery_epoch"`
	Reconciler    string    `json:"reconciler"`
	Facts         Facts     `json:"facts"`
	FactsDigest   string    `json:"facts_digest"`
	AuditSequence uint64    `json:"audit_sequence"`
	AuditDigest   string    `json:"audit_digest"`
	CreatedAt     time.Time `json:"created_at"`
}

func (p Proof) Validate() error {
	if p.Kind != ProofKind || p.RecoveryEpoch == 0 || !boundedProofString(p.Reconciler, 256) ||
		!p.Facts.Clear() || !canonical.ValidDigest(p.FactsDigest) || p.CreatedAt.IsZero() {
		return fmt.Errorf("invalid recovery reconciliation proof")
	}
	digest, err := canonical.Digest(p.Facts)
	if err != nil || digest != p.FactsDigest {
		return fmt.Errorf("recovery facts digest mismatch")
	}
	if p.AuditSequence == 0 {
		if p.AuditDigest != "" {
			return fmt.Errorf("empty audit chain must not carry a digest")
		}
	} else if !canonical.ValidDigest(p.AuditDigest) {
		return fmt.Errorf("invalid audit head digest")
	}
	return nil
}

func (p Proof) Digest() (string, error) {
	if err := p.Validate(); err != nil {
		return "", err
	}
	return canonical.Digest(p)
}

func boundedProofString(value string, max int) bool {
	return value != "" && len(value) <= max && strings.TrimSpace(value) == value
}
