package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jiying2007/engineering-platform/internal/core"
	"github.com/jiying2007/engineering-platform/internal/store"
	"github.com/jiying2007/engineering-platform/internal/verification"
)

type intakeContextProbe struct {
	*store.Memory
	createWorkCalls int
	readWorkCalls   int
	freezeTaskCalls int
	readTaskCalls   int
}

func (p *intakeContextProbe) CreateWorkContext(ctx context.Context, item core.WorkItem) error {
	p.createWorkCalls++
	if err := ctx.Err(); err != nil {
		return err
	}
	return p.Memory.CreateWork(item)
}
func (p *intakeContextProbe) GetWorkContext(ctx context.Context, id string) (core.WorkItem, error) {
	p.readWorkCalls++
	if err := ctx.Err(); err != nil {
		return core.WorkItem{}, err
	}
	return p.Memory.GetWork(id)
}
func (p *intakeContextProbe) UpdateWorkContext(ctx context.Context, id string, version uint64, item core.WorkItem) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return p.Memory.UpdateWork(id, version, item)
}
func (p *intakeContextProbe) CreateTaskAndUpdateWorkContext(ctx context.Context, task core.TaskContract, plan verification.Plan, version uint64, work core.WorkItem) error {
	p.freezeTaskCalls++
	if err := ctx.Err(); err != nil {
		return err
	}
	return p.Memory.CreateTaskAndUpdateWork(task, plan, version, work)
}
func (p *intakeContextProbe) GetTaskContext(ctx context.Context, id string) (core.TaskContract, error) {
	p.readTaskCalls++
	if err := ctx.Err(); err != nil {
		return core.TaskContract{}, err
	}
	return p.Memory.GetTask(id)
}

func TestWorkTaskHTTPRoutesUseContextualCoreStoreWhenAvailable(t *testing.T) {
	p := &intakeContextProbe{Memory: store.NewMemory()}
	h := NewServer(p).Handler()
	_ = createWorkAndTask(t, h, "intake-work", "intake-task", "FEATURE", "driver")
	mustRequest(t, h, http.MethodGet, "/api/v1/work-items/intake-work", nil, http.StatusOK)
	mustRequest(t, h, http.MethodGet, "/api/v1/task-contracts/intake-task", nil, http.StatusOK)
	if p.createWorkCalls != 1 || p.readWorkCalls != 2 ||
		p.freezeTaskCalls != 1 || p.readTaskCalls != 1 {
		t.Fatalf("HTTP routed around Context-aware Core authority: %#v", p)
	}
}

func TestCancelledWorkHTTPCallCannotCreateCoreRecord(t *testing.T) {
	p := &intakeContextProbe{Memory: store.NewMemory()}
	h := NewServer(p).Handler()
	raw, err := json.Marshal(map[string]any{
		"work_item_id": "cancelled-work-http",
		"title":        "cancelled intake",
		"human_owner":  "engineer",
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/work-items", bytes.NewReader(raw)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	h.ServeHTTP(response, req)
	if response.Code == http.StatusCreated || p.createWorkCalls != 0 {
		t.Fatalf("cancelled HTTP request created WorkItem: status=%d calls=%d", response.Code, p.createWorkCalls)
	}
	if _, err := p.Memory.GetWork("cancelled-work-http"); err != store.ErrNotFound {
		t.Fatalf("cancelled request mutated authoritative Core state: %v", err)
	}
}
