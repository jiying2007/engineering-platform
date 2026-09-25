package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"reflect"
	"regexp"
	"strings"
	"time"

	"github.com/jiying2007/engineering-platform/internal/cievidence"
	"github.com/jiying2007/engineering-platform/internal/controlclient"
	"github.com/jiying2007/engineering-platform/internal/core"
	"github.com/jiying2007/engineering-platform/internal/run"
	"github.com/jiying2007/engineering-platform/internal/session"
)

var importIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,191}$`)

type taskReadResponse struct {
	Contract core.TaskContract `json:"contract"`
	Digest   string            `json:"digest"`
}

type runReadResponse struct {
	Run      run.Run               `json:"run"`
	Session  session.Session       `json:"session"`
	RunInput core.RunInputManifest `json:"run_input"`
}

type createDeliveryInput struct {
	ID           string             `json:"delivery_receipt_id"`
	RunID        string             `json:"run_id"`
	ResultCommit string             `json:"result_commit"`
	Artifacts    []core.ArtifactRef `json:"artifacts"`
	KnownLimits  []string           `json:"known_limits,omitempty"`
}

type createEvidenceInput struct {
	DeliveryReceiptID string           `json:"delivery_receipt_id"`
	Evidence          core.EvidenceRef `json:"evidence"`
}

type githubEvidenceImportResult struct {
	GitHubRunID    int64            `json:"github_run_id"`
	ReceiptDigest  string           `json:"ci_receipt_digest"`
	Delivery       core.DeliveryReceipt `json:"delivery"`
	Evidence       core.EvidenceRef  `json:"evidence"`
}

func importGitHubEvidence(args []string) error {
	fs := flag.NewFlagSet("evidence-import-github-ci", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	repository := fs.String("repository", "", "exact GitHub owner/repo")
	githubRun := fs.Int64("github-run-id", 0, "exact GitHub Actions PR workflow run ID")
	coreRun := fs.String("core-run-id", "", "completed Core Run ID")
	taskID := fs.String("task-id", "", "exact active TaskContract ID for the Core Run")
	deliveryID := fs.String("delivery-id", "", "deterministic DeliveryReceipt ID")
	requirementID := fs.String("requirement-id", "", "frozen VerificationPlan requirement ID")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 {
		return fmt.Errorf("usage: eng evidence-import-github-ci --repository owner/repo --github-run-id N --core-run-id ID --task-id ID --delivery-id ID --requirement-id ID")
	}
	if *repository == "" || *githubRun <= 0 || !safeImportID(*coreRun) || !safeImportID(*taskID) || !safeImportID(*deliveryID) || !boundedImportLabel(*requirementID) {
		return fmt.Errorf("explicit canonical repository/run/task/delivery/requirement identity required")
	}
	token := os.Getenv("GITHUB_TOKEN")
	verifier, err := cievidence.NewGitHubVerifier(token)
	if err != nil {
		return err
	}
	defer verifier.Close()
	control, err := controlclient.FromEnvironment(os.Getenv)
	if err != nil {
		return err
	}
	defer control.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	facts, err := verifier.ResolvePullRequest(ctx, *repository, *githubRun)
	if err != nil {
		return err
	}
	var task taskReadResponse
	if err := control.Call(ctx, http.MethodGet, "/api/v1/task-contracts/"+*taskID, nil, &task); err != nil {
		return fmt.Errorf("read task: %w", err)
	}
	digest, err := task.Contract.Digest()
	if err != nil || digest != task.Digest {
		return fmt.Errorf("task response digest mismatch")
	}
	if err := facts.ValidateTask(task.Contract); err != nil {
		return err
	}
	var execution runReadResponse
	if err := control.Call(ctx, http.MethodGet, "/api/v1/runs/"+*coreRun, nil, &execution); err != nil {
		return fmt.Errorf("read run: %w", err)
	}
	if execution.Run.ID != *coreRun || execution.Run.State != run.Completed || execution.Run.TaskContractDigest != task.Digest {
		return fmt.Errorf("Core Run is not completed under the exact verified task")
	}

	expectedDelivery := createDeliveryInput{
		ID:           *deliveryID,
		RunID:        *coreRun,
		ResultCommit: facts.SourceSHA(),
		Artifacts:    facts.Artifacts(),
		KnownLimits:  []string{"GitHub Actions attests CI/job/artifact facts; independent engineering verification remains required."},
	}
	delivery, err := createOrReconcileDelivery(ctx, control, expectedDelivery)
	if err != nil {
		return err
	}
	if delivery.TaskContractDigest != task.Digest || delivery.RunID != *coreRun {
		return fmt.Errorf("persisted delivery is bound to a different Core task/run")
	}
	evidence, err := facts.BindDelivery(delivery, *requirementID)
	if err != nil {
		return err
	}
	stored, err := createOrReconcileEvidence(ctx, control, delivery.ID, evidence)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(stored, evidence) {
		return fmt.Errorf("persisted Evidence differs from verified import")
	}
	printJSON(githubEvidenceImportResult{
		GitHubRunID:   *githubRun,
		ReceiptDigest: facts.ReceiptDigest(),
		Delivery:      delivery,
		Evidence:      stored,
	})
	return nil
}

func safeImportID(value string) bool {
	return importIDPattern.MatchString(value)
}

func boundedImportLabel(value string) bool {
	if value == "" || len(value) > 256 || strings.TrimSpace(value) != value {
		return false
	}
	for _, ch := range value {
		if ch < 0x20 || ch == 0x7f {
			return false
		}
	}
	return true
}

func createOrReconcileDelivery(ctx context.Context, client *controlclient.Client, input createDeliveryInput) (core.DeliveryReceipt, error) {
	var created core.DeliveryReceipt
	err := client.Call(ctx, http.MethodPost, "/api/v1/deliveries", input, &created)
	if err == nil {
		if !deliveryMatchesInput(created, input) {
			return core.DeliveryReceipt{}, fmt.Errorf("created delivery differs from deterministic input")
		}
		return created, nil
	}
	var existing core.DeliveryReceipt
	if readErr := client.Call(ctx, http.MethodGet, "/api/v1/deliveries/"+input.ID, nil, &existing); readErr != nil {
		return core.DeliveryReceipt{}, fmt.Errorf("delivery create outcome unresolved: create=%v reconcile=%v", err, readErr)
	}
	if !deliveryMatchesInput(existing, input) {
		return core.DeliveryReceipt{}, fmt.Errorf("existing delivery ID is bound to different content")
	}
	return existing, nil
}

func deliveryMatchesInput(delivery core.DeliveryReceipt, input createDeliveryInput) bool {
	return delivery.ID == input.ID &&
		delivery.RunID == input.RunID &&
		delivery.ResultCommit == input.ResultCommit &&
		reflect.DeepEqual(delivery.Artifacts, input.Artifacts) &&
		reflect.DeepEqual(delivery.KnownLimits, input.KnownLimits)
}

func createOrReconcileEvidence(ctx context.Context, client *controlclient.Client, deliveryID string, evidence core.EvidenceRef) (core.EvidenceRef, error) {
	input := createEvidenceInput{DeliveryReceiptID: deliveryID, Evidence: evidence}
	var created core.EvidenceRef
	err := client.Call(ctx, http.MethodPost, "/api/v1/evidence", input, &created)
	if err == nil {
		return created, nil
	}
	var existing core.EvidenceRef
	if readErr := client.Call(ctx, http.MethodGet, "/api/v1/evidence/"+evidence.ID, nil, &existing); readErr != nil {
		return core.EvidenceRef{}, fmt.Errorf("evidence registration outcome unresolved: create=%v reconcile=%v", err, readErr)
	}
	return existing, nil
}
