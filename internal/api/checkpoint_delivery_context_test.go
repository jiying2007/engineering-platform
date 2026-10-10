package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jiying2007/engineering-platform/internal/core"
	"github.com/jiying2007/engineering-platform/internal/session"
	"github.com/jiying2007/engineering-platform/internal/store"
)

type checkpointDeliveryContextProbe struct {
	*store.Memory
	createCheckpointCalls int
	readCheckpointCalls   int
	createDeliveryCalls   int
	readDeliveryCalls     int
}

func (p *checkpointDeliveryContextProbe) CreateCheckpointContext(ctx context.Context, item session.Checkpoint) (string, error) {
	p.createCheckpointCalls++
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return p.Memory.CreateCheckpoint(item)
}
func (p *checkpointDeliveryContextProbe) GetCheckpointContext(ctx context.Context, id string) (session.Checkpoint, string, error) {
	p.readCheckpointCalls++
	if err := ctx.Err(); err != nil {
		return session.Checkpoint{}, "", err
	}
	return p.Memory.GetCheckpoint(id)
}
func (p *checkpointDeliveryContextProbe) CreateDeliveryContext(ctx context.Context, item core.DeliveryReceipt) error {
	p.createDeliveryCalls++
	if err := ctx.Err(); err != nil {
		return err
	}
	return p.Memory.CreateDelivery(item)
}
func (p *checkpointDeliveryContextProbe) GetDeliveryContext(ctx context.Context, id string) (core.DeliveryReceipt, error) {
	p.readDeliveryCalls++
	if err := ctx.Err(); err != nil {
		return core.DeliveryReceipt{}, err
	}
	return p.Memory.GetDelivery(id)
}

func TestCheckpointDeliveryHTTPRoutesUseRequestContext(t *testing.T) {
	probe := &checkpointDeliveryContextProbe{Memory: store.NewMemory()}
	handler := NewServer(probe).Handler()
	digest := createWorkAndTask(t, handler, "cd-work", "cd-task", "FEATURE", "driver")
	mustRequest(t, handler, http.MethodPost, "/api/v1/runs", map[string]any{
		"run_id": "cd-run",
		"task_contract_digest": digest,
		"attempt_id": "cd-attempt",
		"run_input": map[string]any{
			"runtime_profile": "codex/default",
			"tool_profile": "tools/m1",
			"worker_profile": "worker/ubuntu",
			"policy_profile": "policy/m1",
		},
	}, http.StatusCreated)

	mustRequest(t, handler, http.MethodPost, "/api/v1/runs/cd-run/checkpoints", map[string]any{
		"checkpoint_id": "cd-checkpoint",
		"execution_epoch": 1,
		"source_tree_digest": "sha256:tree",
	}, http.StatusCreated)
	mustRequest(t, handler, http.MethodGet, "/api/v1/checkpoints/cd-checkpoint", nil, http.StatusOK)
	mustRequest(t, handler, http.MethodPost, "/api/v1/runs/cd-run/complete", map[string]any{
		"execution_epoch": 1,
	}, http.StatusOK)
	mustRequest(t, handler, http.MethodPost, "/api/v1/deliveries", map[string]any{
		"delivery_receipt_id": "cd-delivery",
		"run_id": "cd-run",
		"result_commit": "0123456789abcdef0123456789abcdef01234567",
	}, http.StatusCreated)
	mustRequest(t, handler, http.MethodGet, "/api/v1/deliveries/cd-delivery", nil, http.StatusOK)
	if probe.createCheckpointCalls != 1 || probe.readCheckpointCalls != 1 ||
		probe.createDeliveryCalls != 1 || probe.readDeliveryCalls != 1 {
		t.Fatalf("HTTP bypassed Context-aware checkpoint/delivery Core store: %+v", probe)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	body, err := json.Marshal(map[string]any{
		"delivery_receipt_id": "must-not-create",
		"run_id": "cd-run",
		"result_commit": "1111111111111111111111111111111111111111",
	})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/deliveries", bytes.NewReader(body)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code == http.StatusCreated || probe.createDeliveryCalls != 1 {
		t.Fatalf("cancelled request created delivery: status=%d calls=%d", rec.Code, probe.createDeliveryCalls)
	}
	if _, err := probe.Memory.GetDelivery("must-not-create"); err != store.ErrNotFound {
		t.Fatalf("cancelled delivery mutated historical Core records: %v", err)
	}
}
