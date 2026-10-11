package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jiying2007/engineering-platform/internal/action"
	"github.com/jiying2007/engineering-platform/internal/store"
)

type actionReadContextProbe struct {
	ActionGateway
	reads int
}

func (p *actionReadContextProbe) GetContext(ctx context.Context, _ string) (action.Operation, error) {
	p.reads++
	if err := ctx.Err(); err != nil {
		return action.Operation{}, err
	}
	return action.Operation{}, action.ErrOperationAbsent
}

func TestActionGetHTTPUsesCallerBoundLedgerReadback(t *testing.T) {
	probe := &actionReadContextProbe{}
	handler := NewServerWithActionGateway(store.NewMemory(), probe).Handler()
	mustRequest(t, handler, http.MethodGet, "/api/v1/actions/not-found", nil, http.StatusNotFound)
	if probe.reads != 1 {
		t.Fatalf("GET bypassed request-scoped Action repository: %d", probe.reads)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/actions/not-found", nil).WithContext(ctx)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code == http.StatusOK || probe.reads != 1 {
		t.Fatalf("cancelled GET invoked ledger: status=%d calls=%d", response.Code, probe.reads)
	}
}
