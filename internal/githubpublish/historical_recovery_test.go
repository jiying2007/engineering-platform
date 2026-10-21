package githubpublish

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/action"
	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/run"
)

func TestProviderFencesOriginalUninvokedPublicationAfterRunAbort(t *testing.T) {
	provider, state, config, upstream, service := independentProviderFixture(t)
	plan, _, err := provider.derive(context.Background(), state.run.ID, state.run.CurrentEpoch, "", false)
	if err != nil {
		t.Fatal(err)
	}
	request := action.Request{ID: "original-uninvoked-action", RunID: state.run.ID, ExecutionEpoch: state.run.CurrentEpoch,
		Action: Action, Capability: Capability, RiskClass: action.ControlledMutation,
		IdempotencyKey: "original-uninvoked-key", ParametersDigest: state.status.Receipt.ResultDigest, RequestedBy: "runtime"}
	digest, err := action.RequestDigest(request)
	if err != nil {
		t.Fatal(err)
	}
	op := *action.NewWithRequestDigest(request, digest, time.Now().UTC())
	op.State = action.Unknown
	identity, err := IdentityForOperation(op)
	if err != nil {
		t.Fatal(err)
	}
	// Model only the v2 pre-invocation stop: the original lease closes without
	// entering Publish, and the immutable Publisher slot remains intact.
	lease, start, err := service.journal.begin(context.Background(), identity, plan)
	if err != nil {
		t.Fatal(err)
	}
	if err := lease.Close(); err != nil {
		t.Fatal(err)
	}
	if err := state.run.Abort(state.run.CurrentEpoch); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(config.ArtifactRoot, state.status.Token.ID+".bundle")); err != nil {
		t.Fatal(err)
	}
	evidence, err := provider.FenceUninvokedPublication(context.Background(), op)
	if err != nil || evidence.Validate() != nil || evidence.Start != start {
		t.Fatalf("valid original uninvoked evidence rejected after abort: %+v %v", evidence, err)
	}
	if upstream.publishCalls != 0 || upstream.observeCalls != 0 {
		t.Fatal("non-invocation evidence caused external work")
	}
}

func TestProviderRecoversOriginalPublicationAfterRunTerminationWithoutReplay(t *testing.T) {
	for _, terminal := range []run.State{run.Completed, run.Aborted} {
		t.Run(string(terminal), func(t *testing.T) {
			provider, state, config, upstream, request, op := publishedProviderFixture(t)
			// The original publication and sealed execution remain immutable. Only
			// the Run's lifecycle has ended, and its local bundle is no longer kept.
			op.State = action.Confirmed
			var err error
			if terminal == run.Completed {
				err = state.run.Complete(state.run.CurrentEpoch)
			} else {
				err = state.run.Abort(state.run.CurrentEpoch)
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(filepath.Join(config.ArtifactRoot, state.status.Token.ID+".bundle")); err != nil {
				t.Fatal(err)
			}
			evidence, err := provider.ObserveOriginalPublication(context.Background(), op)
			if err != nil || evidence.ValidateForRecovery() != nil || evidence.Start.Identity.ExecutionEpoch != op.ExecutionEpoch || evidence.Start.Identity.RequestDigest != op.RequestDigest {
				t.Fatalf("valid terminal-Run evidence rejected: %+v %v", evidence, err)
			}
			// Historical evidence cannot restore current dispatch authority.
			result, err := provider.Dispatch(context.Background(), request)
			if err != nil || result.Outcome != action.DispatchUnknown || result.ObservedState != "PRECONDITION_FAILED" {
				t.Fatalf("terminal Run regained dispatch authority: %+v %v", result, err)
			}
			if _, err := provider.FenceUninvokedPublication(context.Background(), op); err == nil {
				t.Fatal("original confirmed publication admitted as uninvoked")
			}
			if upstream.publishCalls != 1 || upstream.observeCalls != 1 {
				t.Fatalf("historical readback caused new effects: publish=%d observe=%d", upstream.publishCalls, upstream.observeCalls)
			}
		})
	}
}

func TestProviderReadsOriginalEpochWithoutGrantingCurrentDispatch(t *testing.T) {
	provider, state, _, upstream, originalRequest, original := publishedProviderFixture(t)
	if _, err := state.run.StartAttempt("later-attempt", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	// The newer live Run epoch cannot adopt a sealed result from the original
	// execution. The bundle still exists, so rejection proves the epoch fence.
	for _, epoch := range []uint64{original.ExecutionEpoch, state.run.CurrentEpoch} {
		request := originalRequest
		request.ExecutionEpoch = epoch
		result, err := provider.Dispatch(context.Background(), request)
		if err != nil || result.Outcome != action.DispatchUnknown || result.ObservedState != "PRECONDITION_FAILED" {
			t.Fatalf("old sealed result regained dispatch authority at epoch %d: %+v %v", epoch, result, err)
		}
	}
	diagnostic, err := provider.Reconcile(context.Background(), original)
	if err != nil || diagnostic.Outcome != action.ReconcileManual || upstream.observeCalls != 0 {
		t.Fatalf("ordinary observation bypassed current epoch: %+v %v", diagnostic, err)
	}
	evidence, err := provider.ObserveOriginalPublication(context.Background(), original)
	if err != nil || evidence.ValidateForRecovery() != nil || evidence.Start.Identity.ExecutionEpoch != original.ExecutionEpoch {
		t.Fatalf("original epoch evidence rejected after Run advancement: %+v %v", evidence, err)
	}
	if upstream.publishCalls != 1 || upstream.observeCalls != 1 {
		t.Fatalf("historical epoch readback caused new effects: publish=%d observe=%d", upstream.publishCalls, upstream.observeCalls)
	}
}

func TestHistoricalPublicationRejectsChangedOriginalBindings(t *testing.T) {
	provider, state, config, upstream, _, original := publishedProviderFixture(t)
	if err := state.run.Complete(state.run.CurrentEpoch); err != nil {
		t.Fatal(err)
	}
	original.State = action.Confirmed
	for _, tc := range []struct {
		name   string
		change func(*publisherState, *action.Operation, *Configuration)
	}{
		{"future-epoch", func(_ *publisherState, op *action.Operation, _ *Configuration) { op.ExecutionEpoch++ }},
		{"wrong-original-epoch", func(s *publisherState, op *action.Operation, _ *Configuration) {
			s.run.CurrentEpoch++
			op.ExecutionEpoch = s.run.CurrentEpoch
		}},
		{"changed-task-bytes", func(s *publisherState, _ *action.Operation, _ *Configuration) { s.task.ID = "substituted-task" }},
		{"missing-transcript", func(s *publisherState, _ *action.Operation, _ *Configuration) { s.status.Runtime.Transcript = nil }},
		{"unsealed-runtime", func(s *publisherState, _ *action.Operation, _ *Configuration) { s.status.Runtime.State = "OPEN" }},
		{"changed-bound-turn", func(s *publisherState, _ *action.Operation, _ *Configuration) {
			s.status.Runtime.Binding.TurnID = "another-turn"
		}},
		{"changed-receipt", func(s *publisherState, _ *action.Operation, _ *Configuration) {
			s.status.Receipt.Worker = "urn:engineering-platform:worker:another"
		}},
		{"changed-valid-result", func(s *publisherState, _ *action.Operation, _ *Configuration) {
			s.status.Receipt.Result.Change.ResultCommit = strings.Repeat("9", 40)
			s.status.Receipt.ResultDigest, _ = canonical.Digest(s.status.Receipt.Result)
		}},
		{"changed-original-key", func(_ *publisherState, op *action.Operation, _ *Configuration) { op.IdempotencyKey = "another-key" }},
		{"changed-original-digest", func(_ *publisherState, op *action.Operation, _ *Configuration) {
			op.RequestDigest = digestOf("another-request")
		}},
		{"changed-original-id", func(_ *publisherState, op *action.Operation, _ *Configuration) { op.ID = "another-operation" }},
		{"changed-target", func(_ *publisherState, _ *action.Operation, c *Configuration) { c.Targets[0].BaseRef = "another-base" }},
		{"unadmitted-planned-state", func(_ *publisherState, op *action.Operation, _ *Configuration) { op.State = action.Planned }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := publisherState{run: state.run, task: state.task}
			raw, err := json.Marshal(state.status)
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(raw, &changed.status); err != nil {
				t.Fatal(err)
			}
			changedConfig := config
			changedConfig.Targets = append([]TargetPolicy(nil), config.Targets...)
			op := original
			tc.change(&changed, &op, &changedConfig)
			candidate, err := New(changedConfig, &changed, provider.remote)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := candidate.ObserveOriginalPublication(context.Background(), op); err == nil {
				t.Fatal("changed original binding accepted")
			}
			if upstream.publishCalls != 1 || upstream.observeCalls != 0 {
				t.Fatalf("invalid original identity reached upstream: publish=%d observe=%d", upstream.publishCalls, upstream.observeCalls)
			}
		})
	}
}
