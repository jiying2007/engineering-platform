package githubpublish

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
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

// State requires caller-bound Core reads. No detached background fallback is
// allowed while reconstructing a frozen publication plan after cancellation.
type State interface {
	GetExecutionContext(context.Context, string) (run.Run, session.Session, error)
	GetTaskByDigestContext(context.Context, string) (core.TaskContract, error)
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
	Version            int    `json:"version"`
	Repository         string `json:"repository"`
	BaseRef            string `json:"base_ref"`
	BaseCommit         string `json:"base_commit"`
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
	Outcome Observation        `json:"outcome"`
	Receipt PublicationReceipt `json:"receipt"`
}

type Remote interface {
	Publish(context.Context, Plan, string, DispatchIdentity) (PublicationReceipt, error)
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
	if err := ctx.Err(); err != nil {
		return action.DispatchResult{}, err
	}
	if req.Action != Action || req.Capability != Capability || req.RiskClass != action.ControlledMutation {
		return action.DispatchResult{}, fmt.Errorf("GitHub publication requires exact controlled-mutation grant")
	}
	plan, bundle, err := p.derive(ctx, req.RunID, req.ExecutionEpoch, req.ParametersDigest, true)
	if err != nil {
		if ctx.Err() != nil {
			return action.DispatchResult{}, ctx.Err()
		}
		return action.DispatchResult{
			Outcome: action.DispatchUnknown, ObservedState: "PRECONDITION_FAILED",
		}, nil
	}
	if err := ctx.Err(); err != nil {
		return action.DispatchResult{}, err
	}
	digest, err := action.RequestDigest(req)
	if err != nil {
		return action.DispatchResult{}, err
	}
	identity := DispatchIdentity{req.ID, req.RunID, req.ExecutionEpoch, req.RecoveryEpoch, req.IdempotencyKey, digest}
	receipt, err := p.remote.Publish(ctx, plan, bundle, identity)
	if err != nil {
		return action.DispatchResult{}, err
	}
	// The independent Publisher is a separate trust boundary. Never
	// authorize CONFIRMED from a remote receipt without binding every
	// externally observed identity back to the frozen Core plan.
	if err := receipt.Validate(plan); err != nil {
		return action.DispatchResult{}, fmt.Errorf("publisher returned an unbound publication receipt: %w", err)
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
	if err := ctx.Err(); err != nil {
		return action.ReconcileResult{}, err
	}
	if op.ObservedState == "PRECONDITION_FAILED" {
		return action.ReconcileResult{Outcome: action.ReconcileManual, ObservedState: op.ObservedState}, nil
	}
	if op.Action != Action || op.Capability != Capability || op.RiskClass != action.ControlledMutation {
		return action.ReconcileResult{Outcome: action.ReconcileManual, ObservedState: "publication grant mismatch"}, nil
	}
	plan, _, err := p.derive(ctx, op.RunID, op.ExecutionEpoch, "", false)
	if err != nil {
		if ctx.Err() != nil {
			return action.ReconcileResult{}, ctx.Err()
		}
		return action.ReconcileResult{Outcome: action.ReconcileManual, ObservedState: "publication identity no longer reconciles"}, nil
	}
	if err := ctx.Err(); err != nil {
		return action.ReconcileResult{}, err
	}
	result, err := p.remote.Observe(ctx, plan)
	if err != nil {
		return action.ReconcileResult{}, err
	}
	switch result.Outcome {
	case ObservedConfirmed:
		// Readback is not authority by itself: malformed or retargeted
		// remote observations cannot settle an UNKNOWN effect as CONFIRMED.
		if err := result.Receipt.Validate(plan); err != nil {
			return action.ReconcileResult{
				Outcome: action.ReconcileManual, ObservedState: "publication receipt does not bind frozen plan",
			}, nil
		}
		observed, err := encodeReceipt(result.Receipt)
		if err != nil {
			return action.ReconcileResult{}, err
		}
		// Keep the exact current observation for diagnosis, not as terminal
		// authority. Only the existing Recovery route can bind it to an
		// original completion, quiescence, epoch and independent reconciler.
		return action.ReconcileResult{
			Outcome:       action.ReconcileManual,
			ExternalRef:   result.Receipt.PullRequestURL,
			ObservedState: observed,
		}, nil
	case ObservedAbsent:
		// A remote negative observation is not independent proof that the
		// original Publisher stopped. A timed-out Git/PR mutation may still
		// appear later, regardless of whether its local source bundle exists.
		// Without independently retained quiescence, never grant retry.
		return action.ReconcileResult{
			Outcome:       action.ReconcileManual,
			ObservedState: "ABSENT_DISPATCH_NOT_QUIESCED",
		}, nil
	case ObservedPartial, ObservedConflict:
		return action.ReconcileResult{Outcome: action.ReconcileManual, ObservedState: string(result.Outcome)}, nil
	default:
		return action.ReconcileResult{Outcome: action.ReconcileManual, ObservedState: "UNRECOGNIZED"}, nil
	}
}

func (p *Provider) derive(ctx context.Context, runID string, epoch uint64, parametersDigest string, requireBundle bool) (Plan, string, error) {
	value, err := p.publicationRun(ctx, runID, epoch)
	if err != nil {
		return Plan{}, "", err
	}
	// Dispatch and ordinary observation still require current live authority.
	// Recovery has a separate readback path; it cannot reach Publish or a bundle.
	if err := value.CheckEpoch(epoch); err != nil {
		return Plan{}, "", err
	}
	plan, err := p.retainedPlan(ctx, value, epoch, parametersDigest)
	if err != nil {
		return Plan{}, "", err
	}
	if !requireBundle {
		return plan, "", nil
	}
	bundle, err := p.verifyBundle(ctx, plan.ExecutionID, plan.BundleDigest, plan.BundleSize)
	if err != nil {
		return Plan{}, "", err
	}
	return plan, bundle, nil
}

// originalPlan reconstructs only a retained original invocation. A Run may have
// completed, aborted, or advanced after that invocation. Its current epoch is
// not historical execution authority: the sealed Runtime transcript must bind
// the exact original epoch instead. The caller additionally matches the entire
// plan and Action identity to the independent Publisher's immutable start.
func (p *Provider) originalPlan(ctx context.Context, op action.Operation) (Plan, error) {
	if _, err := IdentityForOperation(op); err != nil {
		return Plan{}, err
	}
	value, err := p.publicationRun(ctx, op.RunID, op.ExecutionEpoch)
	if err != nil {
		return Plan{}, err
	}
	if op.ExecutionEpoch > value.CurrentEpoch {
		return Plan{}, fmt.Errorf("%w: original publication epoch exceeds retained Run", run.ErrStaleEpoch)
	}
	return p.retainedPlan(ctx, value, op.ExecutionEpoch, "")
}

func (p *Provider) publicationRun(ctx context.Context, runID string, epoch uint64) (run.Run, error) {
	if p == nil || p.state == nil || p.remote == nil || runID == "" || epoch == 0 {
		return run.Run{}, fmt.Errorf("publisher is not fully configured")
	}
	if err := ctx.Err(); err != nil {
		return run.Run{}, err
	}
	value, _, err := p.state.GetExecutionContext(ctx, runID)
	if err != nil {
		return run.Run{}, err
	}
	if value.ID != runID {
		return run.Run{}, fmt.Errorf("publication identity does not belong to retained Run")
	}
	return value, nil
}

func (p *Provider) retainedPlan(ctx context.Context, value run.Run, epoch uint64, parametersDigest string) (Plan, error) {
	var empty Plan
	task, err := p.state.GetTaskByDigestContext(ctx, value.TaskContractDigest)
	if err != nil {
		return empty, err
	}
	taskDigest, err := task.Digest()
	if err != nil || taskDigest != value.TaskContractDigest {
		return empty, fmt.Errorf("publication TaskContract does not bind frozen Run")
	}
	target, ok := p.targets[task.Repository]
	if !ok {
		return empty, fmt.Errorf("repository is not allowed by publisher policy")
	}
	status, err := p.state.GetCodex(ctx, value.ID)
	if err != nil {
		return empty, err
	}
	if status.VerifyFinishedControls() != nil || status.Token.RunID != value.ID ||
		status.Runtime.Binding.ExecutionEpoch != epoch {
		return empty, fmt.Errorf("publication requires exact original FINISHED Runtime epoch")
	}
	receipt := *status.Receipt
	if receipt.Token != status.Token || receipt.ResultDigest == "" {
		return empty, fmt.Errorf("Codex publication identity mismatch")
	}
	if parametersDigest != "" && parametersDigest != receipt.ResultDigest {
		return empty, fmt.Errorf("action parameters_digest does not bind the retained Codex result")
	}
	receiptDigest, err := engineeringevidence.CodexReceiptDigest(receipt)
	if err != nil {
		return empty, err
	}
	change := receipt.Result.Change
	if strings.ToLower(task.BaseCommit) != change.BaseCommit {
		return empty, fmt.Errorf("Codex base commit does not match frozen TaskContract")
	}
	if len(status.Token.ID) < 24 {
		return empty, fmt.Errorf("invalid Codex execution identity")
	}
	plan := Plan{
		Version:       1,
		RunID:         value.ID,
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
		return empty, err
	}
	// Identity readback needs only retained Core bytes. Bundle loss must not
	// authorize a retry or prevent observation of an original remote effect.
	return plan, ctx.Err()
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
		r.PullRequestURL != fmt.Sprintf("https://github.com/%s/pull/%d", plan.Repository, r.PullRequestNumber) {
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

func (p *Provider) verifyBundle(ctx context.Context, id, wantDigest string, wantSize int64) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if !executionID.MatchString(id) || !digestPattern.MatchString(wantDigest) || wantSize <= 0 || wantSize > 1<<30 {
		return "", fmt.Errorf("invalid retained bundle identity")
	}
	root, err := openBundleRoot(p.artifactRoot, p.artifactIdentity)
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
	file, err := openBundleRead(root, name, before, wantSize)
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	n, copyErr := copyBundle(ctx, hash, file, wantSize)
	closeErr := file.Close()
	after, afterErr := root.Lstat(name)
	if copyErr != nil || closeErr != nil || afterErr != nil || !os.SameFile(before, after) ||
		n != wantSize || n > 1<<30 || ctx.Err() != nil {
		return "", errors.Join(ctx.Err(), fmt.Errorf("retained result bundle changed during read"))
	}
	actual := "sha256:" + hex.EncodeToString(hash.Sum(nil))
	if actual != wantDigest {
		return "", fmt.Errorf("retained result bundle digest mismatch")
	}
	return filepath.Join(p.artifactRoot, name), nil
}
