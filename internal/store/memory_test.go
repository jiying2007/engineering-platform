package store

import (
	"errors"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/core"
	"github.com/jiying2007/engineering-platform/internal/recovery"
	"github.com/jiying2007/engineering-platform/internal/run"
	"github.com/jiying2007/engineering-platform/internal/session"
	"github.com/jiying2007/engineering-platform/internal/verification"
)

func planFor(statement, procedure string) verification.Plan {
	return verification.Plan{
		ID: "vp-1",
		Criteria: []verification.Criterion{{
			ID:        "ac-1",
			Statement: statement,
			Requirements: []verification.EvidenceRequirement{{
				ID:        "req-1",
				Procedure: procedure,
			}},
		}},
	}
}

func bindPlan(t *testing.T, task *core.TaskContract, plan verification.Plan) {
	t.Helper()
	digest, err := plan.Digest()
	if err != nil {
		t.Fatal(err)
	}
	task.VerificationPlanID = plan.ID
	task.VerificationPlanDigest = digest
}

func TestTaskRevisionsAreImmutableAndMonotonic(t *testing.T) {
	s := NewMemory()
	r1 := core.TaskContract{
		ID:                 "task-1",
		WorkItemID:         "work-1",
		TaskType:           "FEATURE",
		Repository:         "repo",
		BaseCommit:         "0123456789abcdef0123456789abcdef01234567",
		AcceptanceCriteria: []string{"A"},
		Revision:           1,
	}
	p1 := planFor("A", "ci.test")
	bindPlan(t, &r1, p1)
	if err := s.CreateTask(r1, p1); err != nil {
		t.Fatal(err)
	}
	d1, _ := r1.Digest()

	r2 := r1
	r2.Revision = 2
	r2.AcceptanceCriteria = []string{"A", "B"}
	p2 := verification.Plan{
		ID: "vp-2",
		Criteria: []verification.Criterion{
			{ID: "ac-1", Statement: "A", Requirements: []verification.EvidenceRequirement{{ID: "req-1", Procedure: "ci.test"}}},
			{ID: "ac-2", Statement: "B", Requirements: []verification.EvidenceRequirement{{ID: "req-2", Procedure: "review.test"}}},
		},
	}
	bindPlan(t, &r2, p2)
	if err := s.CreateTask(r2, p2); err != nil {
		t.Fatal(err)
	}
	d2, _ := r2.Digest()
	if d1 == d2 {
		t.Fatal("expected revision digest to change")
	}

	latest, err := s.GetTask("task-1")
	if err != nil {
		t.Fatal(err)
	}
	if latest.Revision != 2 {
		t.Fatalf("expected latest revision 2, got %d", latest.Revision)
	}
	old, err := s.GetTaskByDigest(d1)
	if err != nil {
		t.Fatal(err)
	}
	if old.Revision != 1 {
		t.Fatalf("historical digest should still resolve revision 1, got %d", old.Revision)
	}

	r4 := r2
	r4.Revision = 4
	if err := s.CreateTask(r4, p2); !errors.Is(err, ErrConflict) {
		t.Fatalf("expected revision gap conflict, got %v", err)
	}
}

func TestTaskRejectsPlanDigestMismatch(t *testing.T) {
	s := NewMemory()
	task := core.TaskContract{
		ID: "task-1", WorkItemID: "work-1", TaskType: "FEATURE",
		Repository: "repo", BaseCommit: "0123456789abcdef0123456789abcdef01234567",
		AcceptanceCriteria: []string{"A"}, Revision: 1,
		VerificationPlanID: "vp-1", VerificationPlanDigest: "sha256:wrong",
	}
	plan := planFor("A", "ci.test")
	if err := s.CreateTask(task, plan); !errors.Is(err, ErrConflict) {
		t.Fatalf("expected plan digest mismatch conflict, got %v", err)
	}
}

func TestExecutionUpdateUsesOptimisticConcurrency(t *testing.T) {
	s := NewMemory()
	r := run.New("run-1", "sha256:task", "sha256:input")
	attempt, err := r.StartAttempt("attempt-1", time.Unix(1, 0))
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(r.ID, attempt.Epoch)
	input := core.RunInputManifest{
		RunID:              r.ID,
		TaskContractDigest: r.TaskContractDigest,
		RuntimeProfile:     "codex/default",
		ToolProfile:        "tools/m1",
		WorkerProfile:      "worker/ubuntu",
		PolicyProfile:      "policy/m1",
	}
	inputDigest, err := input.Digest()
	if err != nil {
		t.Fatal(err)
	}
	r.RunInputManifestDigest = inputDigest
	if err := s.CreateExecution(*r, *sess, input); err != nil {
		t.Fatal(err)
	}

	first, firstSession, err := s.GetExecution("run-1")
	if err != nil {
		t.Fatal(err)
	}
	stale, staleSession, err := s.GetExecution("run-1")
	if err != nil {
		t.Fatal(err)
	}

	if err := first.Pause(first.CurrentEpoch); err != nil {
		t.Fatal(err)
	}
	if err := firstSession.Pause(first.CurrentEpoch); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateExecution(first.ID, first.Version, first, firstSession); err != nil {
		t.Fatal(err)
	}

	if err := stale.Resume(stale.CurrentEpoch); err != nil {
		t.Fatal(err)
	}
	if err := staleSession.Resume(stale.CurrentEpoch); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateExecution(stale.ID, stale.Version, stale, staleSession); !errors.Is(err, ErrConflict) {
		t.Fatalf("expected optimistic concurrency conflict, got %v", err)
	}
}

func TestRecoveryStateUsesEpochCASAndRequiresReconciliation(t *testing.T) {
	s := NewMemory()
	initial := s.GetRecovery()
	if initial.Epoch != 0 || initial.Mode != recovery.Normal {
		t.Fatalf("unexpected initial recovery state: %#v", initial)
	}

	active, err := s.BeginRecovery(0)
	if err != nil {
		t.Fatal(err)
	}
	if active.Epoch != 1 || active.Mode != recovery.RecoveryReconciliation {
		t.Fatalf("unexpected active recovery state: %#v", active)
	}

	if _, err := s.BeginRecovery(0); !errors.Is(err, ErrConflict) {
		t.Fatalf("expected stale begin conflict, got %v", err)
	}
	if _, err := s.BeginRecovery(1); !errors.Is(err, recovery.ErrAlreadyRecovering) {
		t.Fatalf("expected duplicate recovery begin rejection, got %v", err)
	}
	if _, err := s.CompleteRecovery(1, false); !errors.Is(err, recovery.ErrReconciliationRequired) {
		t.Fatalf("expected reconciliation requirement, got %v", err)
	}
	completed, err := s.CompleteRecovery(1, true)
	if err != nil {
		t.Fatal(err)
	}
	if completed.Mode != recovery.Normal || completed.Epoch != 1 {
		t.Fatalf("unexpected completed recovery state: %#v", completed)
	}
}


func TestCreateTaskAndUpdateWorkIsAtomicOnStaleVersion(t *testing.T) {
	s := NewMemory()
	work := core.WorkItem{
		ID: "work-atomic-task", Title: "atomic task", HumanOwner: "owner",
		State: core.WorkDraft, Version: 1, CreatedAt: time.Unix(1, 0),
	}
	if err := s.CreateWork(work); err != nil {
		t.Fatal(err)
	}
	plan := planFor("A", "ci.test")
	task := core.TaskContract{
		ID: "task-atomic", WorkItemID: work.ID, TaskType: "FEATURE",
		Repository: "repo", BaseCommit: "0123456789abcdef0123456789abcdef01234567",
		AcceptanceCriteria: []string{"A"}, Revision: 1,
	}
	bindPlan(t, &task, plan)
	taskDigest, _ := task.Digest()

	desired := work
	desired.ActiveTaskContractDigest = taskDigest
	if err := desired.Transition(core.WorkReady); err != nil {
		t.Fatal(err)
	}

	if err := s.CreateTaskAndUpdateWork(task, plan, 0, desired); !errors.Is(err, ErrConflict) {
		t.Fatalf("expected stale work conflict, got %v", err)
	}
	if _, err := s.GetTask(task.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("task survived failed atomic mutation: %v", err)
	}
	unchanged, err := s.GetWork(work.ID)
	if err != nil {
		t.Fatal(err)
	}
	if unchanged.State != core.WorkDraft || unchanged.ActiveTaskContractDigest != "" {
		t.Fatalf("work changed during failed atomic mutation: %#v", unchanged)
	}

	if err := s.CreateTaskAndUpdateWork(task, plan, 1, desired); err != nil {
		t.Fatal(err)
	}
	ready, err := s.GetWork(work.ID)
	if err != nil {
		t.Fatal(err)
	}
	if ready.State != core.WorkReady || ready.ActiveTaskContractDigest != taskDigest || ready.Version != 2 {
		t.Fatalf("unexpected ready work: %#v", ready)
	}
}

func TestCreateAndCompleteExecutionAreAtomicWithWork(t *testing.T) {
	s := NewMemory()
	work := core.WorkItem{
		ID: "work-atomic-run", Title: "atomic run", HumanOwner: "owner",
		State: core.WorkDraft, Version: 1, CreatedAt: time.Unix(1, 0),
	}
	if err := s.CreateWork(work); err != nil {
		t.Fatal(err)
	}
	plan := planFor("A", "ci.test")
	task := core.TaskContract{
		ID: "task-atomic-run", WorkItemID: work.ID, TaskType: "FEATURE",
		Repository: "repo", BaseCommit: "0123456789abcdef0123456789abcdef01234567",
		AcceptanceCriteria: []string{"A"}, Revision: 1,
	}
	bindPlan(t, &task, plan)
	taskDigest, _ := task.Digest()
	readyWork := work
	readyWork.ActiveTaskContractDigest = taskDigest
	if err := readyWork.Transition(core.WorkReady); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateTaskAndUpdateWork(task, plan, 1, readyWork); err != nil {
		t.Fatal(err)
	}
	readyWork, _ = s.GetWork(work.ID)

	input := core.RunInputManifest{
		RunID: "run-atomic", TaskContractDigest: taskDigest,
		RuntimeProfile: "codex/default", ToolProfile: "tools/m1",
		WorkerProfile: "worker/ubuntu", PolicyProfile: "policy/m1",
	}
	inputDigest, _ := input.Digest()
	value := run.New(input.RunID, taskDigest, inputDigest)
	attempt, err := value.StartAttempt("attempt-1", time.Unix(2, 0))
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(value.ID, attempt.Epoch)
	executingWork := readyWork
	executingWork.ActiveRunID = value.ID
	if err := executingWork.Transition(core.WorkExecuting); err != nil {
		t.Fatal(err)
	}

	if err := s.CreateExecutionAndUpdateWork(*value, attempt, *sess, input, readyWork.Version-1, executingWork); !errors.Is(err, ErrConflict) {
		t.Fatalf("expected stale work conflict, got %v", err)
	}
	if _, _, err := s.GetExecution(value.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("run survived failed atomic create: %v", err)
	}
	if _, err := s.GetAttempt(value.ID, attempt.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("attempt survived failed atomic create: %v", err)
	}

	if err := s.CreateExecutionAndUpdateWork(*value, attempt, *sess, input, readyWork.Version, executingWork); err != nil {
		t.Fatal(err)
	}
	persistedAttempt, err := s.GetAttempt(value.ID, attempt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persistedAttempt.Epoch != attempt.Epoch {
		t.Fatalf("unexpected attempt: %#v", persistedAttempt)
	}

	currentRun, currentSession, err := s.GetExecution(value.ID)
	if err != nil {
		t.Fatal(err)
	}
	currentWork, err := s.GetWork(work.ID)
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
		value.ID,
		currentRun.Version,
		completedRun,
		currentSession,
		currentWork.Version-1,
		verifyingWork,
	); !errors.Is(err, ErrConflict) {
		t.Fatalf("expected stale completion conflict, got %v", err)
	}
	stillRunning, _, _ := s.GetExecution(value.ID)
	stillExecuting, _ := s.GetWork(work.ID)
	if stillRunning.State != run.Running || stillExecuting.State != core.WorkExecuting {
		t.Fatalf("failed completion partially persisted run=%s work=%s", stillRunning.State, stillExecuting.State)
	}

	if err := s.UpdateExecutionAndWork(
		value.ID,
		currentRun.Version,
		completedRun,
		currentSession,
		currentWork.Version,
		verifyingWork,
	); err != nil {
		t.Fatal(err)
	}
	finished, _, _ := s.GetExecution(value.ID)
	verifying, _ := s.GetWork(work.ID)
	if finished.State != run.Completed || verifying.State != core.WorkVerifying {
		t.Fatalf("atomic completion failed run=%s work=%s", finished.State, verifying.State)
	}
}
