package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/core"
	"github.com/jiying2007/engineering-platform/internal/recovery"
	"github.com/jiying2007/engineering-platform/internal/store"
)

func TestWorkTaskRunVerticalSlice(t *testing.T) {
	s := NewServer(store.NewMemory())
	h := s.Handler()

	taskDigest := createWorkAndTask(t, h, "work-1", "task-1", "FEATURE", "MCU driver")
	mustRequest(t, h, http.MethodPost, "/api/v1/runs", map[string]any{
		"run_id":               "run-1",
		"task_contract_digest": taskDigest,
		"attempt_id":           "attempt-1",
		"run_input": map[string]any{
			"runtime_profile": "codex/default",
			"tool_profile":    "tools/m1",
			"worker_profile":  "worker/ubuntu",
			"policy_profile":  "policy/m1",
		},
	}, http.StatusCreated)

	body := mustRequest(t, h, http.MethodGet, "/api/v1/runs/run-1", nil, http.StatusOK)
	var got struct {
		Run struct {
			TaskContractDigest string `json:"task_contract_digest"`
			Version            uint64 `json:"version"`
		} `json:"run"`
	}
	mustJSON(t, body, &got)
	if got.Run.TaskContractDigest != taskDigest || got.Run.Version == 0 {
		t.Fatalf("unexpected run: %#v", got.Run)
	}
}

func TestTaskCreationFailsClosedOnMaterial(t *testing.T) {
	s := NewServer(store.NewMemory())
	h := s.Handler()

	mustRequest(t, h, http.MethodPost, "/api/v1/work-items", map[string]any{
		"work_item_id": "work-1",
		"title":        "Debug boot",
		"human_owner":  "owner",
	}, http.StatusCreated)

	task := map[string]any{
		"contract": map[string]any{
			"task_contract_id": "task-1",
			"work_item_id":     "work-1",
			"task_type":        "DEBUG",
		},
		"material": map[string]any{
			"repository":          "repo",
			"base_commit":         "main",
			"acceptance_criteria": []string{"root cause"},
		},
		"subsystem": "Linux UBI",
	}
	mustRequest(t, h, http.MethodPost, "/api/v1/task-contracts", task, http.StatusUnprocessableEntity)
}

func TestPauseSteerAndTakeover(t *testing.T) {
	s := NewServer(store.NewMemory())
	h := s.Handler()

	taskDigest := createWorkAndTask(t, h, "work-s", "task-s", "FEATURE", "driver")
	mustRequest(t, h, http.MethodPost, "/api/v1/runs", map[string]any{
		"run_id":               "run-s",
		"task_contract_digest": taskDigest,
		"attempt_id":           "attempt-s",
		"run_input": map[string]any{
			"runtime_profile": "codex/default",
			"tool_profile":    "tools/m1",
			"worker_profile":  "worker/ubuntu",
			"policy_profile":  "policy/m1",
		},
	}, http.StatusCreated)

	mustRequest(t, h, http.MethodPost, "/api/v1/runs/run-s/steer", map[string]any{
		"steering_command_id": "steer-1",
		"execution_epoch":     1,
		"sequence":            1,
		"actor":               "engineer",
		"content_digest":      "sha256:steer",
	}, http.StatusAccepted)

	steerBody := mustRequest(t, h, http.MethodGet, "/api/v1/steering/steer-1", nil, http.StatusOK)
	var storedSteer struct {
		ID             string `json:"steering_command_id"`
		RunID          string `json:"run_id"`
		ExecutionEpoch uint64 `json:"execution_epoch"`
		Sequence       uint64 `json:"sequence"`
	}
	mustJSON(t, steerBody, &storedSteer)
	if storedSteer.ID != "steer-1" || storedSteer.RunID != "run-s" || storedSteer.ExecutionEpoch != 1 || storedSteer.Sequence != 1 {
		t.Fatalf("unexpected persisted steering command: %#v", storedSteer)
	}

	mustRequest(t, h, http.MethodPost, "/api/v1/runs/run-s/steer", map[string]any{
		"steering_command_id": "steer-duplicate-sequence",
		"execution_epoch":     1,
		"sequence":            1,
		"actor":               "engineer",
		"content_digest":      "sha256:duplicate",
	}, http.StatusConflict)

	mustRequest(t, h, http.MethodPost, "/api/v1/runs/run-s/pause", map[string]any{
		"execution_epoch": 1,
	}, http.StatusOK)
	mustRequest(t, h, http.MethodPost, "/api/v1/runs/run-s/resume", map[string]any{
		"execution_epoch": 1,
	}, http.StatusOK)
	mustRequest(t, h, http.MethodPost, "/api/v1/runs/run-s/takeover", map[string]any{
		"execution_epoch": 1,
	}, http.StatusOK)

	mustRequest(t, h, http.MethodPost, "/api/v1/runs/run-s/steer", map[string]any{
		"steering_command_id": "steer-stale",
		"execution_epoch":     1,
		"sequence":            2,
		"actor":               "runtime",
		"content_digest":      "sha256:late",
	}, http.StatusConflict)
}

func TestDeliveryRequiresCompletedRun(t *testing.T) {
	s := NewServer(store.NewMemory())
	h := s.Handler()

	taskDigest := createWorkAndTask(t, h, "work-d", "task-d", "FEATURE", "driver")
	mustRequest(t, h, http.MethodPost, "/api/v1/runs", map[string]any{
		"run_id":               "run-d",
		"task_contract_digest": taskDigest,
		"attempt_id":           "attempt-d",
		"run_input": map[string]any{
			"runtime_profile": "codex/default",
			"tool_profile":    "tools/m1",
			"worker_profile":  "worker/ubuntu",
			"policy_profile":  "policy/m1",
		},
	}, http.StatusCreated)

	mustRequest(t, h, http.MethodPost, "/api/v1/deliveries", map[string]any{
		"delivery_receipt_id": "delivery-d",
		"run_id":              "run-d",
		"result_commit":       "1111111111111111111111111111111111111111",
	}, http.StatusConflict)
}

func TestFeatureLifecycleClosesOnlyAfterExactVerification(t *testing.T) {
	s := NewServer(store.NewMemory())
	h := s.Handler()

	taskDigest := createWorkAndTask(t, h, "work-f", "task-f", "FEATURE", "driver")
	mustRequest(t, h, http.MethodPost, "/api/v1/runs", map[string]any{
		"run_id":               "run-f",
		"task_contract_digest": taskDigest,
		"attempt_id":           "attempt-f",
		"run_input": map[string]any{
			"runtime_profile": "codex/default",
			"tool_profile":    "tools/m1",
			"worker_profile":  "worker/ubuntu",
			"policy_profile":  "policy/m1",
		},
	}, http.StatusCreated)

	mustRequest(t, h, http.MethodPost, "/api/v1/runs/run-f/complete", map[string]any{
		"execution_epoch": 1,
	}, http.StatusOK)

	deliveryBody := mustRequest(t, h, http.MethodPost, "/api/v1/deliveries", map[string]any{
		"delivery_receipt_id": "delivery-f",
		"run_id":              "run-f",
		"result_commit":       "1111111111111111111111111111111111111111",
		"artifacts": []any{
			map[string]any{
				"artifact_id": "firmware",
				"digest":      "sha256:firmware",
				"media_type":  "application/octet-stream",
			},
		},
	}, http.StatusCreated)
	var delivery core.DeliveryReceipt
	mustJSON(t, deliveryBody, &delivery)
	if delivery.SubjectDigest == "" || delivery.TaskContractDigest != taskDigest {
		t.Fatalf("unexpected delivery identity: %#v", delivery)
	}

	mustRequest(t, h, http.MethodPost, "/api/v1/evidence", map[string]any{
		"delivery_receipt_id": "delivery-f",
		"evidence": map[string]any{
			"evidence_id":    "ev-f",
			"requirement_id": "req-1",
			"issuer":         "ci",
			"procedure":      "ci.test",
			"result":         "PASS",
			"applicable":     true,
		},
	}, http.StatusCreated)

	verificationBody := mustRequest(t, h, http.MethodPost, "/api/v1/verifications", map[string]any{
		"verification_report_id": "vr-f",
		"delivery_receipt_id":    "delivery-f",
		"verifier":               "verification-service",
		"evidence_ids":           []string{"ev-f"},
	}, http.StatusCreated)
	var verificationReport struct {
		Result            string `json:"result"`
		SubjectDigest     string `json:"subject_digest"`
		DeliveryReceiptID string `json:"delivery_receipt_id"`
	}
	mustJSON(t, verificationBody, &verificationReport)
	if verificationReport.Result != "PASS" ||
		verificationReport.SubjectDigest != delivery.SubjectDigest ||
		verificationReport.DeliveryReceiptID != delivery.ID {
		t.Fatalf("unexpected verification report: %#v", verificationReport)
	}

	// Verification alone cannot close work anymore.
	mustRequest(t, h, http.MethodPost, "/api/v1/closures", map[string]any{
		"closure_receipt_id":     "closure-without-review",
		"delivery_receipt_id":    "delivery-f",
		"verification_report_id": "vr-f",
	}, http.StatusBadRequest)

	for _, reviewer := range []string{"verification-service", "owner"} {
		mustRequest(t, h, http.MethodPost, "/api/v1/reviews", map[string]any{
			"review_report_id":       "review-denied-" + reviewer,
			"delivery_receipt_id":    "delivery-f",
			"verification_report_id": "vr-f",
			"reviewer":               reviewer,
			"result":                 "PASS",
		}, http.StatusUnprocessableEntity)
	}

	reviewBody := mustRequest(t, h, http.MethodPost, "/api/v1/reviews", map[string]any{
		"review_report_id":       "review-f",
		"delivery_receipt_id":    "delivery-f",
		"verification_report_id": "vr-f",
		"reviewer":               "independent-reviewer",
		"result":                 "PASS",
		"known_limits":           []string{"host remains trusted"},
	}, http.StatusCreated)
	var reviewReport struct {
		Result               string `json:"result"`
		SubjectDigest        string `json:"subject_digest"`
		DeliveryReceiptID    string `json:"delivery_receipt_id"`
		VerificationReportID string `json:"verification_report_id"`
	}
	mustJSON(t, reviewBody, &reviewReport)
	if reviewReport.Result != "PASS" ||
		reviewReport.SubjectDigest != delivery.SubjectDigest ||
		reviewReport.DeliveryReceiptID != delivery.ID ||
		reviewReport.VerificationReportID != "vr-f" {
		t.Fatalf("unexpected review report: %#v", reviewReport)
	}
	reviewingBody := mustRequest(t, h, http.MethodGet, "/api/v1/work-items/work-f", nil, http.StatusOK)
	var reviewing core.WorkItem
	mustJSON(t, reviewingBody, &reviewing)
	if reviewing.State != core.WorkReviewing {
		t.Fatalf("expected REVIEWING work, got %s", reviewing.State)
	}

	mustRequest(t, h, http.MethodPost, "/api/v1/closures", map[string]any{
		"closure_receipt_id":     "closure-f",
		"delivery_receipt_id":    "delivery-f",
		"verification_report_id": "vr-f",
		"review_report_id":       "review-f",
	}, http.StatusCreated)

	workBody := mustRequest(t, h, http.MethodGet, "/api/v1/work-items/work-f", nil, http.StatusOK)
	var work core.WorkItem
	mustJSON(t, workBody, &work)
	if work.State != core.WorkClosed {
		t.Fatalf("expected CLOSED work, got %s", work.State)
	}
}

func TestEvidenceFromAnotherDeliveryCannotVerifySubject(t *testing.T) {
	s := NewServer(store.NewMemory())
	h := s.Handler()

	deliveryA := createCompletedDelivery(t, h, "a")
	deliveryB := createCompletedDelivery(t, h, "b")

	mustRequest(t, h, http.MethodPost, "/api/v1/evidence", map[string]any{
		"delivery_receipt_id": deliveryA.ID,
		"evidence": map[string]any{
			"evidence_id":    "ev-a",
			"requirement_id": "req-1",
			"issuer":         "ci",
			"procedure":      "ci.test",
			"result":         "PASS",
			"applicable":     true,
		},
	}, http.StatusCreated)

	mustRequest(t, h, http.MethodPost, "/api/v1/verifications", map[string]any{
		"verification_report_id": "vr-cross",
		"delivery_receipt_id":    deliveryB.ID,
		"verifier":               "verification-service",
		"evidence_ids":           []string{"ev-a"},
	}, http.StatusUnprocessableEntity)
}

func TestFailedVerificationCannotClose(t *testing.T) {
	s := NewServer(store.NewMemory())
	h := s.Handler()

	delivery := createCompletedDelivery(t, h, "fail")
	mustRequest(t, h, http.MethodPost, "/api/v1/evidence", map[string]any{
		"delivery_receipt_id": delivery.ID,
		"evidence": map[string]any{
			"evidence_id":    "ev-fail",
			"requirement_id": "req-1",
			"issuer":         "ci",
			"procedure":      "ci.test",
			"result":         "FAIL",
			"applicable":     true,
		},
	}, http.StatusCreated)

	mustRequest(t, h, http.MethodPost, "/api/v1/verifications", map[string]any{
		"verification_report_id": "vr-fail",
		"delivery_receipt_id":    delivery.ID,
		"verifier":               "verification-service",
		"evidence_ids":           []string{"ev-fail"},
	}, http.StatusUnprocessableEntity)

	mustRequest(t, h, http.MethodPost, "/api/v1/closures", map[string]any{
		"closure_receipt_id":     "closure-fail",
		"delivery_receipt_id":    delivery.ID,
		"verification_report_id": "vr-fail",
		"review_report_id":       "review-fail",
	}, http.StatusUnprocessableEntity)
}

func TestFailedIndependentReviewStaysVerifyingAndCannotClose(t *testing.T) {
	s := NewServer(store.NewMemory())
	h := s.Handler()
	delivery := createCompletedDelivery(t, h, "review-fail")

	mustRequest(t, h, http.MethodPost, "/api/v1/evidence", map[string]any{
		"delivery_receipt_id": delivery.ID,
		"evidence": map[string]any{
			"evidence_id": "ev-review-fail",
			"issuer":      "ci",
			"procedure":   "ci.test",
			"result":      "PASS",
			"applicable":  true,
		},
	}, http.StatusCreated)
	mustRequest(t, h, http.MethodPost, "/api/v1/verifications", map[string]any{
		"verification_report_id": "vr-review-fail",
		"delivery_receipt_id":    delivery.ID,
		"verifier":               "verification-service",
		"evidence_ids":           []string{"ev-review-fail"},
	}, http.StatusCreated)

	mustRequest(t, h, http.MethodPost, "/api/v1/reviews", map[string]any{
		"review_report_id":       "review-fail",
		"delivery_receipt_id":    delivery.ID,
		"verification_report_id": "vr-review-fail",
		"reviewer":               "independent-reviewer",
		"result":                 "FAIL",
		"findings": []any{
			map[string]any{"finding_id": "blocking-1", "severity": "BLOCKING", "summary": "change is not acceptable"},
		},
	}, http.StatusCreated)

	workBody := mustRequest(t, h, http.MethodGet, "/api/v1/work-items/work-review-fail", nil, http.StatusOK)
	var work core.WorkItem
	mustJSON(t, workBody, &work)
	if work.State != core.WorkVerifying {
		t.Fatalf("failed review must remain repairable in VERIFYING, got %s", work.State)
	}
	mustRequest(t, h, http.MethodPost, "/api/v1/closures", map[string]any{
		"closure_receipt_id":     "closure-review-fail",
		"delivery_receipt_id":    delivery.ID,
		"verification_report_id": "vr-review-fail",
		"review_report_id":       "review-fail",
	}, http.StatusUnprocessableEntity)
}

func TestVerificationRejectsUnregisteredEvidenceReference(t *testing.T) {
	s := NewServer(store.NewMemory())
	h := s.Handler()
	delivery := createCompletedDelivery(t, h, "missing")

	mustRequest(t, h, http.MethodPost, "/api/v1/verifications", map[string]any{
		"verification_report_id": "vr-missing",
		"delivery_receipt_id":    delivery.ID,
		"verifier":               "verification-service",
		"evidence_ids":           []string{"ev-missing"},
	}, http.StatusNotFound)
}

func createWorkAndTask(t *testing.T, h http.Handler, workID, taskID, taskType, subsystem string) string {
	t.Helper()
	mustRequest(t, h, http.MethodPost, "/api/v1/work-items", map[string]any{
		"work_item_id": workID,
		"title":        "test work",
		"human_owner":  "owner",
	}, http.StatusCreated)
	body := mustRequest(t, h, http.MethodPost, "/api/v1/task-contracts", map[string]any{
		"contract": map[string]any{
			"task_contract_id": taskID,
			"work_item_id":     workID,
			"task_type":        taskType,
		},
		"material": map[string]any{
			"repository":          "repo",
			"base_commit":         "0123456789abcdef0123456789abcdef01234567",
			"target_id":           "target-1",
			"acceptance_criteria": []string{"tests pass"},
		},
		"subsystem": subsystem,
		"verification_plan": map[string]any{
			"verification_plan_id": "vp-" + taskID,
			"criteria": []any{
				map[string]any{
					"criterion_id": "ac-1",
					"statement":    "tests pass",
					"evidence_requirements": []any{
						map[string]any{
							"requirement_id": "req-1",
							"procedure":      "ci.test",
						},
					},
				},
			},
		},
	}, http.StatusCreated)
	var response struct {
		Digest string `json:"digest"`
	}
	mustJSON(t, body, &response)
	if response.Digest == "" {
		t.Fatal("expected task digest")
	}
	return response.Digest
}

func createCompletedDelivery(t *testing.T, h http.Handler, suffix string) core.DeliveryReceipt {
	t.Helper()
	workID := "work-" + suffix
	taskID := "task-" + suffix
	runID := "run-" + suffix
	deliveryID := "delivery-" + suffix
	taskDigest := createWorkAndTask(t, h, workID, taskID, "FEATURE", "driver")

	mustRequest(t, h, http.MethodPost, "/api/v1/runs", map[string]any{
		"run_id":               runID,
		"task_contract_digest": taskDigest,
		"attempt_id":           "attempt-" + suffix,
		"run_input": map[string]any{
			"runtime_profile": "codex/default",
			"tool_profile":    "tools/m1",
			"worker_profile":  "worker/ubuntu",
			"policy_profile":  "policy/m1",
		},
	}, http.StatusCreated)
	mustRequest(t, h, http.MethodPost, "/api/v1/runs/"+runID+"/complete", map[string]any{
		"execution_epoch": 1,
	}, http.StatusOK)
	body := mustRequest(t, h, http.MethodPost, "/api/v1/deliveries", map[string]any{
		"delivery_receipt_id": deliveryID,
		"run_id":              runID,
		"result_commit":       "1111111111111111111111111111111111111111",
	}, http.StatusCreated)
	var delivery core.DeliveryReceipt
	mustJSON(t, body, &delivery)
	return delivery
}

func mustRequest(t *testing.T, h http.Handler, method, path string, body any, want int) []byte {
	t.Helper()
	var data []byte
	var err error
	if body != nil {
		data, err = json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(data))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != want {
		t.Fatalf("%s %s: got %d want %d: %s", method, path, rec.Code, want, rec.Body.String())
	}
	return rec.Body.Bytes()
}

func mustJSON(t *testing.T, body []byte, dst any) {
	t.Helper()
	if err := json.Unmarshal(body, dst); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, body)
	}
}

func TestCheckpointBindsFrozenRunInputAndRejectsStaleEpoch(t *testing.T) {
	s := NewServer(store.NewMemory())
	h := s.Handler()

	taskDigest := createWorkAndTask(t, h, "work-cp", "task-cp", "FEATURE", "driver")
	runBody := mustRequest(t, h, http.MethodPost, "/api/v1/runs", map[string]any{
		"run_id":               "run-cp",
		"task_contract_digest": taskDigest,
		"attempt_id":           "attempt-cp",
		"run_input": map[string]any{
			"runtime_profile": "codex/default",
			"tool_profile":    "tools/m1",
			"worker_profile":  "worker/ubuntu",
			"policy_profile":  "policy/m1",
			"context_refs":    []core.ContextRef{{Source: "doc:datasheet", Type: "DOCUMENT", Version: "r1", Digest: canonical.BytesDigest([]byte("datasheet")), Trust: core.ContextApproved}},
		},
	}, http.StatusCreated)
	var runResponse struct {
		Run struct {
			RunInputManifestDigest string `json:"run_input_manifest_digest"`
		} `json:"run"`
	}
	mustJSON(t, runBody, &runResponse)
	if runResponse.Run.RunInputManifestDigest == "" {
		t.Fatal("expected frozen run input manifest digest")
	}

	cpBody := mustRequest(t, h, http.MethodPost, "/api/v1/runs/run-cp/checkpoints", map[string]any{
		"checkpoint_id":             "cp-1",
		"execution_epoch":           1,
		"source_tree_digest":        "sha256:tree",
		"diff_digest":               "sha256:diff",
		"objective":                 "finish driver fix",
		"completed":                 []string{"analysis"},
		"pending":                   []string{"test"},
		"last_event_sequence":       5,
		"external_operation_cursor": "op-2",
	}, http.StatusCreated)
	var cpResponse struct {
		Digest     string `json:"digest"`
		Checkpoint struct {
			TaskContractDigest     string `json:"task_contract_digest"`
			RunInputManifestDigest string `json:"run_input_manifest_digest"`
			ExecutionEpoch         uint64 `json:"execution_epoch"`
		} `json:"checkpoint"`
	}
	mustJSON(t, cpBody, &cpResponse)
	if cpResponse.Digest == "" ||
		cpResponse.Checkpoint.TaskContractDigest != taskDigest ||
		cpResponse.Checkpoint.RunInputManifestDigest != runResponse.Run.RunInputManifestDigest ||
		cpResponse.Checkpoint.ExecutionEpoch != 1 {
		t.Fatalf("unexpected checkpoint binding: %#v", cpResponse)
	}

	mustRequest(t, h, http.MethodPost, "/api/v1/runs/run-cp/takeover", map[string]any{
		"execution_epoch": 1,
	}, http.StatusOK)

	mustRequest(t, h, http.MethodPost, "/api/v1/runs/run-cp/checkpoints", map[string]any{
		"checkpoint_id":      "cp-stale",
		"execution_epoch":    1,
		"source_tree_digest": "sha256:tree2",
	}, http.StatusConflict)
}

func TestRecoveryLifecycleIsEpochBound(t *testing.T) {
	s := NewServer(store.NewMemory())
	h := s.Handler()

	initialBody := mustRequest(t, h, http.MethodGet, "/api/v1/recovery", nil, http.StatusOK)
	var initial struct {
		Epoch uint64 `json:"recovery_epoch"`
		Mode  string `json:"mode"`
	}
	mustJSON(t, initialBody, &initial)
	if initial.Epoch != 0 || initial.Mode != "NORMAL" {
		t.Fatalf("unexpected initial recovery state: %#v", initial)
	}

	beginBody := mustRequest(t, h, http.MethodPost, "/api/v1/recovery/begin", map[string]any{
		"expected_recovery_epoch": 0,
	}, http.StatusOK)
	var active struct {
		Epoch uint64 `json:"recovery_epoch"`
		Mode  string `json:"mode"`
	}
	mustJSON(t, beginBody, &active)
	if active.Epoch != 1 || active.Mode != "RECOVERY_RECONCILIATION" {
		t.Fatalf("unexpected active recovery state: %#v", active)
	}

	mustRequest(t, h, http.MethodPost, "/api/v1/recovery/begin", map[string]any{
		"expected_recovery_epoch": 0,
	}, http.StatusConflict)

	mustRequest(t, h, http.MethodPost, "/api/v1/recovery/complete", map[string]any{
		"recovery_epoch": 1,
		"reconciled":     false,
	}, http.StatusUnprocessableEntity)

	completeBody := mustRequest(t, h, http.MethodPost, "/api/v1/recovery/complete", map[string]any{
		"recovery_epoch": 1,
		"reconciled":     true,
	}, http.StatusOK)
	var normal struct {
		Epoch uint64 `json:"recovery_epoch"`
		Mode  string `json:"mode"`
	}
	mustJSON(t, completeBody, &normal)
	if normal.Epoch != 1 || normal.Mode != "NORMAL" {
		t.Fatalf("unexpected completed recovery state: %#v", normal)
	}

	mustRequest(t, h, http.MethodPost, "/api/v1/recovery/complete", map[string]any{
		"recovery_epoch": 0,
		"reconciled":     true,
	}, http.StatusBadRequest)
}

func TestSupersededTaskRevisionCannotStartRun(t *testing.T) {
	s := NewServer(store.NewMemory())
	h := s.Handler()

	oldDigest := createWorkAndTask(t, h, "work-rev", "task-rev", "FEATURE", "driver")

	newTaskBody := mustRequest(t, h, http.MethodPost, "/api/v1/task-contracts", map[string]any{
		"contract": map[string]any{
			"task_contract_id": "task-rev",
			"work_item_id":     "work-rev",
			"task_type":        "FEATURE",
			"revision":         2,
		},
		"material": map[string]any{
			"repository":          "repo",
			"base_commit":         "0123456789abcdef0123456789abcdef01234567",
			"target_id":           "target-1",
			"acceptance_criteria": []string{"tests pass"},
		},
		"subsystem": "driver",
		"verification_plan": map[string]any{
			"verification_plan_id": "vp-task-rev-2",
			"criteria": []any{
				map[string]any{
					"criterion_id": "ac-1",
					"statement":    "tests pass",
					"evidence_requirements": []any{
						map[string]any{
							"requirement_id": "req-1",
							"procedure":      "ci.test",
						},
					},
				},
			},
		},
	}, http.StatusCreated)
	var newTask struct {
		Digest string `json:"digest"`
	}
	mustJSON(t, newTaskBody, &newTask)
	if newTask.Digest == "" || newTask.Digest == oldDigest {
		t.Fatalf("expected a new active task digest old=%s new=%s", oldDigest, newTask.Digest)
	}

	runInput := map[string]any{
		"runtime_profile": "codex/default",
		"tool_profile":    "tools/m1",
		"worker_profile":  "worker/ubuntu",
		"policy_profile":  "policy/m1",
	}
	mustRequest(t, h, http.MethodPost, "/api/v1/runs", map[string]any{
		"run_id":               "run-old-rev",
		"task_contract_digest": oldDigest,
		"attempt_id":           "attempt-old",
		"run_input":            runInput,
	}, http.StatusConflict)

	mustRequest(t, h, http.MethodPost, "/api/v1/runs", map[string]any{
		"run_id":               "run-new-rev",
		"task_contract_digest": newTask.Digest,
		"attempt_id":           "attempt-new",
		"run_input":            runInput,
	}, http.StatusCreated)
}

type failingRecoveryStore struct {
	*store.Memory
	err error
}

func (s failingRecoveryStore) GetRecovery() (recovery.Manager, error) {
	return recovery.Manager{}, s.err
}

func TestRecoveryReadFailureReturnsServiceUnavailable(t *testing.T) {
	expected := errors.New("database unavailable")
	s := NewServer(failingRecoveryStore{Memory: store.NewMemory(), err: expected})
	mustRequest(t, s.Handler(), http.MethodGet, "/api/v1/recovery", nil, http.StatusServiceUnavailable)
}

func TestDuplicateSteeringIDCannotOverwriteHistory(t *testing.T) {
	s := NewServer(store.NewMemory())
	h := s.Handler()

	taskDigest := createWorkAndTask(t, h, "work-steer-id", "task-steer-id", "FEATURE", "driver")
	mustRequest(t, h, http.MethodPost, "/api/v1/runs", map[string]any{
		"run_id":               "run-steer-id",
		"task_contract_digest": taskDigest,
		"attempt_id":           "attempt-steer-id",
		"run_input": map[string]any{
			"runtime_profile": "codex/default",
			"tool_profile":    "tools/m1",
			"worker_profile":  "worker/ubuntu",
			"policy_profile":  "policy/m1",
		},
	}, http.StatusCreated)

	mustRequest(t, h, http.MethodPost, "/api/v1/runs/run-steer-id/steer", map[string]any{
		"steering_command_id": "steer-fixed",
		"execution_epoch":     1,
		"sequence":            1,
		"actor":               "engineer",
		"content_digest":      "sha256:first",
	}, http.StatusAccepted)

	mustRequest(t, h, http.MethodPost, "/api/v1/runs/run-steer-id/steer", map[string]any{
		"steering_command_id": "steer-fixed",
		"execution_epoch":     1,
		"sequence":            2,
		"actor":               "engineer",
		"content_digest":      "sha256:second",
	}, http.StatusConflict)

	body := mustRequest(t, h, http.MethodGet, "/api/v1/steering/steer-fixed", nil, http.StatusOK)
	var stored struct {
		Sequence      uint64 `json:"sequence"`
		ContentDigest string `json:"content_digest"`
	}
	mustJSON(t, body, &stored)
	if stored.Sequence != 1 || stored.ContentDigest != "sha256:first" {
		t.Fatalf("historical steering command was overwritten: %#v", stored)
	}
}

func TestEvidenceMustBindExactFrozenRequirement(t *testing.T) {
	s := NewServer(store.NewMemory())
	h := s.Handler()
	delivery := createCompletedDelivery(t, h, "requirement")

	mustRequest(t, h, http.MethodPost, "/api/v1/evidence", map[string]any{
		"delivery_receipt_id": delivery.ID,
		"evidence": map[string]any{
			"evidence_id":    "ev-missing-requirement",
			"requirement_id": "req-missing",
			"issuer":         "ci",
			"procedure":      "ci.test",
			"result":         "PASS",
			"applicable":     true,
		},
	}, http.StatusUnprocessableEntity)

	mustRequest(t, h, http.MethodPost, "/api/v1/evidence", map[string]any{
		"delivery_receipt_id": delivery.ID,
		"evidence": map[string]any{
			"evidence_id":    "ev-wrong-procedure",
			"requirement_id": "req-1",
			"issuer":         "ci",
			"procedure":      "other.test",
			"result":         "PASS",
			"applicable":     true,
		},
	}, http.StatusUnprocessableEntity)

	mustRequest(t, h, http.MethodPost, "/api/v1/evidence", map[string]any{
		"delivery_receipt_id": delivery.ID,
		"evidence": map[string]any{
			"evidence_id": "ev-no-requirement",
			"issuer":      "ci",
			"procedure":   "ci.test",
			"result":      "PASS",
			"applicable":  true,
		},
	}, http.StatusBadRequest)

	mustRequest(t, h, http.MethodPost, "/api/v1/evidence", map[string]any{
		"delivery_receipt_id": delivery.ID,
		"evidence": map[string]any{
			"evidence_id":    "ev-foreign-artifact",
			"requirement_id": "req-1",
			"issuer":         "ci",
			"procedure":      "ci.test",
			"result":         "PASS",
			"artifact_refs":  []string{"not-in-delivery"},
			"applicable":     true,
		},
	}, http.StatusUnprocessableEntity)
}
