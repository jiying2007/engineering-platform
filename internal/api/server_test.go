package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jiying2007/engineering-platform/internal/store"
)

func TestWorkTaskRunVerticalSlice(t *testing.T) {
	s := NewServer(store.NewMemory())
	handler := s.Handler()

	work := map[string]any{
		"work_item_id": "work-1",
		"title":        "Add MCU telemetry",
		"human_owner":  "owner",
	}
	mustRequest(t, handler, http.MethodPost, "/api/v1/work-items", work, http.StatusCreated)

	task := map[string]any{
		"contract": map[string]any{
			"task_contract_id": "task-1",
			"work_item_id":     "work-1",
			"task_type":        "FEATURE",
		},
		"material": map[string]any{
			"repository":          "jiying2007/example",
			"base_commit":         "0123456789abcdef0123456789abcdef01234567",
			"target_id":           "target-1",
			"acceptance_criteria": []string{"CI passes"},
		},
		"subsystem": "MCU driver",
	}
	mustRequest(t, handler, http.MethodPost, "/api/v1/task-contracts", task, http.StatusCreated)

	runReq := map[string]any{
		"run_id":           "run-1",
		"task_contract_id": "task-1",
		"attempt_id":       "attempt-1",
	}
	mustRequest(t, handler, http.MethodPost, "/api/v1/runs", runReq, http.StatusCreated)
	mustRequest(t, handler, http.MethodGet, "/api/v1/runs/run-1", nil, http.StatusOK)
}

func TestTaskCreationFailsClosedOnMaterial(t *testing.T) {
	s := NewServer(store.NewMemory())
	handler := s.Handler()

	mustRequest(t, handler, http.MethodPost, "/api/v1/work-items", map[string]any{
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
	mustRequest(t, handler, http.MethodPost, "/api/v1/task-contracts", task, http.StatusUnprocessableEntity)
}

func mustRequest(t *testing.T, h http.Handler, method, path string, body any, want int) {
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
}

func TestPauseSteerAndTakeover(t *testing.T) {
	s := NewServer(store.NewMemory())
	h := s.Handler()

	mustRequest(t, h, http.MethodPost, "/api/v1/work-items", map[string]any{
		"work_item_id": "work-s",
		"title":        "Interactive run",
		"human_owner":  "owner",
	}, http.StatusCreated)

	mustRequest(t, h, http.MethodPost, "/api/v1/task-contracts", map[string]any{
		"contract": map[string]any{
			"task_contract_id": "task-s",
			"work_item_id":     "work-s",
			"task_type":        "FEATURE",
		},
		"material": map[string]any{
			"repository":          "repo",
			"base_commit":         "0123456789abcdef0123456789abcdef01234567",
			"acceptance_criteria": []string{"CI passes"},
		},
		"subsystem": "driver",
	}, http.StatusCreated)

	mustRequest(t, h, http.MethodPost, "/api/v1/runs", map[string]any{
		"run_id":           "run-s",
		"task_contract_id": "task-s",
		"attempt_id":       "attempt-s",
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
