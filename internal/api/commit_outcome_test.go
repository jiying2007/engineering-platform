package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jiying2007/engineering-platform/internal/core"
	"github.com/jiying2007/engineering-platform/internal/recovery"
	"github.com/jiying2007/engineering-platform/internal/store"
)

func TestUnknownCommitOutcomeAPINeverClaimsRollbackOrExposesSQL(t *testing.T) {
	for _, tc := range []struct {
		name string
		reply func(http.ResponseWriter, error)
	}{
		{name: "action", reply: writeActionError},
		{name: "mutation", reply: writeMutationError},
		{name: "readback", reply: writeStoreError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			tc.reply(rec, fmt.Errorf("private SQL/connection details: %w", store.ErrCommitOutcomeUnknown))
			if rec.Code != http.StatusServiceUnavailable ||
				!strings.Contains(rec.Body.String(), "commit_outcome_unknown") ||
				!strings.Contains(rec.Body.String(), "read back") ||
				strings.Contains(rec.Body.String(), "private SQL") {
				t.Fatalf("unknown COMMIT misclassified or leaked SQL: status=%d body=%s", rec.Code, rec.Body.String())
			}
		})
	}
}

type ambiguousRecoveryStore struct{ *store.Memory }

func (p *ambiguousRecoveryStore) GetRecoveryContext(_ context.Context) (recovery.Manager, error) {
	return p.Memory.GetRecovery()
}

func (p *ambiguousRecoveryStore) BeginRecoveryContext(_ context.Context, _ uint64) (recovery.Manager, error) {
	return recovery.Manager{}, fmt.Errorf("private SQL/connection details: %w", store.ErrCommitOutcomeUnknown)
}

func (p *ambiguousRecoveryStore) CompleteRecoveryContext(_ context.Context, _ uint64, _ bool) (recovery.Manager, error) {
	return recovery.Manager{}, fmt.Errorf("private SQL/connection details: %w", store.ErrCommitOutcomeUnknown)
}

func (p *ambiguousRecoveryStore) CreateRecoveryProof(_ context.Context, _ uint64, _ string) (recovery.Proof, error) {
	return recovery.Proof{}, fmt.Errorf("private SQL/connection details: %w", store.ErrCommitOutcomeUnknown)
}

func (p *ambiguousRecoveryStore) GetRecoveryProof(_ context.Context, _ uint64) (recovery.Proof, error) {
	return recovery.Proof{}, store.ErrNotFound
}

func TestUnknownCommitRecoveryRoutesPreserveAmbiguityWithoutLeakingSQL(t *testing.T) {
	handler := NewServer(&ambiguousRecoveryStore{Memory: store.NewMemory()}).Handler()
	for _, tc := range []struct {
		path string
		body map[string]any
	}{
		{path: "/api/v1/recovery/begin", body: map[string]any{"expected_recovery_epoch": 0}},
		{path: "/api/v1/recovery/complete", body: map[string]any{"recovery_epoch": 1}},
		{path: "/api/v1/recovery/proofs", body: map[string]any{"recovery_epoch": 1, "reconciler": "reviewer"}},
	} {
		body := mustRequest(t, handler, http.MethodPost, tc.path, tc.body, http.StatusServiceUnavailable)
		if !strings.Contains(string(body), "commit_outcome_unknown") ||
			!strings.Contains(string(body), "read back") ||
			strings.Contains(string(body), "private SQL") {
			t.Fatalf("Recovery handler misclassified lost COMMIT at %s: %s", tc.path, body)
		}
	}
}

type ambiguousWorkStore struct{ *store.Memory }

func (*ambiguousWorkStore) CreateWork(core.WorkItem) error {
	return fmt.Errorf("private SQL/connection details: %w", store.ErrCommitOutcomeUnknown)
}

func TestUnknownCommitWorkIntakeDoesNotLeakOrClaimRollback(t *testing.T) {
	handler := NewServer(&ambiguousWorkStore{Memory: store.NewMemory()}).Handler()
	response := mustRequest(t, handler, http.MethodPost, "/api/v1/work-items", map[string]any{
		"work_item_id": "unknown-commit-work", "title": "unknown COMMIT", "human_owner": "owner",
	}, http.StatusServiceUnavailable)
	if !strings.Contains(string(response), "commit_outcome_unknown") ||
		!strings.Contains(string(response), "read back") ||
		strings.Contains(string(response), "private SQL") {
		t.Fatalf("Work intake misreported ambiguous COMMIT: %s", response)
	}
}
