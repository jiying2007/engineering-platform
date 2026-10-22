package production

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"regexp"
	"strings"
	"time"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/core"
	"github.com/jiying2007/engineering-platform/internal/review"
	"github.com/jiying2007/engineering-platform/internal/run"
	"github.com/jiying2007/engineering-platform/internal/session"
	"github.com/jiying2007/engineering-platform/internal/strictjson"
	"github.com/jiying2007/engineering-platform/internal/verification"
)

const (
	TerminalReadbackVersion        = 1
	TerminalReadbackTimeout        = 30 * time.Second
	TerminalReadbackMaxRecords     = 12
	TerminalReadbackMaxRecordBytes = 256 << 10
	TerminalReadbackMaxTotalBytes  = 2 << 20
	TerminalCoreVerified           = "CORE_RELATIONSHIPS_VERIFIED"
)

var terminalReadbackID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,255}$`)
var terminalReadbackCommit = regexp.MustCompile(`^[0-9a-f]{40}$`)

type TerminalReadbackRequest struct {
	ClosureID     string `json:"closure_receipt_id"`
	RunID         string `json:"run_id"`
	SubjectDigest string `json:"subject_digest"`
}

func (r TerminalReadbackRequest) Validate() error {
	if !terminalReadbackID.MatchString(r.ClosureID) || !terminalReadbackID.MatchString(r.RunID) || !canonical.ValidDigest(r.SubjectDigest) {
		return fmt.Errorf("terminal readback requires bounded closure/run IDs and an exact sha256 subject digest")
	}
	return nil
}

// TerminalCoreReader is implemented by the direct, no-redirect mTLS Control
// client. Callers must provide that authenticated client in production; an
// arbitrary implementation of this interface does not establish authentication.
type TerminalCoreReader interface {
	Subject() string
	Raw(context.Context, string, string, []byte) ([]byte, error)
}

type TerminalReadbackRecord struct {
	Resource     string `json:"resource"`
	SourceDigest string `json:"source_digest"`
	RecordDigest string `json:"record_digest"`
	// []byte is deliberately base64 in JSON. Indenting a json.RawMessage would
	// change the retained source bytes and invalidate their source_digest.
	SourceBytes []byte `json:"source_bytes"`
}

type TerminalGateObservation struct {
	Gate        string `json:"gate"`
	Observation string `json:"observation"`
	Reason      string `json:"reason"`
}

type TerminalProcedureObservation struct {
	RequirementID string `json:"requirement_id"`
	Procedure     string `json:"procedure"`
	Issuer        string `json:"issuer"`
	EvidenceID    string `json:"evidence_id"`
}

type TerminalReadbackReport struct {
	Version                     int                            `json:"version"`
	Status                      string                         `json:"status"`
	FixtureID                   string                         `json:"fixture_id"`
	TerminalPlanDigest          string                         `json:"terminal_plan_digest"`
	ReaderSubject               string                         `json:"reader_subject"`
	Request                     TerminalReadbackRequest        `json:"request"`
	TaskContractDigest          string                         `json:"task_contract_digest"`
	VerificationPlanDigest      string                         `json:"verification_plan_digest"`
	RunInputManifestDigest      string                         `json:"run_input_manifest_digest"`
	VerificationReportID        string                         `json:"verification_report_id"`
	ReviewReportID              string                         `json:"review_report_id"`
	CoreReviewPrincipalsDiffer  bool                           `json:"core_review_principals_differ"`
	HumanPresenceEstablished    bool                           `json:"human_presence_established"`
	ArtifactSourceBytesVerified bool                           `json:"artifact_source_bytes_verified"`
	QualificationGranted        bool                           `json:"qualification_granted"`
	ProductionQualified         bool                           `json:"production_qualified"`
	Ready                       bool                           `json:"ready"`
	Procedures                  []TerminalProcedureObservation `json:"procedures"`
	Gates                       []TerminalGateObservation      `json:"gates"`
	Records                     []TerminalReadbackRecord       `json:"records"`
}

type terminalTaskEnvelope struct {
	Contract core.TaskContract `json:"contract"`
	Digest   string            `json:"digest"`
}

type terminalVerificationPlanEnvelope struct {
	Plan   verification.Plan `json:"plan"`
	Digest string            `json:"digest"`
}

type terminalRunEnvelope struct {
	Run      run.Run               `json:"run"`
	Session  session.Session       `json:"session"`
	RunInput core.RunInputManifest `json:"run_input"`
}

type terminalReadbackError struct {
	message string
	cause   error
}

func (e *terminalReadbackError) Error() string { return "terminal Core readback: " + e.message }
func (e *terminalReadbackError) Unwrap() error { return e.cause }

type terminalRecordReader struct {
	client  TerminalCoreReader
	subject string
	total   int
	records []TerminalReadbackRecord
}

func (r *terminalRecordReader) get(ctx context.Context, resource, id string, out any) error {
	if err := ctx.Err(); err != nil {
		return &terminalReadbackError{"canceled or deadline exceeded", err}
	}
	if !terminalReadbackID.MatchString(id) || len(r.records) >= TerminalReadbackMaxRecords || r.client.Subject() != r.subject {
		return fmt.Errorf("terminal Core readback: invalid resource identity, count or reader subject")
	}
	path := "/api/v1/" + resource + "/" + id
	raw, err := r.client.Raw(ctx, http.MethodGet, path, nil)
	if err != nil {
		// Never include a transport URL, response body, TLS credential path or
		// arbitrary reader error in printable diagnostics.
		return &terminalReadbackError{"source request failed", err}
	}
	if err := ctx.Err(); err != nil {
		return &terminalReadbackError{"canceled or deadline exceeded", err}
	}
	if r.client.Subject() != r.subject || len(raw) == 0 || len(raw) > TerminalReadbackMaxRecordBytes || r.total+len(raw) > TerminalReadbackMaxTotalBytes {
		return fmt.Errorf("terminal Core readback: reader subject changed or source byte limit exceeded")
	}
	if err := strictjson.Decode(raw, out); err != nil {
		return &terminalReadbackError{"invalid strict source record", err}
	}
	digest, err := canonical.Digest(out)
	if err != nil {
		return &terminalReadbackError{"source record digest failed", err}
	}
	r.total += len(raw)
	r.records = append(r.records, TerminalReadbackRecord{Resource: path, SourceDigest: canonical.BytesDigest(raw), RecordDigest: digest, SourceBytes: append([]byte(nil), raw...)})
	return nil
}

// ObserveTerminalCore rechecks the existing, frozen maintenance Core chain.
// It does not accept new evidence, resolve operational gates, infer a real human
// from a stored principal name, or treat Closure/SLO as production authority.
func ObserveTerminalCore(ctx context.Context, client TerminalCoreReader, request TerminalReadbackRequest) (TerminalReadbackReport, error) {
	var zero TerminalReadbackReport
	if err := request.Validate(); err != nil {
		return zero, err
	}
	if ctx == nil || client == nil || !terminalPrincipal(client.Subject()) {
		return zero, fmt.Errorf("terminal Core readback requires a context and authenticated reader identity")
	}
	ctx, cancel := context.WithTimeout(ctx, TerminalReadbackTimeout)
	defer cancel()
	r := terminalRecordReader{client: client, subject: client.Subject()}
	var closure core.ClosureReceipt
	if err := r.get(ctx, "closures", request.ClosureID, &closure); err != nil {
		return zero, err
	}
	if closure.ID != request.ClosureID || closure.RunID != request.RunID || closure.SubjectDigest != request.SubjectDigest || closure.Result != "CLOSED" || closure.CreatedAt.IsZero() || !canonical.ValidDigest(closure.TaskContractDigest) {
		return zero, fmt.Errorf("terminal Core readback: Closure does not match the exact requested run and subject")
	}
	var delivery core.DeliveryReceipt
	if err := r.get(ctx, "deliveries", closure.DeliveryReceiptID, &delivery); err != nil {
		return zero, err
	}
	deliveryDigest, err := delivery.CalculateSubjectDigest()
	if err != nil || deliveryDigest != request.SubjectDigest || delivery.ID != closure.DeliveryReceiptID || delivery.WorkItemID != closure.WorkItemID || delivery.RunID != request.RunID || delivery.TaskContractDigest != closure.TaskContractDigest || delivery.SubjectDigest != request.SubjectDigest || delivery.CreatedAt.IsZero() || closure.CreatedAt.Before(delivery.CreatedAt) || !terminalReadbackCommit.MatchString(delivery.BaseCommit) || !terminalReadbackCommit.MatchString(delivery.ResultCommit) || delivery.BaseCommit == delivery.ResultCommit || !terminalArtifacts(delivery.Artifacts) {
		return zero, fmt.Errorf("terminal Core readback: Delivery subject, artifacts or Closure relationship invalid")
	}
	var taskEnvelope terminalTaskEnvelope
	if err := r.get(ctx, "task-contracts/by-digest", closure.TaskContractDigest, &taskEnvelope); err != nil {
		return zero, err
	}
	task := taskEnvelope.Contract
	taskDigest, err := task.Digest()
	if err != nil || taskDigest != closure.TaskContractDigest || taskEnvelope.Digest != taskDigest || task.WorkItemID != delivery.WorkItemID || task.BaseCommit != delivery.BaseCommit || task.TargetID != delivery.TargetID || !canonical.ValidDigest(task.VerificationPlanDigest) {
		return zero, fmt.Errorf("terminal Core readback: frozen Task digest or Delivery relationship invalid")
	}
	var planEnvelope terminalVerificationPlanEnvelope
	if err := r.get(ctx, "verification-plans/by-digest", task.VerificationPlanDigest, &planEnvelope); err != nil {
		return zero, err
	}
	plan := planEnvelope.Plan
	planDigest, err := plan.Digest()
	if err != nil || planDigest != task.VerificationPlanDigest || planEnvelope.Digest != planDigest || plan.ID != task.VerificationPlanID || !verification.ValidatePlan(plan, task.AcceptanceCriteria) || !terminalFrozenMaintenance(task, plan) {
		return zero, fmt.Errorf("terminal Core readback: frozen maintenance Task or VerificationPlan invalid")
	}
	var execution terminalRunEnvelope
	if err := r.get(ctx, "runs", request.RunID, &execution); err != nil {
		return zero, err
	}
	inputDigest, err := execution.RunInput.Digest()
	if err != nil || !canonical.ValidDigest(execution.Run.RunInputManifestDigest) || inputDigest != execution.Run.RunInputManifestDigest || execution.Run.ID != request.RunID || execution.Run.TaskContractDigest != taskDigest || execution.Run.State != run.Completed || execution.Run.Version == 0 || execution.Run.CurrentEpoch == 0 || execution.Session.RunID != request.RunID || execution.Session.ExecutionEpoch != execution.Run.CurrentEpoch || string(execution.Session.Owner) != execution.Run.ControlOwner || (execution.Session.Owner != session.Runtime && execution.Session.Owner != session.Human) || execution.RunInput.RunID != request.RunID || execution.RunInput.TaskContractDigest != taskDigest {
		return zero, fmt.Errorf("terminal Core readback: completed Run, Session or immutable input relationship invalid")
	}
	var work core.WorkItem
	if err := r.get(ctx, "work-items", delivery.WorkItemID, &work); err != nil {
		return zero, err
	}
	// Work is mutable. In particular its current active Task/Run and state are
	// not a historical lookup key and must not invalidate a prior Closure.
	if work.ID != delivery.WorkItemID || !terminalPrincipal(work.HumanOwner) {
		return zero, fmt.Errorf("terminal Core readback: Work identity or owner invalid")
	}
	var verified verification.Report
	if err := r.get(ctx, "verifications", closure.VerificationReportID, &verified); err != nil {
		return zero, err
	}
	if verified.ID != closure.VerificationReportID || verified.DeliveryReceiptID != delivery.ID || verified.PlanID != plan.ID || verified.VerificationPlanDigest != planDigest || verified.SubjectDigest != request.SubjectDigest || verified.Result != "PASS" || !terminalPrincipal(verified.Verifier) || verified.CreatedAt.IsZero() || verified.CreatedAt.Before(delivery.CreatedAt) || closure.CreatedAt.Before(verified.CreatedAt) || len(verified.EvidenceIDs) != 3 {
		return zero, fmt.Errorf("terminal Core readback: Verification metadata or evidence count invalid")
	}
	var evidence []core.EvidenceRef
	var procedures []TerminalProcedureObservation
	seenIDs, seenRequirements := map[string]bool{}, map[string]bool{}
	for _, id := range verified.EvidenceIDs {
		if seenIDs[id] {
			return zero, fmt.Errorf("terminal Core readback: duplicate Evidence identity")
		}
		seenIDs[id] = true
		var item core.EvidenceRef
		if err := r.get(ctx, "evidence", id, &item); err != nil {
			return zero, err
		}
		if item.ID != id || item.DeliveryReceiptID != delivery.ID || item.SubjectDigest != request.SubjectDigest || item.Result != "PASS" || !item.Applicable || seenRequirements[item.RequirementID] || !verification.EvidenceMatchesPlan(plan, item) || len(item.ArtifactRefs) == 0 || !verification.EvidenceArtifactsBelongToDelivery(delivery, item) {
			return zero, fmt.Errorf("terminal Core readback: Evidence identity, requirement, issuer or artifact relationship invalid")
		}
		seenRequirements[item.RequirementID] = true
		evidence = append(evidence, item)
		procedures = append(procedures, TerminalProcedureObservation{RequirementID: item.RequirementID, Procedure: item.Procedure, Issuer: item.Issuer, EvidenceID: item.ID})
	}
	expectedVerification := verification.Evaluate(plan, request.SubjectDigest, evidence)
	expectedVerification.ID = verified.ID
	expectedVerification.DeliveryReceiptID = delivery.ID
	expectedVerification.VerificationPlanDigest = planDigest
	expectedVerification.Verifier = verified.Verifier
	expectedVerification.CreatedAt = verified.CreatedAt
	if !reflect.DeepEqual(expectedVerification, verified) {
		return zero, fmt.Errorf("terminal Core readback: Verification does not reproduce from the frozen plan and exact Evidence")
	}
	var reviewed review.Report
	if err := r.get(ctx, "reviews", closure.ReviewReportID, &reviewed); err != nil {
		return zero, err
	}
	if reviewed.Validate() != nil || reviewed.ID != closure.ReviewReportID || reviewed.DeliveryReceiptID != delivery.ID || reviewed.VerificationReportID != verified.ID || reviewed.TaskContractDigest != taskDigest || reviewed.SubjectDigest != request.SubjectDigest || reviewed.Result != review.ResultPass || reviewed.Reviewer == verified.Verifier || reviewed.Reviewer == work.HumanOwner || reviewed.CreatedAt.Before(verified.CreatedAt) || closure.CreatedAt.Before(reviewed.CreatedAt) {
		return zero, fmt.Errorf("terminal Core readback: independent Core Review or Closure relationship invalid")
	}
	var reread core.ClosureReceipt
	if err := r.get(ctx, "closures", request.ClosureID, &reread); err != nil {
		return zero, err
	}
	if !reflect.DeepEqual(closure, reread) {
		return zero, fmt.Errorf("terminal Core readback: immutable Closure changed during readback")
	}
	terminalPlan, err := BuildTerminalPlan()
	if err != nil {
		return zero, fmt.Errorf("terminal Core readback: terminal diagnostic plan invalid")
	}
	report := TerminalReadbackReport{
		Version: TerminalReadbackVersion, Status: TerminalCoreVerified,
		FixtureID: terminalPlan.Plan.FixtureID, TerminalPlanDigest: terminalPlan.PlanDigest,
		ReaderSubject: r.subject, Request: request, TaskContractDigest: taskDigest,
		VerificationPlanDigest: planDigest, RunInputManifestDigest: inputDigest,
		VerificationReportID: verified.ID, ReviewReportID: reviewed.ID,
		CoreReviewPrincipalsDiffer: true, Procedures: procedures, Records: r.records,
	}
	for _, gate := range terminalPlan.Plan.RequiredGates {
		observation := TerminalGateObservation{Gate: gate, Observation: "NOT_OBSERVED", Reason: "existing maintenance Core records do not establish this operational gate"}
		switch gate {
		case "verification_pass":
			observation.Observation, observation.Reason = "OBSERVED", "stored PASS Verification reproduces from the exact frozen three-requirement plan and Evidence"
		case "closure_created":
			observation.Observation, observation.Reason = "OBSERVED", "immutable Closure is present and its complete Core relationships were checked and reread"
		case "independent_human_review_pass":
			observation.Reason = "stored reviewer differs from verifier and work-owner; this does not establish a real independent human acceptance"
		case "exact_pr_head_ci":
			observation.Reason = "accepted trusted-CI Evidence reference is present; original PR event, exact head and artifact source bytes were not revalidated"
		case "source_verified_slo_evidence_accepted":
			observation.Reason = "SLO source verification and operational Evidence acceptance are absent from the frozen maintenance plan; Closure cannot accept itself"
		}
		report.Gates = append(report.Gates, observation)
	}
	if err := ctx.Err(); err != nil {
		return zero, &terminalReadbackError{"canceled or deadline exceeded", err}
	}
	return report, nil
}

func terminalPrincipal(value string) bool {
	return value != "" && len(value) <= 256 && strings.TrimSpace(value) == value && !strings.ContainsAny(value, "\r\n\t\x00")
}

func terminalArtifacts(artifacts []core.ArtifactRef) bool {
	if len(artifacts) == 0 || len(artifacts) > 64 {
		return false
	}
	seen := map[string]bool{}
	for _, artifact := range artifacts {
		if !terminalReadbackID.MatchString(artifact.ID) || !canonical.ValidDigest(artifact.Digest) || seen[artifact.ID] {
			return false
		}
		seen[artifact.ID] = true
	}
	return true
}

// These are the existing terminal-maintenance template's requirements, not new
// evidence procedures or a new acceptance authority. A future template requires
// an explicit reader version change rather than silently broadening coverage.
func terminalFrozenMaintenance(task core.TaskContract, plan verification.Plan) bool {
	if task.TaskType != "RELEASE" || task.Repository != "jiying2007/engineering-platform" || task.TargetID != "engineering-platform-production-qualification" || task.Revision == 0 || !terminalReadbackID.MatchString(task.ID) || plan.ID != "production-terminal-maintenance-vp" || len(plan.Criteria) != 3 || !reflect.DeepEqual(task.AllowedActions, []string{"worker.codex-execute", "github.publish-pr"}) || !reflect.DeepEqual(task.ExpectedOutputs, []string{
		"Only examples/production/terminal-maintenance/qualification.txt changes",
		"Exact marker bytes change from PRE_LIVE_READY to TERMINAL_QUALIFIED",
		"No product behavior, Core authority, provider policy or CI policy changes",
	}) {
		return false
	}
	expected := map[string]struct{ requirement, procedure, issuer, statement string }{
		"terminal-fixture-change":   {"terminal-ci", "github.actions.trusted-ci.v1", "github-actions-importer", "The retained result changes only examples/production/terminal-maintenance/qualification.txt from PRE_LIVE_READY to TERMINAL_QUALIFIED and changes no other file."},
		"terminal-unattended-codex": {"terminal-codex", "codex.core.execution.v1", "codex-execution-importer", "The retained result is produced by one authorized Core-bound Codex engineering execution using the exact selected and live-qualified unattended Provider v3 identity."},
		"terminal-git-ci":           {"terminal-git", "git.changed-tree.v1", "git-change-importer", "The retained Git changed-tree evidence binds the exact frozen base and result commit, and exact PR-head trusted CI passes."},
	}
	for _, criterion := range plan.Criteria {
		want, ok := expected[criterion.ID]
		if !ok || criterion.Statement != want.statement || len(criterion.Requirements) != 1 || criterion.Requirements[0] != (verification.EvidenceRequirement{ID: want.requirement, Procedure: want.procedure, Issuer: want.issuer}) {
			return false
		}
		delete(expected, criterion.ID)
	}
	return len(expected) == 0
}
