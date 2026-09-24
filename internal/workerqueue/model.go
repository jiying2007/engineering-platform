// Package workerqueue owns durable Run-intent admission. INPUT_VALIDATED is
// deliberately not execution, material readiness, Evidence or Run completion.
package workerqueue

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/core"
)

var (
	ErrIdentity = errors.New("worker input identity mismatch")
	ErrLease    = errors.New("worker lease is absent, expired or superseded")
	ErrRecovery = errors.New("worker admission blocked by recovery")
	ErrInactive = errors.New("run is terminal or no longer runtime-owned")
	ErrPaused   = errors.New("run or session is paused; defer without revoking intent")
)

const Protocol = "worker-input-v1"
const Validated = "INPUT_VALIDATED"

var logicalName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/-]{0,191}$`)
var fullCommit = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)

func ValidProfile(profile string) bool { return logicalName.MatchString(profile) }

type Intent struct {
	RunID          string `json:"run_id"`
	TaskDigest     string `json:"task_contract_digest"`
	InputDigest    string `json:"run_input_manifest_digest"`
	ExecutionEpoch uint64 `json:"execution_epoch"`
}

func (i Intent) Digest() (string, error) {
	if i.RunID == "" || len(i.RunID) > 256 || i.ExecutionEpoch == 0 || !canonical.ValidDigest(i.TaskDigest) || !canonical.ValidDigest(i.InputDigest) {
		return "", ErrIdentity
	}
	return canonical.Digest(i)
}

type Token struct {
	Profile       string `json:"worker_profile"`
	InboxID       int64  `json:"inbox_id"`
	Generation    uint64 `json:"lease_generation"`
	RecoveryEpoch uint64 `json:"recovery_epoch"`
}

func (t Token) Valid() bool { return t.InboxID > 0 && t.Generation > 0 && ValidProfile(t.Profile) }

type Assignment struct {
	Token        Token                 `json:"token"`
	LeaseUntil   time.Time             `json:"lease_until"`
	Intent       Intent                `json:"intent"`
	IntentDigest string                `json:"intent_digest"`
	Input        core.RunInputManifest `json:"run_input"`
	Task         core.TaskContract     `json:"task"`
}
type Validation struct {
	Protocol             string `json:"protocol"`
	IntentDigest         string `json:"intent_digest"`
	InputDigest          string `json:"run_input_manifest_digest"`
	TaskDigest           string `json:"task_contract_digest"`
	ContextCount         int    `json:"context_count"`
	ContextBytesVerified bool   `json:"context_bytes_verified"`
	ExecutionStarted     bool   `json:"execution_started"`
}

// Validate checks frozen identities, not source bytes, tools or provider output.
// Both worker and server compute this same deterministic validation record.
func Validate(a Assignment) (Validation, error) {
	digest, err := a.Intent.Digest()
	if err != nil || digest != a.IntentDigest || !a.Token.Valid() {
		return Validation{}, ErrIdentity
	}
	inputDigest, err := a.Input.Digest()
	if err != nil || inputDigest != a.Intent.InputDigest || a.Input.RunID != a.Intent.RunID || a.Input.TaskContractDigest != a.Intent.TaskDigest {
		return Validation{}, ErrIdentity
	}
	taskDigest, err := a.Task.Digest()
	if err != nil || taskDigest != a.Intent.TaskDigest || a.Task.ID == "" || a.Task.WorkItemID == "" || !fullCommit.MatchString(a.Task.BaseCommit) || len(a.Task.AcceptanceCriteria) == 0 {
		return Validation{}, ErrIdentity
	}
	if a.Token.Profile != a.Input.WorkerProfile || a.Input.RuntimeProfile == "" || a.Input.ToolProfile == "" || !ValidProfile(a.Input.WorkerProfile) || a.Input.PolicyProfile == "" {
		return Validation{}, ErrIdentity
	}
	return Validation{Protocol: Protocol, IntentDigest: digest, InputDigest: inputDigest, TaskDigest: taskDigest, ContextCount: len(a.Input.ContextRefs)}, nil
}

type Report struct {
	Token            Token  `json:"token"`
	ValidationDigest string `json:"validation_digest"`
}

func NewReport(a Assignment) (Report, error) {
	v, err := Validate(a)
	if err != nil {
		return Report{}, err
	}
	digest, err := canonical.Digest(v)
	return Report{Token: a.Token, ValidationDigest: digest}, err
}

type Receipt struct {
	Token      Token      `json:"token"`
	Worker     string     `json:"worker_subject"`
	Kind       string     `json:"kind"`
	Validation Validation `json:"validation"`
	ReceivedAt time.Time  `json:"received_at"`
}
type Status struct {
	InboxID    int64    `json:"inbox_id"`
	RunID      string   `json:"run_id"`
	State      string   `json:"state"`
	Generation uint64   `json:"lease_generation"`
	Receipt    *Receipt `json:"receipt,omitempty"`
}
type Repository interface {
	ClaimInput(context.Context, string, string) (*Assignment, error)
	RenewInput(context.Context, string, Token) (time.Time, error)
	ReportInput(context.Context, string, Report) (Receipt, error)
	GetInbox(context.Context, string) (Status, error)
}

func VerifyReceipt(a Assignment, r Receipt) error {
	v, err := Validate(a)
	if err != nil || r.Token != a.Token || r.Kind != Validated || r.Validation != v || r.Worker == "" || r.ReceivedAt.IsZero() {
		return fmt.Errorf("%w: invalid admission receipt", ErrIdentity)
	}
	return nil
}
