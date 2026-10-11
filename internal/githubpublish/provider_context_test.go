package githubpublish

import (
	"context"
	"errors"
	"testing"

	"github.com/jiying2007/engineering-platform/internal/action"
	"github.com/jiying2007/engineering-platform/internal/core"
	"github.com/jiying2007/engineering-platform/internal/run"
	"github.com/jiying2007/engineering-platform/internal/session"
)

type publisherCancellationState struct {
	publisherState
	cancel      context.CancelFunc
	frozenReads int
}

func (s *publisherCancellationState) GetExecutionContext(ctx context.Context, id string) (run.Run, session.Session, error) {
	s.frozenReads++
	value, sess, err := s.publisherState.GetExecutionContext(ctx, id)
	if err == nil && s.cancel != nil {
		s.cancel() // Simulate caller loss between two frozen-Core reads.
	}
	return value, sess, err
}

func (s *publisherCancellationState) GetTaskByDigestContext(ctx context.Context, digest string) (core.TaskContract, error) {
	return s.publisherState.GetTaskByDigestContext(ctx, digest)
}

func TestCancelledPublisherRequestCannotReachGitHubRemote(t *testing.T) {
	state, config := publisherFixture(t)
	remote := &publisherRemote{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	provider, err := New(config, state, remote)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Dispatch(ctx, action.Request{
		RunID: state.run.ID, ExecutionEpoch: state.run.CurrentEpoch,
		Action: Action, Capability: Capability, RiskClass: action.ControlledMutation,
		ParametersDigest: state.status.Receipt.ResultDigest,
	}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled publication did not return caller cancellation: %v", err)
	}
	if _, err := provider.Reconcile(ctx, action.Operation{
		RunID: state.run.ID, ExecutionEpoch: state.run.CurrentEpoch,
		Action: Action, Capability: Capability, RiskClass: action.ControlledMutation,
	}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled observation did not return caller cancellation: %v", err)
	}
	if remote.publishCalls != 0 || remote.observeCalls != 0 {
		t.Fatalf("cancelled request reached external Provider: publish=%d observe=%d", remote.publishCalls, remote.observeCalls)
	}
}

func TestCancellationDuringFrozenReadStopsPublisherBeforeSideEffect(t *testing.T) {
	state, config := publisherFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	scoped := &publisherCancellationState{publisherState: state, cancel: cancel}
	remote := &publisherRemote{}
	provider, err := New(config, scoped, remote)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Dispatch(ctx, action.Request{
		RunID: state.run.ID, ExecutionEpoch: state.run.CurrentEpoch,
		Action: Action, Capability: Capability, RiskClass: action.ControlledMutation,
		ParametersDigest: state.status.Receipt.ResultDigest,
	}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled frozen Core read reached publisher: %v", err)
	}
	if scoped.frozenReads != 1 || remote.publishCalls != 0 || remote.observeCalls != 0 {
		t.Fatalf("request cancellation failed closed: reads=%d publish=%d observe=%d", scoped.frozenReads, remote.publishCalls, remote.observeCalls)
	}
}
