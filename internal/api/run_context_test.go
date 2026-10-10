package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jiying2007/engineering-platform/internal/core"
	"github.com/jiying2007/engineering-platform/internal/run"
	"github.com/jiying2007/engineering-platform/internal/session"
	"github.com/jiying2007/engineering-platform/internal/store"
)

type runStartContextProbe struct {
	*store.Memory
	frozenTaskReads int
	starts          int
	runReads        int
	inputReads      int
}

func (p *runStartContextProbe) GetTaskByDigestContext(ctx context.Context, digest string) (core.TaskContract, error) {
	p.frozenTaskReads++
	if err := ctx.Err(); err != nil {
		return core.TaskContract{}, err
	}
	return p.Memory.GetTaskByDigest(digest)
}

func (p *runStartContextProbe) CreateExecutionAndUpdateWorkContext(ctx context.Context, value run.Run, attempt run.Attempt, sess session.Session, input core.RunInputManifest, version uint64, work core.WorkItem) error {
	p.starts++
	if err := ctx.Err(); err != nil {
		return err
	}
	return p.Memory.CreateExecutionAndUpdateWork(value, attempt, sess, input, version, work)
}

func (p *runStartContextProbe) GetExecutionContext(ctx context.Context, id string) (run.Run, session.Session, error) {
	p.runReads++
	if err := ctx.Err(); err != nil {
		return run.Run{}, session.Session{}, err
	}
	return p.Memory.GetExecution(id)
}

func (p *runStartContextProbe) GetRunInputByDigestContext(ctx context.Context, digest string) (core.RunInputManifest, error) {
	p.inputReads++
	if err := ctx.Err(); err != nil {
		return core.RunInputManifest{}, err
	}
	return p.Memory.GetRunInputByDigest(digest)
}

func runStartBody(runID, taskDigest string) map[string]any {
	return map[string]any{
		"run_id":               runID,
		"task_contract_digest": taskDigest,
		"attempt_id":           "attempt-" + runID,
		"run_input": map[string]any{
			"runtime_profile": "codex/default",
			"tool_profile":    "tools/m1",
			"worker_profile":  "worker/ubuntu",
			"policy_profile":  "policy/m1",
		},
	}
}

func TestRunHTTPRoutesUseRequestScopedCoreStore(t *testing.T) {
	p := &runStartContextProbe{Memory: store.NewMemory()}
	h := NewServer(p).Handler()
	taskDigest := createWorkAndTask(t, h, "work-run-context", "task-run-context", "FEATURE", "driver")
	mustRequest(t, h, http.MethodPost, "/api/v1/runs",
		runStartBody("run-context", taskDigest), http.StatusCreated)
	mustRequest(t, h, http.MethodGet, "/api/v1/runs/run-context", nil, http.StatusOK)
	if p.frozenTaskReads != 1 || p.starts != 1 ||
		p.runReads != 1 || p.inputReads != 1 {
		t.Fatalf("Run HTTP request silently bypassed contextual Core Store: task=%d start=%d run=%d input=%d",
			p.frozenTaskReads, p.starts, p.runReads, p.inputReads)
	}
}

func TestCancelledRunHTTPDoesNotStartExecutionOrOutbox(t *testing.T) {
	p := &runStartContextProbe{Memory: store.NewMemory()}
	h := NewServer(p).Handler()
	taskDigest := createWorkAndTask(t, h, "work-cancel-run-context", "task-cancel-run-context", "FEATURE", "driver")
	raw, err := json.Marshal(runStartBody("cancelled-run", taskDigest))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/runs", bytes.NewReader(raw)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	h.ServeHTTP(response, req)
	if response.Code == http.StatusCreated || p.starts != 0 {
		t.Fatalf("canceled Run was created: status=%d calls=%d", response.Code, p.starts)
	}
	if _, _, err := p.Memory.GetExecution("cancelled-run"); err != store.ErrNotFound {
		t.Fatalf("canceled request created Run authority: %v", err)
	}
	if work, err := p.Memory.GetWork("work-cancel-run-context"); err != nil ||
		work.State != core.WorkReady || work.ActiveRunID != "" {
		t.Fatalf("canceled Run advanced Work: %#v err=%v", work, err)
	}
}
