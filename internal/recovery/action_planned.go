package recovery

import (
	"fmt"
	"strings"
	"time"

	"github.com/jiying2007/engineering-platform/internal/canonical"
)

const ActionPlannedAbandonmentKind = "RECOVERY_ACTION_PLANNED_ABANDONMENT_V1"

// ActionPlannedAbandonRequest targets only a durable reservation which has
// never reached DISPATCHED. The observation digest identifies an independently
// retained operator record; its existence alone is NOT proof of remote absence.
// The actual no-dispatch proof is the locked PostgreSQL PLANNED state combined
// with the now-fenced Action admission/dispatch contract.
type ActionPlannedAbandonRequest struct {
	Version               int    `json:"version"`
	OperationID           string `json:"operation_id"`
	RunID                 string `json:"run_id"`
	ExecutionEpoch        uint64 `json:"execution_epoch"`
	OriginalRecoveryEpoch uint64 `json:"original_recovery_epoch"`
	RecoveryEpoch         uint64 `json:"recovery_epoch"`
	IdempotencyKey        string `json:"idempotency_key"`
	OriginalRequestDigest string `json:"original_request_digest"`
	ObservationDigest     string `json:"observation_digest"`
	Disposition           string `json:"disposition"`
}

func (r ActionPlannedAbandonRequest) Validate() error {
	for _, id := range []string{r.OperationID, r.RunID, r.IdempotencyKey} {
		if id == "" || len(id) > 256 || strings.TrimSpace(id) != id ||
			strings.ContainsAny(id, "\x00\r\n") {
			return fmt.Errorf("invalid Action reservation reconciliation identity")
		}
	}
	if r.Version != 1 || r.ExecutionEpoch == 0 || r.RecoveryEpoch == 0 ||
		r.OriginalRecoveryEpoch >= r.RecoveryEpoch ||
		r.RecoveryEpoch > 1<<62 ||
		!canonical.ValidDigest(r.OriginalRequestDigest) ||
		!canonical.ValidDigest(r.ObservationDigest) ||
		r.Disposition != AbandonNoReplay {
		return fmt.Errorf("invalid planned Action abandonment request")
	}
	return nil
}

func (r ActionPlannedAbandonRequest) Digest() (string, error) {
	if err := r.Validate(); err != nil {
		return "", err
	}
	return canonical.Digest(r)
}

type ActionPlannedAbandonReceipt struct {
	Kind                string                      `json:"kind"`
	Request             ActionPlannedAbandonRequest `json:"request"`
	RequestDigest       string                      `json:"request_digest"`
	Reconciler          string                      `json:"reconciler"`
	PreviousState       string                      `json:"previous_state"`
	CreatedAt           time.Time                   `json:"created_at"`
	EffectConfirmed     bool                        `json:"effect_confirmed"`
	ExecutionAuthorized bool                        `json:"execution_authorized"`
	ReplayAuthorized    bool                        `json:"replay_authorized"`
	ProductionQualified bool                        `json:"production_qualified"`
}

func (r ActionPlannedAbandonReceipt) Validate() error {
	digest, err := r.Request.Digest()
	if err != nil || r.Kind != ActionPlannedAbandonmentKind ||
		r.RequestDigest != digest || !boundedProofString(r.Reconciler, 256) ||
		r.PreviousState != "PLANNED" || r.CreatedAt.IsZero() ||
		r.EffectConfirmed || r.ExecutionAuthorized || r.ReplayAuthorized ||
		r.ProductionQualified {
		return fmt.Errorf("invalid planned Action abandonment receipt")
	}
	return nil
}
