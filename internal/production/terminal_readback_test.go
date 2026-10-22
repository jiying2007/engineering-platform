package production

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/core"
	"github.com/jiying2007/engineering-platform/internal/review"
	"github.com/jiying2007/engineering-platform/internal/run"
	"github.com/jiying2007/engineering-platform/internal/session"
	"github.com/jiying2007/engineering-platform/internal/verification"
)

// These are deliberately synthetic Core records. No test principal is a real
// human acceptance and no test Evidence is live provider/CI/operational proof.
type terminalFixtureReader struct {
	sources map[string][]byte
	subject string
	calls   []string
	before  func(context.Context, string) error
	after   func(string, []byte) []byte
}

func (r *terminalFixtureReader) Subject() string { return r.subject }
func (r *terminalFixtureReader) Raw(ctx context.Context, method, path string, body []byte) ([]byte, error) {
	if method != http.MethodGet || body != nil {
		return nil, fmt.Errorf("only bodyless GET allowed")
	}
	r.calls = append(r.calls, path)
	if r.before != nil {
		if err := r.before(ctx, path); err != nil {
			return nil, err
		}
	}
	raw, ok := r.sources[path]
	if !ok {
		return nil, fmt.Errorf("missing fixture source")
	}
	if r.after != nil {
		return r.after(path, raw), nil
	}
	return raw, nil
}

func (r *terminalFixtureReader) put(t *testing.T, path string, value any) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	r.sources[path] = raw
}

func (r *terminalFixtureReader) change(t *testing.T, path string, mutate func(map[string]any)) {
	t.Helper()
	var value map[string]any
	if err := json.Unmarshal(r.sources[path], &value); err != nil {
		t.Fatal(err)
	}
	mutate(value)
	r.put(t, path, value)
}

func terminalReadbackFixture(t *testing.T) (*terminalFixtureReader, TerminalReadbackRequest) {
	t.Helper()
	r := &terminalFixtureReader{sources: map[string][]byte{}, subject: "urn:engineering-platform:test:terminal-reader"}
	raw, err := os.ReadFile("../../examples/production/terminal-maintenance/task.json.tmpl")
	if err != nil {
		t.Fatal(err)
	}
	var template struct {
		Contract core.TaskContract `json:"contract"`
		Material struct {
			Repository         string   `json:"repository"`
			TargetID           string   `json:"target_id"`
			AcceptanceCriteria []string `json:"acceptance_criteria"`
		} `json:"material"`
		Plan verification.Plan `json:"verification_plan"`
	}
	if err := json.Unmarshal(raw, &template); err != nil {
		t.Fatal(err)
	}
	task, plan := template.Contract, template.Plan
	task.Repository, task.TargetID, task.AcceptanceCriteria = template.Material.Repository, template.Material.TargetID, template.Material.AcceptanceCriteria
	task.BaseCommit, task.Revision, task.VerificationPlanID = strings.Repeat("a", 40), 1, plan.ID
	task.VerificationPlanDigest, _ = plan.Digest()
	taskDigest, _ := task.Digest()
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	input := core.RunInputManifest{RunID: "terminal-run", TaskContractDigest: taskDigest, RuntimeProfile: "test-runtime", ToolProfile: "test-tool", WorkerProfile: "test-worker", PolicyProfile: "test-policy"}
	inputDigest, _ := input.Digest()
	execution := terminalRunEnvelope{Run: run.Run{ID: input.RunID, TaskContractDigest: taskDigest, RunInputManifestDigest: inputDigest, State: run.Completed, Version: 2, CurrentEpoch: 1, CurrentAttemptID: "attempt-1", ControlOwner: "RUNTIME"}, Session: session.Session{RunID: input.RunID, ExecutionEpoch: 1, Owner: session.Runtime}, RunInput: input}
	delivery := core.DeliveryReceipt{ID: "terminal-delivery", WorkItemID: task.WorkItemID, TaskContractDigest: taskDigest, RunID: input.RunID, TargetID: task.TargetID, BaseCommit: task.BaseCommit, ResultCommit: strings.Repeat("b", 40), Artifacts: []core.ArtifactRef{{ID: "test-artifact", Digest: canonical.BytesDigest([]byte("synthetic artifact bytes"))}}, CreatedAt: now}
	delivery.SubjectDigest, _ = delivery.CalculateSubjectDigest()
	var evidence []core.EvidenceRef
	for _, criterion := range plan.Criteria {
		req := criterion.Requirements[0]
		item := core.EvidenceRef{ID: "evidence-" + req.ID, DeliveryReceiptID: delivery.ID, RequirementID: req.ID, SubjectDigest: delivery.SubjectDigest, Issuer: req.Issuer, Procedure: req.Procedure, Result: "PASS", Applicable: true, ArtifactRefs: []string{"test-artifact"}}
		evidence = append(evidence, item)
		r.put(t, "/api/v1/evidence/"+item.ID, item)
	}
	verified := verification.Evaluate(plan, delivery.SubjectDigest, evidence)
	verified.ID, verified.DeliveryReceiptID, verified.VerificationPlanDigest = "terminal-verification", delivery.ID, task.VerificationPlanDigest
	verified.Verifier, verified.CreatedAt = "urn:engineering-platform:test:verifier", now.Add(time.Second)
	reviewed := review.Report{ID: "terminal-review", DeliveryReceiptID: delivery.ID, VerificationReportID: verified.ID, TaskContractDigest: taskDigest, SubjectDigest: delivery.SubjectDigest, Reviewer: "urn:engineering-platform:test:reviewer", Result: review.ResultPass, CreatedAt: now.Add(2 * time.Second)}
	closure := core.ClosureReceipt{ID: "terminal-closure", WorkItemID: task.WorkItemID, TaskContractDigest: taskDigest, RunID: input.RunID, DeliveryReceiptID: delivery.ID, VerificationReportID: verified.ID, ReviewReportID: reviewed.ID, SubjectDigest: delivery.SubjectDigest, Result: "CLOSED", CreatedAt: now.Add(3 * time.Second)}
	// A newer mutable Work does not replace the old Run's frozen Task identity.
	work := core.WorkItem{ID: task.WorkItemID, HumanOwner: "urn:engineering-platform:test:owner", State: core.WorkExecuting, ActiveTaskContractDigest: canonical.BytesDigest([]byte("newer task")), ActiveRunID: "newer-run", Version: 8, CreatedAt: now.Add(-time.Hour)}
	r.put(t, "/api/v1/closures/"+closure.ID, closure)
	r.put(t, "/api/v1/deliveries/"+delivery.ID, delivery)
	r.put(t, "/api/v1/task-contracts/by-digest/"+taskDigest, terminalTaskEnvelope{Contract: task, Digest: taskDigest})
	r.put(t, "/api/v1/verification-plans/by-digest/"+task.VerificationPlanDigest, terminalVerificationPlanEnvelope{Plan: plan, Digest: task.VerificationPlanDigest})
	r.put(t, "/api/v1/runs/"+input.RunID, execution)
	r.put(t, "/api/v1/work-items/"+work.ID, work)
	r.put(t, "/api/v1/verifications/"+verified.ID, verified)
	r.put(t, "/api/v1/reviews/"+reviewed.ID, reviewed)
	return r, TerminalReadbackRequest{ClosureID: closure.ID, RunID: input.RunID, SubjectDigest: delivery.SubjectDigest}
}

func TestTerminalReadbackHistoricalCoreAndFourteenGateDiagnostics(t *testing.T) {
	reader, request := terminalReadbackFixture(t)
	var firstContext context.Context
	reader.before = func(ctx context.Context, _ string) error {
		if firstContext == nil {
			firstContext = ctx
		}
		if ctx != firstContext {
			t.Fatal("readback did not share one total deadline context")
		}
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > TerminalReadbackTimeout {
			t.Fatal("missing or oversized total deadline")
		}
		return nil
	}
	report, err := ObserveTerminalCore(context.Background(), reader, request)
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != TerminalCoreVerified || report.QualificationGranted || report.ProductionQualified || report.Ready || report.HumanPresenceEstablished || report.ArtifactSourceBytesVerified || !report.CoreReviewPrincipalsDiffer {
		t.Fatalf("wrong diagnostic boundary: %+v", report)
	}
	if len(reader.calls) != TerminalReadbackMaxRecords || len(report.Records) != 12 || len(report.Procedures) != 3 || len(report.Gates) != 14 {
		t.Fatal("incomplete readback or diagnostic coverage")
	}
	plan, _ := BuildTerminalPlan()
	for i, gate := range report.Gates {
		if gate.Gate != plan.Plan.RequiredGates[i] {
			t.Fatal("gate order or coverage drift")
		}
		want := "NOT_OBSERVED"
		if gate.Gate == "verification_pass" || gate.Gate == "closure_created" {
			want = "OBSERVED"
		}
		if gate.Observation != want {
			t.Fatalf("gate %s unexpectedly grants evidence: %+v", gate.Gate, gate)
		}
	}
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	var restored TerminalReadbackReport
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	for _, record := range restored.Records {
		if canonical.BytesDigest(record.SourceBytes) != record.SourceDigest || !reflect.DeepEqual(record.SourceBytes, reader.sources[record.Resource]) || !canonical.ValidDigest(record.RecordDigest) {
			t.Fatal("original source bytes were normalized or digest lost")
		}
	}
	if reader.calls[0] != reader.calls[11] {
		t.Fatal("immutable Closure was not reread")
	}
}

func TestTerminalReadbackRejectsBrokenImmutableRelationships(t *testing.T) {
	cases := []struct {
		name, path string
		mutate     func(map[string]any)
	}{
		{"closure-run", "/api/v1/closures/terminal-closure", func(v map[string]any) { v["run_id"] = "another-run" }},
		{"closure-subject", "/api/v1/closures/terminal-closure", func(v map[string]any) { v["subject_digest"] = canonical.BytesDigest([]byte("other")) }},
		{"closure-path-injection", "/api/v1/closures/terminal-closure", func(v map[string]any) { v["delivery_receipt_id"] = "../private" }},
		{"delivery-artifact-digest", "/api/v1/deliveries/terminal-delivery", func(v map[string]any) {
			v["artifacts"].([]any)[0].(map[string]any)["digest"] = canonical.BytesDigest([]byte("wrong"))
		}},
		{"delivery-reference", "/api/v1/deliveries/terminal-delivery", func(v map[string]any) { v["work_item_id"] = "wrong-work" }},
		{"run-not-completed", "/api/v1/runs/terminal-run", func(v map[string]any) { v["run"].(map[string]any)["state"] = "RUNNING" }},
		{"input-digest", "/api/v1/runs/terminal-run", func(v map[string]any) { v["run_input"].(map[string]any)["runtime_profile"] = "changed" }},
		{"session-epoch", "/api/v1/runs/terminal-run", func(v map[string]any) { v["session"].(map[string]any)["execution_epoch"] = 2 }},
		{"verification-plan", "/api/v1/verifications/terminal-verification", func(v map[string]any) { v["verification_plan_digest"] = canonical.BytesDigest([]byte("wrong")) }},
		{"verification-result", "/api/v1/verifications/terminal-verification", func(v map[string]any) { v["criteria"].([]any)[0].(map[string]any)["result"] = "FAIL" }},
		{"duplicate-evidence", "/api/v1/verifications/terminal-verification", func(v map[string]any) { v["evidence_ids"].([]any)[1] = v["evidence_ids"].([]any)[0] }},
		{"incomplete-evidence", "/api/v1/verifications/terminal-verification", func(v map[string]any) { v["evidence_ids"] = []string{"evidence-terminal-ci"} }},
		{"foreign-issuer", "/api/v1/evidence/evidence-terminal-ci", func(v map[string]any) { v["issuer"] = "untrusted-importer" }},
		{"foreign-procedure", "/api/v1/evidence/evidence-terminal-ci", func(v map[string]any) { v["procedure"] = "invented.procedure" }},
		{"foreign-subject", "/api/v1/evidence/evidence-terminal-ci", func(v map[string]any) { v["subject_digest"] = canonical.BytesDigest([]byte("wrong")) }},
		{"foreign-artifact", "/api/v1/evidence/evidence-terminal-ci", func(v map[string]any) { v["artifact_refs"] = []string{"absent"} }},
		{"not-applicable", "/api/v1/evidence/evidence-terminal-ci", func(v map[string]any) { v["applicable"] = false }},
		{"self-review-verifier", "/api/v1/reviews/terminal-review", func(v map[string]any) { v["reviewer"] = "urn:engineering-platform:test:verifier" }},
		{"self-review-owner", "/api/v1/reviews/terminal-review", func(v map[string]any) { v["reviewer"] = "urn:engineering-platform:test:owner" }},
		{"review-wrong-verification", "/api/v1/reviews/terminal-review", func(v map[string]any) { v["verification_report_id"] = "wrong-report" }},
		{"review-time-order", "/api/v1/reviews/terminal-review", func(v map[string]any) { v["created_at"] = "2026-10-19T00:00:00Z" }},
		{"unknown-source-field", "/api/v1/closures/terminal-closure", func(v map[string]any) { v["secret_unexpected_field"] = "never-print-this" }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			reader, req := terminalReadbackFixture(t)
			reader.change(t, c.path, c.mutate)
			report, err := ObserveTerminalCore(context.Background(), reader, req)
			if err == nil || !reflect.DeepEqual(report, TerminalReadbackReport{}) {
				t.Fatal("invalid chain emitted a report")
			}
			if strings.Contains(err.Error(), "never-print-this") {
				t.Fatal("raw source field leaked")
			}
		})
	}
	for _, resource := range []string{"task-contracts/by-digest/", "verification-plans/by-digest/"} {
		t.Run(resource, func(t *testing.T) {
			reader, req := terminalReadbackFixture(t)
			for path := range reader.sources {
				if strings.Contains(path, resource) {
					reader.change(t, path, func(v map[string]any) { v["digest"] = canonical.BytesDigest([]byte("wrong")) })
				}
			}
			report, err := ObserveTerminalCore(context.Background(), reader, req)
			if err == nil || !reflect.DeepEqual(report, TerminalReadbackReport{}) {
				t.Fatal("mismatched frozen digest emitted a report")
			}
		})
	}
}

func TestTerminalReadbackLimitsIdentityCancellationAndSafeErrors(t *testing.T) {
	for _, scenario := range []string{"record-bytes", "total-bytes", "subject-change", "closure-change", "duplicate-json", "transport-secret", "canceled", "deadline"} {
		t.Run(scenario, func(t *testing.T) {
			reader, req := terminalReadbackFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var wantCause error
			switch scenario {
			case "record-bytes":
				reader.after = func(_ string, raw []byte) []byte {
					return append(append([]byte(nil), raw...), []byte(strings.Repeat(" ", TerminalReadbackMaxRecordBytes))...)
				}
			case "total-bytes":
				reader.after = func(_ string, raw []byte) []byte {
					return append(append([]byte(nil), raw...), []byte(strings.Repeat(" ", (240<<10)-len(raw)))...)
				}
			case "subject-change":
				reader.after = func(_ string, raw []byte) []byte {
					reader.subject = "urn:engineering-platform:test:changed"
					return raw
				}
			case "closure-change":
				reader.after = func(_ string, raw []byte) []byte {
					if len(reader.calls) == 12 {
						return []byte(strings.Replace(string(raw), `"result":"CLOSED"`, `"result":"OTHER"`, 1))
					}
					return raw
				}
			case "duplicate-json":
				reader.after = func(_ string, raw []byte) []byte { return append([]byte(`{"result":"CLOSED",`), raw[1:]...) }
			case "transport-secret":
				wantCause = errors.New("SECRET bearer-token private-key-path")
				reader.before = func(context.Context, string) error { return wantCause }
			case "canceled":
				wantCause = context.Canceled
				reader.after = func(_ string, raw []byte) []byte { cancel(); return raw }
			case "deadline":
				var stop context.CancelFunc
				ctx, stop = context.WithTimeout(ctx, 5*time.Millisecond)
				defer stop()
				wantCause = context.DeadlineExceeded
				reader.before = func(ctx context.Context, _ string) error { <-ctx.Done(); return ctx.Err() }
			}
			report, err := ObserveTerminalCore(ctx, reader, req)
			if err == nil || !reflect.DeepEqual(report, TerminalReadbackReport{}) {
				t.Fatal("failure emitted a report")
			}
			if wantCause != nil && !errors.Is(err, wantCause) {
				t.Fatal("error cause lost")
			}
			if strings.Contains(err.Error(), "SECRET") || strings.Contains(err.Error(), "private-key-path") {
				t.Fatal("transport detail leaked")
			}
			if len(reader.calls) > TerminalReadbackMaxRecords {
				t.Fatal("record count exceeded")
			}
		})
	}
}
