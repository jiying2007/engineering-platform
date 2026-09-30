package api

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/production"
	"github.com/jiying2007/engineering-platform/internal/store"
)

type operationalMemory struct {
	*store.Memory
	status production.OperationalStatus
	err    error
}

func (m *operationalMemory) ReadOperationalStatus(context.Context) (production.OperationalStatus, error) {
	return m.status, m.err
}

func TestOperationalStatusEndpointUsesStoreAuthority(t *testing.T) {
	snapshot := production.Snapshot{
		Version:      production.OperationalStatusVersion,
		CapturedAt:   time.Unix(1700000000, 0).UTC(),
		RecoveryMode: "NORMAL",
	}
	status, err := production.EvaluateSnapshot(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	h := NewServer(&operationalMemory{Memory: store.NewMemory(), status: status}).Handler()
	body := mustRequest(t, h, http.MethodGet, "/api/v1/operations/status", nil, http.StatusOK)
	var got production.OperationalStatus
	mustJSON(t, body, &got)
	if !got.Ready || got.State != status.State || got.Snapshot != status.Snapshot || len(got.Reasons) != 0 {
		t.Fatalf("unexpected operational status: %#v", got)
	}
}

func TestOperationalStatusEndpointFailsClosedWithoutAuthorityReader(t *testing.T) {
	h := NewServer(store.NewMemory()).Handler()
	mustRequest(t, h, http.MethodGet, "/api/v1/operations/status", nil, http.StatusServiceUnavailable)
}
