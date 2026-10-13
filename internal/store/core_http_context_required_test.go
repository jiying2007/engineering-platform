package store

import (
	"context"
	"errors"
	"testing"

	"github.com/jiying2007/engineering-platform/internal/core"
	"github.com/jiying2007/engineering-platform/internal/recovery"
	"github.com/jiying2007/engineering-platform/internal/review"
	"github.com/jiying2007/engineering-platform/internal/run"
	"github.com/jiying2007/engineering-platform/internal/session"
	"github.com/jiying2007/engineering-platform/internal/verification"
)

var _ Store = (*Memory)(nil)

// All former HTTP fallback entrypoints must reject a cancelled caller
// before touching the single Core ledger. The production PostgreSQL
// implementation is separately exercised under isolated row locks in CI.
func TestMemoryCoreHTTPContractAlwaysHonorsPreCancelledCaller(t *testing.T) {
	m := NewMemory()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	checks := []struct {
		name string
		call func() error
	}{
		{"CreateWork", func() error { return m.CreateWorkContext(ctx, core.WorkItem{ID: "cancelled-work"}) }},
		{"GetWork", func() error { _, err := m.GetWorkContext(ctx, "cancelled-work"); return err }},
		{"UpdateWork", func() error { return m.UpdateWorkContext(ctx, "cancelled-work", 0, core.WorkItem{}) }},
		{"CreateTaskAndUpdateWork", func() error {
			return m.CreateTaskAndUpdateWorkContext(ctx, core.TaskContract{}, verification.Plan{}, 0, core.WorkItem{})
		}},
		{"GetTask", func() error { _, err := m.GetTaskContext(ctx, "missing-task"); return err }},
		{"GetTaskByDigest", func() error { _, err := m.GetTaskByDigestContext(ctx, "missing-digest"); return err }},
		{"GetVerificationPlanByDigest", func() error { _, err := m.GetVerificationPlanByDigestContext(ctx, "missing-plan"); return err }},
		{"CreateExecutionAndUpdateWork", func() error {
			return m.CreateExecutionAndUpdateWorkContext(ctx, run.Run{}, run.Attempt{}, session.Session{}, core.RunInputManifest{}, 0, core.WorkItem{})
		}},
		{"GetExecution", func() error { _, _, err := m.GetExecutionContext(ctx, "missing-run"); return err }},
		{"GetRunInputByDigest", func() error { _, err := m.GetRunInputByDigestContext(ctx, "missing-input"); return err }},
		{"UpdateExecution", func() error { return m.UpdateExecutionContext(ctx, "missing-run", 0, run.Run{}, session.Session{}) }},
		{"UpdateExecutionAndWork", func() error {
			return m.UpdateExecutionAndWorkContext(ctx, "missing-run", 0, run.Run{}, session.Session{}, 0, core.WorkItem{})
		}},
		{"CreateCheckpoint", func() error { _, err := m.CreateCheckpointContext(ctx, session.Checkpoint{}); return err }},
		{"GetCheckpoint", func() error { _, _, err := m.GetCheckpointContext(ctx, "missing-checkpoint"); return err }},
		{"CreateDelivery", func() error { return m.CreateDeliveryContext(ctx, core.DeliveryReceipt{}) }},
		{"GetDelivery", func() error { _, err := m.GetDeliveryContext(ctx, "missing-delivery"); return err }},
		{"CreateEvidence", func() error { return m.CreateEvidenceContext(ctx, core.EvidenceRef{}) }},
		{"GetEvidence", func() error { _, err := m.GetEvidenceContext(ctx, "missing-evidence"); return err }},
		{"CreateVerification", func() error { return m.CreateVerificationContext(ctx, verification.Report{}) }},
		{"GetVerification", func() error { _, err := m.GetVerificationContext(ctx, "missing-verification"); return err }},
		{"CreateReviewAndUpdateWork", func() error { return m.CreateReviewAndUpdateWorkContext(ctx, review.Report{}, 0, core.WorkItem{}) }},
		{"GetReview", func() error { _, err := m.GetReviewContext(ctx, "missing-review"); return err }},
		{"CreateClosureAndUpdateWork", func() error {
			return m.CreateClosureAndUpdateWorkContext(ctx, core.ClosureReceipt{}, 0, core.WorkItem{})
		}},
		{"GetClosure", func() error { _, err := m.GetClosureContext(ctx, "missing-closure"); return err }},
		{"GetSteering", func() error { _, err := m.GetSteeringContext(ctx, "missing-steering"); return err }},
	}
	for _, tc := range checks {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.call(); !errors.Is(err, context.Canceled) {
				t.Fatalf("Core HTTP operation bypassed caller cancellation: %v", err)
			}
		})
	}
	if _, err := m.GetWorkContext(context.Background(), "cancelled-work"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cancelled HTTP mutation left Core Work residue: %v", err)
	}
	state, err := m.GetRecoveryContext(context.Background())
	if err != nil || state.Epoch != 0 || state.Mode != recovery.Normal {
		t.Fatalf("cancelled HTTP operations changed Recovery authority: %+v %v", state, err)
	}
}
