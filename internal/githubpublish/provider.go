package githubpublish

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/jiying2007/engineering-platform/internal/action"
	"github.com/jiying2007/engineering-platform/internal/codexexec"
	"github.com/jiying2007/engineering-platform/internal/core"
	"github.com/jiying2007/engineering-platform/internal/engineeringevidence"
	"github.com/jiying2007/engineering-platform/internal/run"
	"github.com/jiying2007/engineering-platform/internal/session"
)

const (
	Action     = "github.publish-pr"
	Capability = "github.publish-pr"
)

type State interface {
	GetExecution(string) (run.Run, session.Session, error)
	GetTaskByDigest(string) (core.TaskContract, error)
	GetCodex(context.Context, string) (codexexec.Status, error)
}

type Plan struct {
	Version       int    `json:"version"`
	RunID         string `json:"run_id"`
	Repository    string `json:"repository"`
	BaseRef       string `json:"base_ref"`
	BaseCommit    string `json:"base_commit"`
	Branch        string `json:"branch"`
	ResultCommit  string `json:"result_commit"`
	ResultDigest  string `json:"result_digest"`
	ReceiptDigest string `json:"codex_receipt_digest"`
	BundleDigest  string `json:"bundle_digest"`
	BundleSize    int64  `json:"bundle_size"`
	ExecutionID   string `json:"execution_id"`
}

type PublicationReceipt struct {
	Version           int    `json:"version"`
	Repository        string `json:"repository"`
	BaseRef           string `json:"base_ref"`
	BaseCommit        string `json:"base_commit"`
	Branch             string `json:"branch"`
	ResultCommit       string `json:"result_commit"`
	PullRequestNumber  int    `json:"pull_request_number"`
	PullRequestURL     string `json:"pull_request_url"`
	PullRequestState   string `json:"pull_request_state"`
	PublicationOutcome string `json:"publication_outcome"`
}

type Observation string

const (
	ObservedAbsent    Observation = "ABSENT"
	ObservedConfirmed Observation = "CONFIRMED"
	ObservedPartial   Observation = "PARTIAL"
	ObservedConflict  Observation = "CONFLICT"
)

type ObserveResult struct {
	Outcome Observation
	Receipt PublicationReceipt
}

type Remote interface {
	Publish(context.Context, Plan, string) (PublicationReceipt, error)
	Observe(context.Context, Plan) (ObserveResult, error)
}

type Provider struct {
	state            State
	artifactRoot     string
	artifactIdentity os.FileInfo
	targets          map[string]TargetPolicy
	remote           Remote
}

func (p *Provider) Dispatch(ctx context.Context, req action.Request) (action.DispatchResult, error) {
	if req.Action != Action || req.Capability != Capability || req.RiskClass != action.ControlledMutation {
		return action.DispatchResult{}, fmt.Errorf("GitHub publication requires exact controlled-mutation grant")
	}
	plan, bundle, err := p.derive(ctx, req.RunID, req.ExecutionEpoch, req.ParametersDigest)
	if err != nil {
		return action.DispatchResult{
			Outcome: action.DispatchUnknown, ObservedState: "PRECONDITION_FAILED",
		}, nil
	}
	receipt, err := p.remote.Publish(ctx, plan, bundle)
	if err != nil {
		return action.DispatchResult{}, err
	}
	observed, err := encodeReceipt(receipt)
	if err != nil {
		return action.DispatchResult{}, err
	}
	return action.DispatchResult{
		Outcome:       action.DispatchConfirmed,
		ExternalRef:   receipt.PullRequestURL,
		ObservedState: observed,
	}, nil
}

func (p *Provider) Reconcile(ctx context.Context, op action.Operation) (action.ReconcileResult, error) {
	if op.ObservedState == "PRECONDITION_FAILED" {
		return action.ReconcileResult{Outcome: action.ReconcileManual, ObservedState: op.ObservedState}, nil
	}
	if op.Action != Action || op.Capability != Capability || op.RiskClass != action.ControlledMutation {
		return action.ReconcileResult{Outcome: action.ReconcileManual, ObservedState: "publication grant mismatch"}, nil
	}
	plan, _, err := p.derive(ctx, op.RunID, op.ExecutionEpoch, "")
	if err != nil {
		return action.ReconcileResult{Outcome: action.ReconcileManual, ObservedState: "publication identity no longer reconciles"}, nil
	}
	result, err := p.remote.Observe(ctx, plan)
	if err != nil {
		return action.ReconcileResult{}, err
	}
	switch result.Outcome {
	case ObservedConfirmed:
		observed, err := encodeReceipt(result.Receipt)
		if err != nil {
			return action.ReconcileResult{}, err
		}
		return action.ReconcileResult{
			Outcome:       action.ReconcileConfirmed,
			ExternalRef:   result.Receipt.PullRequestURL,
			ObservedState: observed,
		}, nil
	case ObservedAbsent:
		return action.ReconcileResult{Outcome: action.ReconcileSafeToRetry, ObservedState: string(result.Outcome)}, nil
	case ObservedPartial, ObservedConflict:
		return action.ReconcileResult{Outcome: action.ReconcileManual, ObservedState: string(result.Outcome)}, nil
	default:
		return action.ReconcileResult{Outcome: action.ReconcileManual, ObservedState: "UNRECOGNIZED"}, nil
	}
}

func (p *Provider) derive(ctx context.Context, runID string, epoch uint64, parametersDigest string) (Plan, string, error) {
	var empty Plan
	if p == nil || p.state == nil || p.remote == nil || runID == "" || epoch == 0 {
		return empty, "", fmt.Errorf("publisher is not fully configured")
	}
	value, _, err := p.state.GetExecution(runID)
	if err != nil {
		return empty, "", err
	}
	if err := value.CheckEpoch(epoch); err != nil {
		return empty, "", err
	}
	task, err := p.state.GetTaskByDigest(value.TaskContractDigest)
	if err != nil {
		return empty, "", err
	}
	target, ok := p.targets[task.Repository]
	if !ok {
		return empty, "", fmt.Errorf("repository is not allowed by publisher policy")
	}
	status, err := p.state.GetCodex(ctx, runID)
	if err != nil {
		return empty, "", err
	}
	if status.State != codexexec.Finished || status.Receipt == nil || status.Token.RunID != runID {
		return empty, "", fmt.Errorf("publication requires one FINISHED Core-bound Codex result")
	}
	receipt := *status.Receipt
	if receipt.Token != status.Token || receipt.ResultDigest == "" {
		return empty, "", fmt.Errorf("Codex publication identity mismatch")
	}
	if parametersDigest != "" && parametersDigest != receipt.ResultDigest {
		return empty, "", fmt.Errorf("action parameters_digest does not bind the retained Codex result")
	}
	receiptDigest, err := engineeringevidence.CodexReceiptDigest(receipt)
	if err != nil {
		return empty, "", err
	}
	change := receipt.Result.Change
	if strings.ToLower(task.BaseCommit) != change.BaseCommit {
		return empty, "", fmt.Errorf("Codex base commit does not match frozen TaskContract")
	}
	if len(status.Token.ID) < 24 {
		return empty, "", fmt.Errorf("invalid Codex execution identity")
	}
	plan := Plan{
		Version:       1,
		RunID:         runID,
		Repository:    target.Repository,
		BaseRef:       target.BaseRef,
		BaseCommit:    change.BaseCommit,
		Branch:        target.BranchPrefix + status.Token.ID[:24],
		ResultCommit:  change.ResultCommit,
		ResultDigest:  receipt.ResultDigest,
		ReceiptDigest: receiptDigest,
		BundleDigest:  change.BundleDigest,
		BundleSize:    change.BundleSize,
		ExecutionID:   status.Token.ID,
	}
	if err := plan.Validate(); err != nil {
		return empty, "", err
	}
	bundle, err := p.verifyBundle(status.Token.ID, change.BundleDigest, change.BundleSize)
	if err != nil {
		return empty, "", err
	}
	return plan, bundle, nil
}

func (p Plan) Validate() error {
	if p.Version != 1 || p.RunID == "" || !validRepository(p.Repository) ||
		!validRef(p.BaseRef) || !validRef(p.Branch) ||
		!fullSHA.MatchString(p.BaseCommit) || !fullSHA.MatchString(p.ResultCommit) ||
		p.BaseCommit == p.ResultCommit || !digestPattern.MatchString(p.ResultDigest) ||
		!digestPattern.MatchString(p.ReceiptDigest) || !digestPattern.MatchString(p.BundleDigest) ||
		p.BundleSize <= 0 || p.BundleSize > 1<<30 || !executionID.MatchString(p.ExecutionID) {
		return fmt.Errorf("invalid GitHub publication plan")
	}
	return nil
}

func (r PublicationReceipt) Validate(plan Plan) error {
	if plan.Validate() != nil || r.Version != 1 || r.Repository != plan.Repository ||
		r.BaseRef != plan.BaseRef || r.BaseCommit != plan.BaseCommit || r.Branch != plan.Branch ||
		r.ResultCommit != plan.ResultCommit || r.PullRequestNumber <= 0 || r.PullRequestState != "open" ||
		!strings.HasPrefix(r.PullRequestURL, "https://github.com/"+plan.Repository+"/pull/") {
		return fmt.Errorf("GitHub publication receipt does not bind the plan")
	}
	switch r.PublicationOutcome {
	case "CREATED", "UPDATED", "EXISTING", "OBSERVED":
		return nil
	default:
		return fmt.Errorf("invalid publication outcome")
	}
}

func encodeReceipt(receipt PublicationReceipt) (string, error) {
	data, err := json.Marshal(receipt)
	if err != nil {
		return "", err
	}
	if len(data) == 0 || len(data) > 16<<10 {
		return "", fmt.Errorf("GitHub publication receipt outside size limit")
	}
	return string(data), nil
}

func (p *Provider) verifyBundle(id, wantDigest string, wantSize int64) (string, error) {
	if !executionID.MatchString(id) || !digestPattern.MatchString(wantDigest) || wantSize <= 0 || wantSize > 1<<30 {
		return "", fmt.Errorf("invalid retained bundle identity")
	}
	current, err := os.Lstat(p.artifactRoot)
	if err != nil || !current.IsDir() || !os.SameFile(current, p.artifactIdentity) {
		return "", fmt.Errorf("publisher artifact root changed")
	}
	root, err := os.OpenRoot(p.artifactRoot)
	if err != nil {
		return "", err
	}
	defer root.Close()
	name := id + ".bundle"
	before, err := root.Lstat(name)
	if err != nil || !before.Mode().IsRegular() || before.Mode().Perm()&0o022 != 0 ||
		before.Size() != wantSize {
		return "", fmt.Errorf("retained result bundle is not the expected regular file")
	}
	file, err := root.Open(name)
	if err != nil {
		return "", err
	}
	opened, err := file.Stat()
	if err != nil || !os.SameFile(before, opened) {
		_ = file.Close()
		return "", fmt.Errorf("retained result bundle changed before read")
	}
	hash := sha256.New()
	n, copyErr := io.Copy(hash, io.LimitReader(file, (1<<30)+1))
	closeErr := file.Close()
	after, afterErr := root.Lstat(name)
	if copyErr != nil || closeErr != nil || afterErr != nil || !os.SameFile(before, after) ||
		n != wantSize || n > 1<<30 {
		return "", fmt.Errorf("retained result bundle changed during read")
	}
	actual := "sha256:" + hex.EncodeToString(hash.Sum(nil))
	if actual != wantDigest {
		return "", fmt.Errorf("retained result bundle digest mismatch")
	}
	return filepath.Join(p.artifactRoot, name), nil
}
