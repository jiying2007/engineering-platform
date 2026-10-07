package recovery

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jiying2007/engineering-platform/internal/canonical"
)

const (
	ExecutionAbandonmentKind = "RECOVERY_EXECUTION_ABANDONMENT_V1"
	ExecutionOffline         = "OFFLINE"
	ExecutionCodex           = "CODEX"
	ExecutionAbandoned       = "ABANDONED_RECONCILED"
	AbandonNoReplay          = "ABANDON_NO_REPLAY"
)

var executionIDPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

type ExecutionAbandonRequest struct {
	Version           int    `json:"version"`
	ExecutionKind     string `json:"execution_kind"`
	RunID             string `json:"run_id"`
	ExecutionID       string `json:"execution_id"`
	RecoveryEpoch     uint64 `json:"recovery_epoch"`
	ObservationDigest string `json:"observation_digest"`
	Disposition       string `json:"disposition"`
}

func (r ExecutionAbandonRequest) Validate() error {
	if r.Version != 1 ||
		(r.ExecutionKind != ExecutionOffline && r.ExecutionKind != ExecutionCodex) ||
		r.RunID == "" || len(r.RunID) > 256 || strings.TrimSpace(r.RunID) != r.RunID ||
		strings.ContainsAny(r.RunID, "\x00\r\n") ||
		!executionIDPattern.MatchString(r.ExecutionID) ||
		r.RecoveryEpoch == 0 || !canonical.ValidDigest(r.ObservationDigest) ||
		r.Disposition != AbandonNoReplay {
		return fmt.Errorf("invalid execution reconciliation request")
	}
	return nil
}

func (r ExecutionAbandonRequest) Digest() (string, error) {
	if err := r.Validate(); err != nil {
		return "", err
	}
	return canonical.Digest(r)
}

type ExecutionAbandonReceipt struct {
	Kind                string                  `json:"kind"`
	Request             ExecutionAbandonRequest `json:"request"`
	RequestDigest       string                  `json:"request_digest"`
	Reconciler          string                  `json:"reconciler"`
	PreviousState       string                  `json:"previous_state"`
	CreatedAt           time.Time               `json:"created_at"`
	ExecutionAuthorized bool                    `json:"execution_authorized"`
	ReplayAuthorized    bool                    `json:"replay_authorized"`
	ProductionQualified bool                    `json:"production_qualified"`
}

func (r ExecutionAbandonReceipt) Validate() error {
	digest, err := r.Request.Digest()
	if err != nil || r.Kind != ExecutionAbandonmentKind || r.RequestDigest != digest ||
		!boundedProofString(r.Reconciler, 256) ||
		(r.PreviousState != "AUTHORIZED" && r.PreviousState != "UNKNOWN") ||
		r.CreatedAt.IsZero() || r.ExecutionAuthorized || r.ReplayAuthorized || r.ProductionQualified {
		return fmt.Errorf("invalid execution reconciliation receipt")
	}
	return nil
}
