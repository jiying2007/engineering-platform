package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jiying2007/engineering-platform/internal/core"
	"github.com/jiying2007/engineering-platform/internal/review"
	"github.com/jiying2007/engineering-platform/internal/store"
)

type reviewClosureContextProbe struct {
	*store.Memory
	createReviewCalls  int
	getReviewCalls     int
	createClosureCalls int
	getClosureCalls    int
}

func (p *reviewClosureContextProbe) CreateReviewAndUpdateWorkContext(ctx context.Context, report review.Report, version uint64, work core.WorkItem) error {
	p.createReviewCalls++
	if err := ctx.Err(); err != nil {
		return err
	}
	return p.Memory.CreateReviewAndUpdateWork(report, version, work)
}

func (p *reviewClosureContextProbe) GetReviewContext(ctx context.Context, id string) (review.Report, error) {
	p.getReviewCalls++
	if err := ctx.Err(); err != nil {
		return review.Report{}, err
	}
	return p.Memory.GetReview(id)
}

func (p *reviewClosureContextProbe) CreateClosureAndUpdateWorkContext(ctx context.Context, item core.ClosureReceipt, version uint64, work core.WorkItem) error {
	p.createClosureCalls++
	if err := ctx.Err(); err != nil {
		return err
	}
	return p.Memory.CreateClosureAndUpdateWork(item, version, work)
}

func (p *reviewClosureContextProbe) GetClosureContext(ctx context.Context, id string) (core.ClosureReceipt, error) {
	p.getClosureCalls++
	if err := ctx.Err(); err != nil {
		return core.ClosureReceipt{}, err
	}
	return p.Memory.GetClosure(id)
}

func TestReviewClosureHTTPRoutesUseCallerBoundCoreAuthority(t *testing.T) {
	probe := &reviewClosureContextProbe{Memory: store.NewMemory()}
	handler := NewServer(probe).Handler()
	delivery := createCompletedDelivery(t, handler, "review-context")
	mustRequest(t, handler, http.MethodPost, "/api/v1/evidence", map[string]any{
		"delivery_receipt_id": delivery.ID,
		"evidence": map[string]any{
			"evidence_id":    "review-context-evidence",
			"requirement_id": "req-1",
			"issuer":         "ci",
			"procedure":      "ci.test",
			"result":         "PASS",
			"applicable":     true,
		},
	}, http.StatusCreated)
	mustRequest(t, handler, http.MethodPost, "/api/v1/verifications", map[string]any{
		"verification_report_id": "review-context-verification",
		"delivery_receipt_id":    delivery.ID,
		"verifier":               "verifier",
		"evidence_ids":           []string{"review-context-evidence"},
	}, http.StatusCreated)
	reviewPayload := map[string]any{
		"review_report_id":       "review-context-report",
		"delivery_receipt_id":    delivery.ID,
		"verification_report_id": "review-context-verification",
		"reviewer":               "independent-reviewer",
		"result":                 "PASS",
	}
	closurePayload := map[string]any{
		"closure_receipt_id":     "review-context-closure",
		"delivery_receipt_id":    delivery.ID,
		"verification_report_id": "review-context-verification",
		"review_report_id":       "review-context-report",
	}
	sendCancelled := func(path string, payload map[string]any) {
		t.Helper()
		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body)).WithContext(ctx)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code == http.StatusCreated {
			t.Fatalf("cancelled request %s created authority", path)
		}
	}
	sendCancelled("/api/v1/reviews", reviewPayload)
	if _, err := probe.Memory.GetReview("review-context-report"); err != store.ErrNotFound || probe.createReviewCalls != 0 {
		t.Fatalf("cancelled review wrote authority: err=%v calls=%d", err, probe.createReviewCalls)
	}
	mustRequest(t, handler, http.MethodPost, "/api/v1/reviews", reviewPayload, http.StatusCreated)
	mustRequest(t, handler, http.MethodGet, "/api/v1/reviews/review-context-report", nil, http.StatusOK)
	sendCancelled("/api/v1/closures", closurePayload)
	if _, err := probe.Memory.GetClosure("review-context-closure"); err != store.ErrNotFound || probe.createClosureCalls != 0 {
		t.Fatalf("cancelled closure wrote authority: err=%v calls=%d", err, probe.createClosureCalls)
	}
	mustRequest(t, handler, http.MethodPost, "/api/v1/closures", closurePayload, http.StatusCreated)
	mustRequest(t, handler, http.MethodGet, "/api/v1/closures/review-context-closure", nil, http.StatusOK)
	if probe.createReviewCalls != 1 || probe.getReviewCalls != 2 ||
		probe.createClosureCalls != 1 || probe.getClosureCalls != 1 {
		t.Fatalf("Review/Closure HTTP bypassed caller-bound Store: %+v", probe)
	}
}
