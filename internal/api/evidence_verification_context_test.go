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

type evidenceVerificationContextProbe struct {
	*store.Memory
	getPlanCalls          int
	registerEvidenceCalls int
	getEvidenceCalls      int
	createVerifyCalls     int
	getVerifyCalls        int
}

func (p *evidenceVerificationContextProbe) GetVerificationPlanByDigestContext(ctx context.Context, digest string) (verification.Plan, error) {
	p.getPlanCalls++
	if err := ctx.Err(); err != nil {
		return verification.Plan{}, err
	}
	return p.Memory.GetVerificationPlanByDigest(digest)
}
func (p *evidenceVerificationContextProbe) CreateEvidenceContext(ctx context.Context, item core.EvidenceRef) error {
	p.registerEvidenceCalls++
	if err := ctx.Err(); err != nil {
		return err
	}
	return p.Memory.CreateEvidence(item)
}
func (p *evidenceVerificationContextProbe) GetEvidenceContext(ctx context.Context, id string) (core.EvidenceRef, error) {
	p.getEvidenceCalls++
	if err := ctx.Err(); err != nil {
		return core.EvidenceRef{}, err
	}
	return p.Memory.GetEvidence(id)
}
func (p *evidenceVerificationContextProbe) CreateVerificationContext(ctx context.Context, report verification.Report) error {
	p.createVerifyCalls++
	if err := ctx.Err(); err != nil {
		return err
	}
	return p.Memory.CreateVerification(report)
}
func (p *evidenceVerificationContextProbe) GetVerificationContext(ctx context.Context, id string) (verification.Report, error) {
	p.getVerifyCalls++
	if err := ctx.Err(); err != nil {
		return verification.Report{}, err
	}
	return p.Memory.GetVerification(id)
}

func TestEvidenceVerificationHTTPRoutesUseCallerBoundCoreAuthority(t *testing.T) {
	probe := &evidenceVerificationContextProbe{Memory: store.NewMemory()}
	handler := NewServer(probe).Handler()
	delivery := createCompletedDelivery(t, handler, "ev-context")
	mustRequest(t, handler, http.MethodPost, "/api/v1/evidence", map[string]any{
		"delivery_receipt_id": delivery.ID,
		"evidence": map[string]any{
			"evidence_id": "ev-context",
			"requirement_id": "req-1",
			"issuer": "ci",
			"procedure": "ci.test",
			"result": "PASS",
			"applicable": true,
		},
	}, http.StatusCreated)
	mustRequest(t, handler, http.MethodGet, "/api/v1/evidence/ev-context", nil, http.StatusOK)
	mustRequest(t, handler, http.MethodPost, "/api/v1/verifications", map[string]any{
		"verification_report_id": "verify-context",
		"delivery_receipt_id": delivery.ID,
		"verifier": "independent-verifier",
		"evidence_ids": []string{"ev-context"},
	}, http.StatusCreated)
	mustRequest(t, handler, http.MethodGet, "/api/v1/verifications/verify-context", nil, http.StatusOK)
	if probe.getPlanCalls != 2 || probe.registerEvidenceCalls != 1 ||
		probe.getEvidenceCalls != 2 || probe.createVerifyCalls != 1 ||
		probe.getVerifyCalls != 1 {
		t.Fatalf("Evidence/Verification HTTP bypassed contextual Store: %+v", probe)
	}

	body, err := json.Marshal(map[string]any{
		"delivery_receipt_id": delivery.ID,
		"evidence": map[string]any{
			"evidence_id": "never-create-cancelled",
			"requirement_id": "req-1",
			"issuer": "ci",
			"procedure": "ci.test",
			"result": "PASS",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/evidence", bytes.NewReader(body)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code == http.StatusCreated || probe.registerEvidenceCalls != 1 {
		t.Fatalf("cancelled Evidence request mutated Core: status=%d calls=%d", rec.Code, probe.registerEvidenceCalls)
	}
	if _, err := probe.Memory.GetEvidence("never-create-cancelled"); err != store.ErrNotFound {
		t.Fatalf("canceled Evidence created record: %v", err)
	}
}
