package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/core"
	"github.com/jiying2007/engineering-platform/internal/store"
	"github.com/jiying2007/engineering-platform/internal/verification"
)

type frozenReadbackStore struct {
	*store.Memory
	calls       atomic.Int32
	wantContext context.Context
	corrupt     bool
	cancel      context.CancelFunc
}

func (s *frozenReadbackStore) GetTaskByDigestContext(ctx context.Context, digest string) (core.TaskContract, error) {
	s.calls.Add(1)
	if s.wantContext != nil && ctx != s.wantContext {
		panic("request context was replaced")
	}
	value, err := s.Memory.GetTaskByDigestContext(ctx, digest)
	if s.corrupt {
		value.Repository = "secret-corrupt-record"
	}
	if s.cancel != nil {
		s.cancel()
	}
	return value, err
}

func (s *frozenReadbackStore) GetVerificationPlanByDigestContext(ctx context.Context, digest string) (verification.Plan, error) {
	s.calls.Add(1)
	if s.wantContext != nil && ctx != s.wantContext {
		panic("request context was replaced")
	}
	value, err := s.Memory.GetVerificationPlanByDigestContext(ctx, digest)
	if s.corrupt {
		value.ID = "secret-corrupt-record"
	}
	if s.cancel != nil {
		s.cancel()
	}
	return value, err
}

func frozenReadbackHistory(t *testing.T) (*frozenReadbackStore, core.TaskContract, verification.Plan) {
	t.Helper()
	s := &frozenReadbackStore{Memory: store.NewMemory()}
	plan := verification.Plan{ID: "historical-plan", Criteria: []verification.Criterion{{ID: "criterion-v1", Statement: "old criterion", Requirements: []verification.EvidenceRequirement{{ID: "req", Procedure: "test.procedure", Issuer: "test-issuer"}}}}}
	digest, err := plan.Digest()
	if err != nil {
		t.Fatal(err)
	}
	task := core.TaskContract{ID: "historical-task", WorkItemID: "historical-work", Revision: 1, VerificationPlanID: plan.ID, VerificationPlanDigest: digest, AcceptanceCriteria: []string{"old criterion"}}
	if err := s.CreateTask(task, plan); err != nil {
		t.Fatal(err)
	}
	newPlan := verification.Plan{ID: plan.ID, Criteria: []verification.Criterion{{ID: "criterion-v2", Statement: "new criterion", Requirements: []verification.EvidenceRequirement{{ID: "req", Procedure: "test.procedure", Issuer: "test-issuer"}}}}}
	newDigest, err := newPlan.Digest()
	if err != nil {
		t.Fatal(err)
	}
	newTask := task
	newTask.Revision, newTask.VerificationPlanDigest, newTask.AcceptanceCriteria = 2, newDigest, []string{"new criterion"}
	if err := s.CreateTask(newTask, newPlan); err != nil {
		t.Fatal(err)
	}
	return s, task, plan
}

func TestFrozenReadbackHistoryAndAuthenticatedReadCapability(t *testing.T) {
	backend, task, plan := frozenReadbackHistory(t)
	taskDigest, _ := task.Digest()
	server, pki := securedTestServer(t, backend, nil, AuthenticatedOptions{})
	reader := pki.Client(t, engineerSubject)
	denied := pki.Client(t, ciSubject)
	paths := []string{"/api/v1/task-contracts/by-digest/" + taskDigest, "/api/v1/verification-plans/by-digest/" + task.VerificationPlanDigest}
	var old struct {
		Contract core.TaskContract `json:"contract"`
		Digest   string            `json:"digest"`
	}
	if err := json.Unmarshal(secureRaw(t, reader, http.MethodGet, server.URL+paths[0], nil, http.StatusOK), &old); err != nil {
		t.Fatal(err)
	}
	if old.Contract.Revision != 1 || old.Digest != taskDigest || old.Contract.AcceptanceCriteria[0] != "old criterion" {
		t.Fatalf("historical Task was not read: %+v", old)
	}
	var oldPlan struct {
		Plan   verification.Plan `json:"plan"`
		Digest string            `json:"digest"`
	}
	if err := json.Unmarshal(secureRaw(t, reader, http.MethodGet, server.URL+paths[1], nil, http.StatusOK), &oldPlan); err != nil {
		t.Fatal(err)
	}
	if oldPlan.Plan.ID != plan.ID || oldPlan.Plan.Criteria[0].ID != "criterion-v1" || oldPlan.Digest != task.VerificationPlanDigest {
		t.Fatalf("historical Plan was not read: %+v", oldPlan)
	}
	if err := json.Unmarshal(secureRaw(t, reader, http.MethodGet, server.URL+"/api/v1/task-contracts/"+task.ID, nil, http.StatusOK), &old); err != nil {
		t.Fatal(err)
	}
	if old.Contract.Revision != 2 {
		t.Fatal("latest Task route changed semantics")
	}
	before := backend.calls.Load()
	for _, path := range paths {
		secureRaw(t, denied, http.MethodGet, server.URL+path, nil, http.StatusForbidden)
	}
	if backend.calls.Load() != before {
		t.Fatal("denied reader reached immutable Store lookup")
	}
}

func TestFrozenReadbackRejectsInvalidMissingCorruptAndCanceled(t *testing.T) {
	for _, resource := range []string{"task-contracts", "verification-plans"} {
		for _, scenario := range []string{"invalid", "missing", "corrupt", "canceled-before", "canceled-during", "request-context"} {
			t.Run(resource+"/"+scenario, func(t *testing.T) {
				backend, task, _ := frozenReadbackHistory(t)
				digest, _ := task.Digest()
				if resource == "verification-plans" {
					digest = task.VerificationPlanDigest
				}
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				backend.wantContext = ctx
				want, wantCalls := http.StatusOK, 1
				switch scenario {
				case "invalid":
					digest, want, wantCalls = "not-a-digest", http.StatusBadRequest, 0
				case "missing":
					digest, want = canonical.BytesDigest([]byte("absent")), http.StatusNotFound
				case "corrupt":
					backend.corrupt, want = true, http.StatusInternalServerError
				case "canceled-before":
					cancel()
					want, wantCalls = 499, 0
				case "canceled-during":
					backend.cancel, want = cancel, 499
				}
				recorder := httptest.NewRecorder()
				req := httptest.NewRequest(http.MethodGet, "/api/v1/"+resource+"/by-digest/"+digest, nil).WithContext(ctx)
				NewServer(backend).Handler().ServeHTTP(recorder, req)
				if strings.HasPrefix(scenario, "canceled") {
					if recorder.Code >= 200 && recorder.Code < 300 {
						t.Fatal("canceled request emitted successful historical record")
					}
				} else if recorder.Code != want {
					t.Fatalf("status=%d want=%d body=%s", recorder.Code, want, recorder.Body.String())
				}
				if backend.calls.Load() != int32(wantCalls) {
					t.Fatalf("Store calls=%d want=%d", backend.calls.Load(), wantCalls)
				}
				if strings.Contains(recorder.Body.String(), "secret-corrupt-record") {
					t.Fatal("corrupt record escaped public error handling")
				}
			})
		}
	}
}
