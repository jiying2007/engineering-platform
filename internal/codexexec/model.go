// Package codexexec owns one non-replayable Core-bound Codex engineering execution.
package codexexec

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/preparation"
	"github.com/jiying2007/engineering-platform/internal/runtime/codexapp"
	"github.com/jiying2007/engineering-platform/internal/workerqueue"
	"github.com/jiying2007/engineering-platform/internal/workspace"
)

const (
	Action     = "worker.codex-execute"
	Kind       = "WORKER_ATTESTED_CODEX_EXECUTION"
	Authorized = "AUTHORIZED"
	Finished   = "FINISHED"
	Unknown    = "UNKNOWN"
)

var tokenID = regexp.MustCompile(`^[0-9a-f]{64}$`)

type Profile struct {
	Version                 int    `json:"version"`
	CodexVersion            string `json:"codex_version"`
	BinaryDigest            string `json:"binary_digest"`
	EngineeringConfigDigest string `json:"engineering_config_digest"`
	Model                   string `json:"model"`
	Sandbox                 string `json:"sandbox"`
	ApprovalPolicy          string `json:"approval_policy"`
	ToolNetwork             bool   `json:"tool_network"`
}

func (p Profile) Validate() error {
	if p.Version != 1 || p.CodexVersion != codexapp.QualifiedCodexVersion ||
		!canonical.ValidDigest(p.BinaryDigest) ||
		p.EngineeringConfigDigest != codexapp.EngineeringConfigDigest() ||
		strings.TrimSpace(p.Model) == "" || len(p.Model) > 128 ||
		p.Sandbox != "workspace-write" || p.ApprovalPolicy != "never" || p.ToolNetwork {
		return workerqueue.ErrIdentity
	}
	return nil
}

func (p Profile) Digest() (string, error) {
	if err := p.Validate(); err != nil {
		return "", err
	}
	return canonical.Digest(p)
}

type Start struct {
	RunID         string  `json:"run_id"`
	WorkerProfile string  `json:"worker_profile"`
	Profile       Profile `json:"profile"`
}

type Token struct {
	ID            string `json:"execution_id"`
	RunID         string `json:"run_id"`
	WorkerProfile string `json:"worker_profile"`
	ProfileDigest string `json:"profile_digest"`
	RecoveryEpoch uint64 `json:"recovery_epoch"`
}

func (t Token) Valid() bool {
	return tokenID.MatchString(t.ID) && t.RunID != "" && len(t.RunID) <= 256 &&
		workerqueue.ValidProfile(t.WorkerProfile) && canonical.ValidDigest(t.ProfileDigest)
}

type Permit struct {
	Token       Token                  `json:"token"`
	Assignment  workerqueue.Assignment `json:"assignment"`
	Preparation preparation.Receipt    `json:"preparation"`
	Profile     Profile                `json:"profile"`
	LeaseUntil  time.Time              `json:"lease_until"`
}

func (p Permit) Check(subject string, request Start) error {
	digest, err := p.Profile.Digest()
	want, werr := request.Profile.Digest()
	if err != nil || werr != nil || digest != want || !p.Token.Valid() ||
		p.Token.ProfileDigest != digest || p.Token.WorkerProfile != request.WorkerProfile ||
		p.Token.RunID != request.RunID || p.Assignment.Intent.RunID != request.RunID ||
		p.Assignment.Input.WorkerProfile != request.WorkerProfile || p.LeaseUntil.IsZero() {
		return workerqueue.ErrIdentity
	}
	if err := CheckAssignment(p.Assignment, p.Profile); err != nil {
		return err
	}
	return preparation.Verify(p.Assignment, p.Preparation, p.Preparation.Facts, subject)
}

type PromptIdentity struct {
	Version            int      `json:"version"`
	RunID              string   `json:"run_id"`
	TaskContractDigest string   `json:"task_contract_digest"`
	RunInputDigest     string   `json:"run_input_manifest_digest"`
	TaskType           string   `json:"task_type"`
	Repository         string   `json:"repository"`
	BaseCommit         string   `json:"base_commit"`
	TargetID           string   `json:"target_id,omitempty"`
	CapabilityIDs      []string `json:"capability_ids,omitempty"`
	SkillIDs           []string `json:"skill_ids,omitempty"`
	AcceptanceCriteria []string `json:"acceptance_criteria"`
	ExpectedOutputs    []string `json:"expected_outputs,omitempty"`
	BundleDigest       string   `json:"context_bundle_digest"`
}

func promptIdentity(a workerqueue.Assignment, prep preparation.Receipt) (PromptIdentity, string, error) {
	if _, err := workerqueue.Validate(a); err != nil {
		return PromptIdentity{}, "", err
	}
	if preparation.Verify(a, prep, prep.Facts, prep.Admission.Worker) != nil ||
		!canonical.ValidDigest(prep.Facts.BundleDigest) {
		return PromptIdentity{}, "", workerqueue.ErrIdentity
	}
	identity := PromptIdentity{
		Version: 1, RunID: a.Intent.RunID, TaskContractDigest: a.Intent.TaskDigest,
		RunInputDigest: a.Intent.InputDigest, TaskType: a.Task.TaskType,
		Repository: a.Task.Repository, BaseCommit: strings.ToLower(a.Task.BaseCommit),
		TargetID: a.Task.TargetID, CapabilityIDs: append([]string(nil), a.Task.CapabilityIDs...),
		SkillIDs: append([]string(nil), a.Task.SkillIDs...),
		AcceptanceCriteria: append([]string(nil), a.Task.AcceptanceCriteria...),
		ExpectedOutputs: append([]string(nil), a.Task.ExpectedOutputs...),
		BundleDigest: prep.Facts.BundleDigest,
	}
	digest, err := canonical.Digest(identity)
	return identity, digest, err
}

func PromptIdentityDigest(a workerqueue.Assignment, prep preparation.Receipt) (string, error) {
	_, digest, err := promptIdentity(a, prep)
	return digest, err
}

func Prompt(a workerqueue.Assignment, prep preparation.Receipt, bundlePath string) (string, string, error) {
	if strings.TrimSpace(bundlePath) == "" {
		return "", "", workerqueue.ErrIdentity
	}
	identity, digest, err := promptIdentity(a, prep)
	if err != nil {
		return "", "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "You are executing one frozen embedded-software TaskContract.\n")
	fmt.Fprintf(&b, "Modify only the current repository workspace. Do not commit, push, fetch, install packages, or use network access.\n")
	fmt.Fprintf(&b, "Use local tools/tests as needed. Finish with a concise summary of changes and tests.\n\n")
	fmt.Fprintf(&b, "Run: %s\nTask type: %s\nRepository: %s\nBase commit: %s\n", identity.RunID, identity.TaskType, identity.Repository, identity.BaseCommit)
	if identity.TargetID != "" {
		fmt.Fprintf(&b, "Target: %s\n", identity.TargetID)
	}
	if len(identity.CapabilityIDs) > 0 {
		fmt.Fprintf(&b, "Capabilities: %s\n", strings.Join(identity.CapabilityIDs, ", "))
	}
	if len(identity.SkillIDs) > 0 {
		fmt.Fprintf(&b, "Skills: %s\n", strings.Join(identity.SkillIDs, ", "))
	}
	fmt.Fprintf(&b, "Approved context bundle: %s\nContext bundle digest: %s\n", bundlePath, identity.BundleDigest)
	fmt.Fprintf(&b, "\nAcceptance criteria:\n")
	for i, criterion := range identity.AcceptanceCriteria {
		fmt.Fprintf(&b, "%d. %s\n", i+1, criterion)
	}
	if len(identity.ExpectedOutputs) > 0 {
		fmt.Fprintf(&b, "\nExpected outputs:\n")
		for _, output := range identity.ExpectedOutputs {
			fmt.Fprintf(&b, "- %s\n", output)
		}
	}
	prompt := b.String()
	if len(prompt) > 64<<10 {
		return "", "", fmt.Errorf("rendered Codex prompt exceeds limit")
	}
	return prompt, digest, nil
}

type Result struct {
	PromptIdentityDigest string                      `json:"prompt_identity_digest"`
	Codex                codexapp.EngineeringReceipt `json:"codex"`
	Change               workspace.ChangeFacts       `json:"change"`
}

func (r Result) Validate(p Profile, permit Permit) error {
	pd, err := p.Digest()
	expectedPrompt, promptErr := PromptIdentityDigest(permit.Assignment, permit.Preparation)
	if err != nil || promptErr != nil || permit.Token.ProfileDigest != pd ||
		r.PromptIdentityDigest != expectedPrompt ||
		r.Codex.Validate() != nil || r.Codex.BinaryDigest != p.BinaryDigest ||
		r.Codex.EngineeringConfigDigest != p.EngineeringConfigDigest || r.Codex.Model != p.Model ||
		r.Change.Recipe != workspace.FinalizeRecipe ||
		r.Change.BaseCommit != permit.Preparation.Facts.BaseCommit ||
		r.Change.BaseTree != permit.Preparation.Facts.TreeCommit ||
		r.Change.BaseSourceDigest != permit.Preparation.Facts.SourceDigest ||
		r.Change.ResultCommit == r.Change.BaseCommit || r.Change.ResultTree == r.Change.BaseTree ||
		r.Change.ResultSourceDigest == r.Change.BaseSourceDigest ||
		!canonical.ValidDigest(r.Change.ResultSourceDigest) || !canonical.ValidDigest(r.Change.BundleDigest) ||
		r.Change.BundleSize <= 0 {
		return workerqueue.ErrIdentity
	}
	return nil
}

type Report struct {
	Token  Token  `json:"token"`
	Result Result `json:"result"`
}

type Receipt struct {
	Kind              string    `json:"kind"`
	Token             Token     `json:"token"`
	Worker            string    `json:"worker_subject"`
	PreparationDigest string    `json:"preparation_digest"`
	Result            Result    `json:"result"`
	ResultDigest      string    `json:"result_digest"`
	ReceivedAt        time.Time `json:"received_at"`
}

func (r Receipt) Verify(subject string, p Permit, result Result) error {
	digest, err := canonical.Digest(result)
	actual, aerr := canonical.Digest(r.Result)
	if err != nil || aerr != nil || digest != actual || r.ResultDigest != digest ||
		r.Kind != Kind || r.Worker != subject || r.Token != p.Token ||
		r.PreparationDigest != p.Preparation.FactsDigest || r.ReceivedAt.IsZero() ||
		r.Result.Validate(p.Profile, p) != nil {
		return fmt.Errorf("%w: Codex receipt mismatch", workerqueue.ErrIdentity)
	}
	return nil
}

type Status struct {
	Token      Token     `json:"token"`
	State      string    `json:"state"`
	LeaseUntil time.Time `json:"lease_until"`
	Receipt    *Receipt  `json:"receipt,omitempty"`
}

type Repository interface {
	StartCodex(context.Context, string, Start) (Permit, error)
	RenewCodex(context.Context, string, Token) (time.Time, error)
	FinishCodex(context.Context, string, Report) (Receipt, error)
	FailCodex(context.Context, string, Token) error
	GetCodex(context.Context, string) (Status, error)
}

func CheckAssignment(a workerqueue.Assignment, p Profile) error {
	digest, err := p.Digest()
	if err != nil {
		return err
	}
	if _, err = workerqueue.Validate(a); err != nil {
		return err
	}
	if a.Input.ToolProfile != "codex/"+digest {
		return workerqueue.ErrIdentity
	}
	for _, allowed := range a.Task.AllowedActions {
		if allowed == Action {
			return nil
		}
	}
	return workerqueue.ErrIdentity
}
