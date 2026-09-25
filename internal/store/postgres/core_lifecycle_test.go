package postgres

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/core"
	"github.com/jiying2007/engineering-platform/internal/review"
	"github.com/jiying2007/engineering-platform/internal/run"
	"github.com/jiying2007/engineering-platform/internal/session"
	"github.com/jiying2007/engineering-platform/internal/verification"
)

func TestPostgresCoreLifecycleVerticalSlice(t *testing.T) {
	url := os.Getenv("POSTGRES_TEST_URL")
	if url == "" {
		t.Skip("POSTGRES_TEST_URL is not set")
	}

	ctx := context.Background()
	s, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.ApplyCoreMigration(ctx); err != nil {
		t.Fatal(err)
	}

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	workID := "work-pg-" + suffix
	taskID := "task-pg-" + suffix
	runID := "run-pg-" + suffix
	deliveryID := "delivery-pg-" + suffix

	work := core.WorkItem{
		ID:         workID,
		Title:      "PostgreSQL vertical slice",
		HumanOwner: "integration-test",
		TargetID:   "target-pg",
		State:      core.WorkDraft,
		Version:    1,
		CreatedAt:  time.Now().UTC(),
	}
	if err := s.CreateWork(work); err != nil {
		t.Fatal(err)
	}

	plan := verification.Plan{
		ID: "vp-" + suffix,
		Criteria: []verification.Criterion{{
			ID:        "ac-1",
			Statement: "CI passes",
			Requirements: []verification.EvidenceRequirement{{
				ID:        "req-1",
				Procedure: "ci.test",
				Issuer:    "ci",
			}},
		}},
	}
	planDigest, err := plan.Digest()
	if err != nil {
		t.Fatal(err)
	}
	task := core.TaskContract{
		ID:                     taskID,
		WorkItemID:             workID,
		TaskType:               "FEATURE",
		CapabilityIDs:          []string{"embedded.driver-component"},
		SkillIDs:               []string{"driver-integration-review"},
		Repository:             "jiying2007/example",
		BaseCommit:             "0123456789abcdef0123456789abcdef01234567",
		TargetID:               "target-pg",
		AcceptanceCriteria:     []string{"CI passes"},
		AllowedActions:         []string{"ci.dispatch"},
		ExpectedOutputs:        []string{"firmware"},
		VerificationPlanID:     plan.ID,
		VerificationPlanDigest: planDigest,
		Revision:               1,
	}
	taskDigest, err := task.Digest()
	if err != nil {
		t.Fatal(err)
	}
	readyWork := work
	readyWork.ActiveTaskContractDigest = taskDigest
	if err := readyWork.Transition(core.WorkReady); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateTaskAndUpdateWork(task, plan, work.Version, readyWork); err != nil {
		t.Fatal(err)
	}

	persistedTask, err := s.GetTaskByDigest(taskDigest)
	if err != nil {
		t.Fatal(err)
	}
	if persistedTask.VerificationPlanDigest != planDigest {
		t.Fatalf("task/plan binding mismatch: %#v", persistedTask)
	}
	persistedPlan, err := s.GetVerificationPlanByDigest(planDigest)
	if err != nil {
		t.Fatal(err)
	}
	if persistedPlan.ID != plan.ID {
		t.Fatalf("unexpected persisted plan: %#v", persistedPlan)
	}

	readyWork, err = s.GetWork(workID)
	if err != nil {
		t.Fatal(err)
	}
	input := core.RunInputManifest{
		RunID:              runID,
		TaskContractDigest: taskDigest,
		ContextRefs:        []core.ContextRef{{Source: "doc:datasheet", Type: "DOCUMENT", Version: "r1", Digest: canonical.BytesDigest([]byte("datasheet")), Trust: core.ContextApproved}},
		RuntimeProfile:     "codex/default",
		ToolProfile:        "tools/m1",
		WorkerProfile:      "worker/ubuntu",
		PolicyProfile:      "policy/m1",
	}
	inputDigest, err := input.Digest()
	if err != nil {
		t.Fatal(err)
	}
	value := run.New(runID, taskDigest, inputDigest)
	attempt, err := value.StartAttempt("attempt-1", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(runID, attempt.Epoch)

	executingWork := readyWork
	executingWork.ActiveRunID = runID
	if err := executingWork.Transition(core.WorkExecuting); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateExecutionAndUpdateWork(
		*value,
		attempt,
		*sess,
		input,
		readyWork.Version,
		executingWork,
	); err != nil {
		t.Fatal(err)
	}

	persistedAttempt, err := s.GetAttempt(runID, attempt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persistedAttempt.Epoch != attempt.Epoch || !persistedAttempt.EndedAt.IsZero() {
		t.Fatalf("unexpected persisted attempt: %#v", persistedAttempt)
	}
	persistedInput, err := s.GetRunInputByDigest(inputDigest)
	if err != nil {
		t.Fatal(err)
	}
	if persistedInput.RuntimeProfile != input.RuntimeProfile {
		t.Fatalf("unexpected run input: %#v", persistedInput)
	}
	persistedInputDigest, err := persistedInput.Digest()
	if err != nil || persistedInputDigest != inputDigest || len(persistedInput.ContextRefs) != 1 || persistedInput.ContextRefs[0] != input.ContextRefs[0] {
		t.Fatalf("structured context identity changed in PostgreSQL: %#v digest=%s err=%v", persistedInput, persistedInputDigest, err)
	}

	currentRun, currentSession, err := s.GetExecution(runID)
	if err != nil {
		t.Fatal(err)
	}
	cmd := session.SteeringCommand{
		ID:             "steer-" + suffix,
		RunID:          runID,
		ExecutionEpoch: currentRun.CurrentEpoch,
		Sequence:       1,
		Actor:          "engineer",
		ContentDigest:  "sha256:steer",
		CreatedAt:      time.Now().UTC(),
	}
	if err := currentSession.ApplySteering(cmd); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordSteering(
		runID,
		currentRun.Version,
		currentRun,
		currentSession,
		cmd,
	); err != nil {
		t.Fatal(err)
	}
	storedSteer, err := s.GetSteering(cmd.ID)
	if err != nil {
		t.Fatal(err)
	}
	if storedSteer.Sequence != 1 || storedSteer.ContentDigest != cmd.ContentDigest {
		t.Fatalf("unexpected persisted steering: %#v", storedSteer)
	}

	currentRun, currentSession, err = s.GetExecution(runID)
	if err != nil {
		t.Fatal(err)
	}
	checkpoint := session.Checkpoint{
		ID:                     "cp-" + suffix,
		RunID:                  runID,
		TaskContractDigest:     taskDigest,
		RunInputManifestDigest: inputDigest,
		ExecutionEpoch:         currentRun.CurrentEpoch,
		SourceTreeDigest:       "sha256:tree",
		DiffDigest:             "sha256:diff",
		Objective:              "finish integration",
		Completed:              []string{"implementation"},
		Pending:                []string{"verification"},
		LastEventSequence:      1,
		CreatedAt:              time.Now().UTC(),
	}
	checkpointDigest, err := s.CreateCheckpoint(checkpoint)
	if err != nil {
		t.Fatal(err)
	}
	storedCheckpoint, storedCheckpointDigest, err := s.GetCheckpoint(checkpoint.ID)
	if err != nil {
		t.Fatal(err)
	}
	if storedCheckpointDigest != checkpointDigest ||
		storedCheckpoint.RunInputManifestDigest != inputDigest {
		t.Fatalf("unexpected checkpoint: %#v digest=%s", storedCheckpoint, storedCheckpointDigest)
	}

	currentWork, err := s.GetWork(workID)
	if err != nil {
		t.Fatal(err)
	}
	completedRun := currentRun
	if err := completedRun.Complete(completedRun.CurrentEpoch); err != nil {
		t.Fatal(err)
	}
	verifyingWork := currentWork
	if err := verifyingWork.Transition(core.WorkVerifying); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateExecutionAndWork(
		runID,
		currentRun.Version,
		completedRun,
		currentSession,
		currentWork.Version,
		verifyingWork,
	); err != nil {
		t.Fatal(err)
	}

	delivery := core.DeliveryReceipt{
		ID:                 deliveryID,
		WorkItemID:         workID,
		TaskContractDigest: taskDigest,
		RunID:              runID,
		TargetID:           task.TargetID,
		BaseCommit:         task.BaseCommit,
		ResultCommit:       "1111111111111111111111111111111111111111",
		Artifacts: []core.ArtifactRef{{
			ID:        "firmware-" + suffix,
			Digest:    "sha256:firmware",
			MediaType: "application/octet-stream",
		}},
		CreatedAt: time.Now().UTC(),
	}
	delivery.SubjectDigest, err = delivery.CalculateSubjectDigest()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CreateDelivery(delivery); err != nil {
		t.Fatal(err)
	}
	storedDelivery, err := s.GetDelivery(delivery.ID)
	if err != nil {
		t.Fatal(err)
	}
	if storedDelivery.SubjectDigest != delivery.SubjectDigest {
		t.Fatalf("unexpected delivery: %#v", storedDelivery)
	}

	evidence := core.EvidenceRef{
		ID:                "evidence-" + suffix,
		DeliveryReceiptID: delivery.ID,
		RequirementID:     "req-1",
		SubjectDigest:     delivery.SubjectDigest,
		Issuer:            "ci",
		Procedure:         "ci.test",
		Result:            "PASS",
		ArtifactRefs:      []string{delivery.Artifacts[0].ID},
		Applicable:        true,
	}
	if err := s.CreateEvidence(evidence); err != nil {
		t.Fatal(err)
	}
	wrongRequirement := evidence
	wrongRequirement.ID += "-wrong-requirement"
	wrongRequirement.RequirementID = "req-other"
	if err := s.CreateEvidence(wrongRequirement); err == nil {
		t.Fatal("PostgreSQL store accepted evidence for a different frozen requirement")
	}
	wrongSubject := evidence
	wrongSubject.ID += "-wrong-subject"
	wrongSubject.SubjectDigest = "sha256:other"
	if err := s.CreateEvidence(wrongSubject); err == nil {
		t.Fatal("PostgreSQL store accepted evidence for a different delivery subject")
	}
	storedEvidence, err := s.GetEvidence(evidence.ID)
	if err != nil {
		t.Fatal(err)
	}
	if storedEvidence.SubjectDigest != delivery.SubjectDigest {
		t.Fatalf("unexpected evidence: %#v", storedEvidence)
	}

	report := verification.Evaluate(plan, delivery.SubjectDigest, []core.EvidenceRef{evidence})
	report.ID = "verification-" + suffix
	report.DeliveryReceiptID = delivery.ID
	report.VerificationPlanDigest = planDigest
	report.Verifier = "integration-verifier"
	report.CreatedAt = time.Now().UTC()
	if report.Result != "PASS" {
		t.Fatalf("expected PASS verification, got %#v", report)
	}
	if err := s.CreateVerification(report); err != nil {
		t.Fatal(err)
	}
	storedReport, err := s.GetVerification(report.ID)
	if err != nil {
		t.Fatal(err)
	}
	if storedReport.Result != "PASS" || storedReport.SubjectDigest != delivery.SubjectDigest {
		t.Fatalf("unexpected verification report: %#v", storedReport)
	}

	verifyingWork, err = s.GetWork(workID)
	if err != nil {
		t.Fatal(err)
	}
	reviewReport := review.Report{
		ID:                   "review-" + suffix,
		DeliveryReceiptID:    delivery.ID,
		VerificationReportID: report.ID,
		TaskContractDigest:   taskDigest,
		SubjectDigest:        delivery.SubjectDigest,
		Reviewer:             "integration-reviewer",
		Result:               review.ResultPass,
		KnownLimits:          []string{"integration host remains trusted"},
		CreatedAt:            time.Now().UTC(),
	}
	reviewingWork := verifyingWork
	if err := reviewingWork.Transition(core.WorkReviewing); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateReviewAndUpdateWork(reviewReport, verifyingWork.Version, reviewingWork); err != nil {
		t.Fatal(err)
	}
	storedReview, err := s.GetReview(reviewReport.ID)
	if err != nil {
		t.Fatal(err)
	}
	if storedReview.Result != review.ResultPass ||
		storedReview.SubjectDigest != delivery.SubjectDigest ||
		storedReview.VerificationReportID != report.ID {
		t.Fatalf("unexpected review report: %#v", storedReview)
	}

	reviewingWork, err = s.GetWork(workID)
	if err != nil {
		t.Fatal(err)
	}
	closedWork := reviewingWork
	if err := closedWork.Transition(core.WorkClosed); err != nil {
		t.Fatal(err)
	}
	closure := core.ClosureReceipt{
		ID:                   "closure-" + suffix,
		WorkItemID:           workID,
		TaskContractDigest:   taskDigest,
		RunID:                runID,
		DeliveryReceiptID:    delivery.ID,
		VerificationReportID: report.ID,
		ReviewReportID:       reviewReport.ID,
		SubjectDigest:        delivery.SubjectDigest,
		Result:               "CLOSED",
		CreatedAt:            time.Now().UTC(),
	}
	if err := s.CreateClosureAndUpdateWork(
		closure,
		reviewingWork.Version,
		closedWork,
	); err != nil {
		t.Fatal(err)
	}
	storedClosure, err := s.GetClosure(closure.ID)
	if err != nil {
		t.Fatal(err)
	}
	if storedClosure.SubjectDigest != delivery.SubjectDigest {
		t.Fatalf("unexpected closure: %#v", storedClosure)
	}
	finalWork, err := s.GetWork(workID)
	if err != nil {
		t.Fatal(err)
	}
	if finalWork.State != core.WorkClosed {
		t.Fatalf("expected CLOSED work, got %#v", finalWork)
	}

	var auditCount int
	if err := s.pool.QueryRow(
		ctx,
		"SELECT COUNT(*) FROM audit_events WHERE aggregate_id IN ($1,$2,$3,$4,$5,$6,$7)",
		workID, taskID, runID, checkpoint.ID, delivery.ID, report.ID, reviewReport.ID,
	).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if auditCount < 9 {
		t.Fatalf("expected lifecycle audit trail, got %d matching audit events", auditCount)
	}

	var outboxCount int
	if err := s.pool.QueryRow(
		ctx,
		"SELECT COUNT(*) FROM outbox_events WHERE outbox_key=$1",
		"run:"+runID+":started",
	).Scan(&outboxCount); err != nil {
		t.Fatal(err)
	}
	if outboxCount != 1 {
		t.Fatalf("expected one run.started outbox row, got %d", outboxCount)
	}
}
