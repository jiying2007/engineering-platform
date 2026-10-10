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

type runControlContextProbe struct {
	*runStartContextProbe
	controlWrites  int
	completeWrites int
}

func (p *runControlContextProbe) UpdateExecutionContext(ctx context.Context, id string, version uint64, value run.Run, sess session.Session) error {
	p.controlWrites++
	if err := ctx.Err(); err != nil {
		return err
	}
	return p.Memory.UpdateExecution(id, version, value, sess)
}

func (p *runControlContextProbe) UpdateExecutionAndWorkContext(ctx context.Context, id string, runVersion uint64, value run.Run, sess session.Session, workVersion uint64, work core.WorkItem) error {
	p.completeWrites++
	if err := ctx.Err(); err != nil {
		return err
	}
	return p.Memory.UpdateExecutionAndWork(id, runVersion, value, sess, workVersion, work)
}

func TestRunControlHTTPRoutesUseRequestBoundCoreAuthority(t *testing.T) {
	p := &runControlContextProbe{
		runStartContextProbe: &runStartContextProbe{Memory: store.NewMemory()},
	}
	h := NewServer(p).Handler()
	taskDigest := createWorkAndTask(t, h, "work-run-control", "task-run-control", "FEATURE", "driver")
	mustRequest(t, h, http.MethodPost, "/api/v1/runs",
		runStartBody("run-control", taskDigest), http.StatusCreated)
	mustRequest(t, h, http.MethodPost, "/api/v1/runs/run-control/pause",
		map[string]any{"execution_epoch": 1}, http.StatusOK)
	mustRequest(t, h, http.MethodPost, "/api/v1/runs/run-control/resume",
		map[string]any{"execution_epoch": 1}, http.StatusOK)
	mustRequest(t, h, http.MethodPost, "/api/v1/runs/run-control/takeover",
		map[string]any{"execution_epoch": 1}, http.StatusOK)
	if p.controlWrites != 3 {
		t.Fatalf("Pause/Resume/Takeover bypassed caller-bound authority: %d writes", p.controlWrites)
	}

	secondDigest := createWorkAndTask(t, h, "work-run-complete", "task-run-complete", "FEATURE", "driver")
	mustRequest(t, h, http.MethodPost, "/api/v1/runs",
		runStartBody("run-complete", secondDigest), http.StatusCreated)
	mustRequest(t, h, http.MethodPost, "/api/v1/runs/run-complete/complete",
		map[string]any{"execution_epoch": 1}, http.StatusOK)
	if p.completeWrites != 1 {
		t.Fatalf("Run completion bypassed atomic request-bound Run/Work authority: %d", p.completeWrites)
	}
	work, err := p.Memory.GetWork("work-run-complete")
	if err != nil || work.State != core.WorkVerifying {
		t.Fatalf("Run completion failed to advance Work atomically: %#v err=%v", work, err)
	}
}

func TestCancelledRunControlHTTPRequestCannotMutateRunOrSession(t *testing.T) {
	p := &runControlContextProbe{
		runStartContextProbe: &runStartContextProbe{Memory: store.NewMemory()},
	}
	h := NewServer(p).Handler()
	digest := createWorkAndTask(t, h, "work-cancel-control", "task-cancel-control", "FEATURE", "driver")
	mustRequest(t, h, http.MethodPost, "/api/v1/runs",
		runStartBody("run-cancel-control", digest), http.StatusCreated)

	raw, err := json.Marshal(map[string]any{"execution_epoch": 1})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/runs/run-cancel-control/pause",
		bytes.NewReader(raw)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	h.ServeHTTP(response, req)
	if response.Code == http.StatusOK || p.controlWrites != 0 {
		t.Fatalf("cancelled Run control crossed Core authority: status=%d writes=%d",
			response.Code, p.controlWrites)
	}
	value, sess, err := p.Memory.GetExecution("run-cancel-control")
	if err != nil || value.State != run.Running || sess.Paused {
		t.Fatalf("cancelled control changed Run/Session state: %#v %#v err=%v",
			value, sess, err)
	}
}
