package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/session"
	"github.com/jiying2007/engineering-platform/internal/store"
)

type steeringContextProbe struct {
	*store.Memory
	steeringReads int
}

func (p *steeringContextProbe) GetSteeringContext(ctx context.Context, id string) (session.SteeringCommand, error) {
	p.steeringReads++
	if err := ctx.Err(); err != nil {
		return session.SteeringCommand{}, err
	}
	return p.Memory.GetSteering(id)
}

func TestSteeringReadHTTPUsesCallerContextWithoutChangingRecordedProvenance(t *testing.T) {
	probe := &steeringContextProbe{Memory: store.NewMemory()}
	handler := NewServer(probe).Handler()
	digest := createWorkAndTask(t, handler, "work-steer-context", "task-steer-context", "FEATURE", "driver")
	mustRequest(t, handler, http.MethodPost, "/api/v1/runs", map[string]any{
		"run_id":               "run-steer-context",
		"task_contract_digest": digest,
		"attempt_id":           "attempt-steer-context",
		"run_input": map[string]any{
			"runtime_profile": "codex/default",
			"tool_profile":    "tools/m1",
			"worker_profile":  "worker/ubuntu",
			"policy_profile":  "policy/m1",
		},
	}, http.StatusCreated)
	value, sess, err := probe.Memory.GetExecution("run-steer-context")
	if err != nil {
		t.Fatal(err)
	}
	cmd := session.SteeringCommand{
		ID:             "steer-context",
		RunID:          value.ID,
		ExecutionEpoch: value.CurrentEpoch,
		Sequence:       1,
		Actor:          "engineer",
		ContentDigest:  "sha256:steer-context",
		CreatedAt:      time.Now().UTC(),
	}
	if err := sess.ApplySteering(cmd); err != nil {
		t.Fatal(err)
	}
	if err := probe.Memory.RecordSteering(value.ID, value.Version, value, sess, cmd); err != nil {
		t.Fatal(err)
	}
	mustRequest(t, handler, http.MethodGet, "/api/v1/steering/"+cmd.ID, nil, http.StatusOK)
	if probe.steeringReads != 1 {
		t.Fatalf("Steering read bypassed caller-bound Core: %d", probe.steeringReads)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/steering/"+cmd.ID, nil).WithContext(ctx)
	out := httptest.NewRecorder()
	handler.ServeHTTP(out, req)
	if out.Code == http.StatusOK || probe.steeringReads != 1 {
		t.Fatalf("cancelled Steering read ignored caller context: status=%d reads=%d", out.Code, probe.steeringReads)
	}
}
