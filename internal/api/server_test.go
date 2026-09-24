package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jiying2007/engineering-platform/internal/core"
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
			"evidence_id": "ev-f",
			"issuer":      "ci",
			"procedure":   "ci.test",
			"result":      "PASS",
			"applicable":  true,
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

	mustRequest(t, h, http.MethodPost, "/api/v1/closures", map[string]any{
		"closure_receipt_id":     "closure-f",
		"delivery_receipt_id":    "delivery-f",
		"verification_report_id": "vr-f",
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
			"evidence_id": "ev-a",
			"issuer":      "ci",
			"procedure":   "ci.test",
			"result":      "PASS",
			"applicable":  true,
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
			"evidence_id": "ev-fail",
			"issuer":      "ci",
			"procedure":   "ci.test",
			"result":      "FAIL",
			"applicable":  true,
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
