package action

import "time"

type RiskClass string

const (
	Observe            RiskClass = "OBSERVE"
	ControlledMutation RiskClass = "CONTROLLED_MUTATION"
	HighRisk           RiskClass = "HIGH_RISK"
)

type Request struct {
	ID               string    `json:"action_request_id"`
	RunID            string    `json:"run_id"`
	ExecutionEpoch   uint64    `json:"execution_epoch"`
	RecoveryEpoch    uint64    `json:"recovery_epoch"`
	Action           string    `json:"action"`
	RiskClass        RiskClass `json:"risk_class"`
	Capability       string    `json:"capability"`
	ParametersDigest string    `json:"parameters_digest"`
	IdempotencyKey   string    `json:"idempotency_key"`
	RequestedBy      string    `json:"requested_by"`
	RequestedAt      time.Time `json:"requested_at"`
}

type Receipt struct {
	ID            string    `json:"action_receipt_id"`
	RequestID     string    `json:"action_request_id"`
	OperationID   string    `json:"operation_id"`
	Result        string    `json:"result"`
	ExternalRef   string    `json:"external_ref,omitempty"`
	ObservedState string    `json:"observed_state,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}
