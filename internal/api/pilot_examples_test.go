package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jiying2007/engineering-platform/internal/core"
	"github.com/jiying2007/engineering-platform/internal/material"
	"github.com/jiying2007/engineering-platform/internal/run"
	"github.com/jiying2007/engineering-platform/internal/store"
)

func TestRetainedPilotTemplatesDryRunToRunning(t *testing.T) {
	const (
		baseCommit    = "0123456789abcdef0123456789abcdef01234567"
		profileDigest = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		workerProfile = "worker/codex-pilot"
		humanOwner    = "urn:engineering-platform:operator:pilot-owner"
	)

	for _, pilot := range []struct {
		name      string
		dir       string
		workID    string
		taskID    string
		runID     string
		wantType  string
		sourceRef string
	}{
		{
			name:      "feature",
			dir:       "feature-routing",
			workID:    "m1-feature-routing-work",
			taskID:    "m1-feature-routing-task",
			runID:     "m1-feature-routing-run",
			wantType:  "FEATURE",
			sourceRef: "https://github.com/jiying2007/engineering-platform/issues/54",
		},
		{
			name:      "debug",
			dir:       "debug-firmware-identity",
			workID:    "m1-debug-firmware-identity-work",
			taskID:    "m1-debug-firmware-identity-task",
			runID:     "m1-debug-firmware-identity-run",
			wantType:  "DEBUG",
			sourceRef: "https://github.com/jiying2007/engineering-platform/issues/55",
		},
	} {
		t.Run(pilot.name, func(t *testing.T) {
			server := NewServer(store.NewMemory())
			h := server.Handler()

			work := decodePilotTemplate(t, pilot.dir, "work.json.tmpl", map[string]string{
				"__HUMAN_OWNER__": humanOwner,
			})
			mustRequest(t, h, http.MethodPost, "/api/v1/work-items", work, http.StatusCreated)

			task := decodePilotTemplate(t, pilot.dir, "task.json.tmpl", map[string]string{
				"__BASE_COMMIT__": baseCommit,
			})
			taskBody := mustRequest(t, h, http.MethodPost, "/api/v1/task-contracts", task, http.StatusCreated)
			var taskResponse struct {
				Contract  core.TaskContract `json:"contract"`
				Digest    string            `json:"digest"`
				Readiness material.Result   `json:"readiness"`
			}
			mustJSON(t, taskBody, &taskResponse)
			if taskResponse.Digest == "" || taskResponse.Readiness.Status != material.Ready {
				t.Fatalf("pilot Task did not become READY: %#v", taskResponse)
			}
			if taskResponse.Contract.ID != pilot.taskID || taskResponse.Contract.TaskType != pilot.wantType ||
				taskResponse.Contract.BaseCommit != baseCommit ||
				len(taskResponse.Contract.AllowedActions) != 2 ||
				taskResponse.Contract.AllowedActions[0] != "worker.codex-execute" ||
				taskResponse.Contract.AllowedActions[1] != "github.publish-pr" {
				t.Fatalf("unexpected frozen Task contract: %#v", taskResponse.Contract)
			}

			runRequest := decodePilotTemplate(t, pilot.dir, "run.json.tmpl", map[string]string{
				"__TASK_DIGEST__":    taskResponse.Digest,
				"__PROFILE_DIGEST__": profileDigest,
				"__WORKER_PROFILE__": workerProfile,
			})
			runBody := mustRequest(t, h, http.MethodPost, "/api/v1/runs", runRequest, http.StatusCreated)
			var runResponse struct {
				Run      run.Run               `json:"run"`
				RunInput core.RunInputManifest `json:"run_input"`
			}
			mustJSON(t, runBody, &runResponse)
			if runResponse.Run.ID != pilot.runID || runResponse.Run.State != run.Running ||
				runResponse.Run.CurrentEpoch != 1 ||
				runResponse.Run.TaskContractDigest != taskResponse.Digest ||
				runResponse.RunInput.ToolProfile != "codex/"+profileDigest ||
				runResponse.RunInput.WorkerProfile != workerProfile {
				t.Fatalf("unexpected pilot Run: %#v input=%#v", runResponse.Run, runResponse.RunInput)
			}

			workBody := mustRequest(t, h, http.MethodGet, "/api/v1/work-items/"+pilot.workID, nil, http.StatusOK)
			var storedWork core.WorkItem
			mustJSON(t, workBody, &storedWork)
			if storedWork.SourceRef != pilot.sourceRef || storedWork.HumanOwner != humanOwner ||
				storedWork.State != core.WorkExecuting ||
				storedWork.ActiveRunID != pilot.runID ||
				storedWork.ActiveTaskContractDigest != taskResponse.Digest {
				t.Fatalf("unexpected pilot Work identity: %#v", storedWork)
			}
		})
	}
}

func decodePilotTemplate(t *testing.T, pilotDir, name string, replacements map[string]string) map[string]any {
	t.Helper()
	path := filepath.Join("..", "..", "examples", "pilots", pilotDir, name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for old, value := range replacements {
		text = strings.ReplaceAll(text, old, value)
	}
	if strings.Contains(text, "__") {
		t.Fatalf("unrendered placeholder in %s", path)
	}
	var value map[string]any
	if err := json.Unmarshal([]byte(text), &value); err != nil {
		t.Fatalf("invalid rendered pilot JSON %s: %v", path, err)
	}
	return value
}
