package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jiying2007/engineering-platform/internal/recovery"
	"github.com/jiying2007/engineering-platform/internal/store"
)

type recoveryContextProbe struct {
	*store.Memory
	reads     int
	begins    int
	completes int
}

func (p *recoveryContextProbe) GetRecoveryContext(ctx context.Context) (recovery.Manager, error) {
	p.reads++
	if err := ctx.Err(); err != nil {
		return recovery.Manager{}, err
	}
	return p.Memory.GetRecovery()
}

func (p *recoveryContextProbe) BeginRecoveryContext(ctx context.Context, epoch uint64) (recovery.Manager, error) {
	p.begins++
	if err := ctx.Err(); err != nil {
		return recovery.Manager{}, err
	}
	return p.Memory.BeginRecovery(epoch)
}

func (p *recoveryContextProbe) CompleteRecoveryContext(ctx context.Context, epoch uint64, reconciled bool) (recovery.Manager, error) {
	p.completes++
	if err := ctx.Err(); err != nil {
		return recovery.Manager{}, err
	}
	return p.Memory.CompleteRecovery(epoch, reconciled)
}

func TestRecoveryLifecycleHTTPUsesCallerBoundCoreAuthority(t *testing.T) {
	probe := &recoveryContextProbe{Memory: store.NewMemory()}
	handler := NewServer(probe).Handler()
	mustRequest(t, handler, http.MethodGet, "/api/v1/recovery", nil, http.StatusOK)
	cancelled := func(path string, body map[string]any) {
		t.Helper()
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(data)).WithContext(ctx)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code == http.StatusOK {
			t.Fatalf("canceled recovery operation %s succeeded", path)
		}
	}
	cancelled("/api/v1/recovery/begin", map[string]any{"expected_recovery_epoch": uint64(0)})
	if probe.begins != 0 {
		t.Fatal("canceled Recovery Begin reached mutable Core authority")
	}
	mustRequest(t, handler, http.MethodPost, "/api/v1/recovery/begin", map[string]any{"expected_recovery_epoch": uint64(0)}, http.StatusOK)
	cancelled("/api/v1/recovery/complete", map[string]any{"recovery_epoch": uint64(1)})
	if probe.completes != 0 {
		t.Fatal("canceled Recovery Complete reached mutable Core authority")
	}
	state, err := probe.Memory.GetRecovery()
	if err != nil || state.Epoch != 1 || state.Mode != recovery.RecoveryReconciliation {
		t.Fatalf("cancelled completion changed recovery mode: state=%+v err=%v", state, err)
	}
	mustRequest(t, handler, http.MethodPost, "/api/v1/recovery/complete", map[string]any{"recovery_epoch": uint64(1)}, http.StatusOK)
	mustRequest(t, handler, http.MethodGet, "/api/v1/recovery", nil, http.StatusOK)
	if probe.reads != 2 || probe.begins != 1 || probe.completes != 1 {
		t.Fatalf("Recovery HTTP bypassed request Context capability: %+v", probe)
	}
}
