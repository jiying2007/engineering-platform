package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/core"
	"github.com/jiying2007/engineering-platform/internal/material"
	"github.com/jiying2007/engineering-platform/internal/production"
	"github.com/jiying2007/engineering-platform/internal/run"
	"github.com/jiying2007/engineering-platform/internal/store"
)

func TestProductionTerminalMaintenanceTemplatesDryRunToRunning(t *testing.T) {
	const (
		baseCommit    = "0123456789abcdef0123456789abcdef01234567"
		profileDigest = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		workerProfile = "worker/codex-production"
		humanOwner    = "urn:engineering-platform:operator:production-owner"
		contextDigest = "sha256:01edbdf525937b5cbd5464fc1c3b9e6b4b677af14b5bb802a137fbd70bfd0445"
	)

	plan, err := production.BuildTerminalPlan()
	if err != nil {
		t.Fatal(err)
	}
	markerPath := filepath.Join("..", "..", plan.Plan.MarkerPath)
	marker, err := os.ReadFile(markerPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(marker) != plan.Plan.MarkerBefore {
		t.Fatalf("terminal marker bytes drifted: %q", marker)
	}

	requirementPath := filepath.Join("..", "..", "examples", "production", "terminal-maintenance", "requirement.md")
	requirement, err := os.ReadFile(requirementPath)
	if err != nil {
		t.Fatal(err)
	}
	if digest := canonical.BytesDigest(requirement); digest != contextDigest {
		t.Fatalf("terminal requirement digest %s want %s", digest, contextDigest)
	}

	server := NewServer(store.NewMemory())
	h := server.Handler()

	work := decodeProductionTerminalTemplate(t, "work.json.tmpl", map[string]string{
		"__HUMAN_OWNER__": humanOwner,
	})
	mustRequest(t, h, http.MethodPost, "/api/v1/work-items", work, http.StatusCreated)

	task := decodeProductionTerminalTemplate(t, "task.json.tmpl", map[string]string{
		"__BASE_COMMIT__": baseCommit,
	})
	taskBody := mustRequest(t, h, http.MethodPost, "/api/v1/task-contracts", task, http.StatusCreated)
	var taskResponse struct {
		Contract  core.TaskContract `json:"contract"`
		Digest    string            `json:"digest"`
		Readiness material.Result   `json:"readiness"`
	}
	mustJSON(t, taskBody, &taskResponse)
	if taskResponse.Digest == "" || taskResponse.Readiness.Status != material.Ready ||
		taskResponse.Contract.TaskType != "RELEASE" ||
		taskResponse.Contract.Repository != plan.Plan.Repository ||
		taskResponse.Contract.TargetID != plan.Plan.TargetID ||
		len(taskResponse.Contract.AllowedActions) != 2 ||
		taskResponse.Contract.AllowedActions[0] != "worker.codex-execute" ||
		taskResponse.Contract.AllowedActions[1] != "github.publish-pr" {
		t.Fatalf("unexpected terminal Task: %#v", taskResponse)
	}

	runRequest := decodeProductionTerminalTemplate(t, "run.json.tmpl", map[string]string{
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
	if runResponse.Run.ID != "production-terminal-maintenance-run" ||
		runResponse.Run.State != run.Running ||
		runResponse.Run.CurrentEpoch != 1 ||
		runResponse.RunInput.ToolProfile != "codex/"+profileDigest ||
		runResponse.RunInput.WorkerProfile != workerProfile ||
		runResponse.RunInput.PolicyProfile != "policy/production-terminal-v1" ||
		len(runResponse.RunInput.ContextRefs) != 1 ||
		runResponse.RunInput.ContextRefs[0].Digest != contextDigest ||
		runResponse.RunInput.ContextRefs[0].Trust != core.ContextApproved {
		t.Fatalf("unexpected terminal Run: %#v input=%#v", runResponse.Run, runResponse.RunInput)
	}

	storedBody := mustRequest(t, h, http.MethodGet, "/api/v1/work-items/production-terminal-maintenance-work", nil, http.StatusOK)
	var stored core.WorkItem
	mustJSON(t, storedBody, &stored)
	if stored.State != core.WorkExecuting ||
		stored.ActiveRunID != runResponse.Run.ID ||
		stored.ActiveTaskContractDigest != taskResponse.Digest {
		t.Fatalf("unexpected terminal Work state: %#v", stored)
	}
}

func decodeProductionTerminalTemplate(t *testing.T, name string, replacements map[string]string) map[string]any {
	t.Helper()
	path := filepath.Join("..", "..", "examples", "production", "terminal-maintenance", name)
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
		t.Fatalf("invalid rendered production terminal JSON %s: %v", path, err)
	}
	return value
}
