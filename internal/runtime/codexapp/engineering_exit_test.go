package codexapp

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/controlclient"
	"github.com/jiying2007/engineering-platform/internal/processscope"
)

type exitContextKey struct{}
type exitController struct {
	invalidCloseContext bool
	*fixtureController
	closeCalls    int
	closeError    error
	closeContext  context.Context
	proof         processscope.Proof
	status        string
	reportError   error
	reportContext context.Context
	reportCalls   int
}

func (c *exitController) Close(ctx context.Context, status string, proof processscope.Proof) error {
	c.closeCalls++
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) <= 0 || time.Until(deadline) > 6*time.Second || ctx.Err() != nil {
		c.invalidCloseContext = true
	}
	c.closeContext = ctx
	c.status = status
	c.proof = proof
	return c.closeError
}
func (c *exitController) Report(ctx context.Context, id, outcome string) error {
	c.reportCalls++
	c.reportContext = ctx
	return c.reportError
}

// Unit-only proof fixture; no real process or provider qualification is claimed.
func exitTestProof() processscope.Proof {
	return processscope.Proof{Version: 1, Mechanism: processscope.Mechanism, HostPID: 123, NamespaceID: 2, ParentNamespaceID: 1, StartedAt: time.Unix(1, 0), ReapedAt: time.Unix(2, 0), InitReaped: true}
}
func assertExitContext(t *testing.T, ctx context.Context, budget time.Duration) {
	t.Helper()
	if ctx.Err() != nil || ctx.Value(exitContextKey{}) != "original" {
		t.Fatal("cleanup lost original values or inherited cancellation")
	}
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) <= 0 || time.Until(deadline) > budget {
		t.Fatal("cleanup budget missing")
	}
}
func TestEngineeringExitPreservesAllCausesWithoutRetry(t *testing.T) {
	for _, alreadyReaped := range []bool{false, true} {
		for _, stopFails := range []bool{false, true} {
			if alreadyReaped && stopFails {
				continue
			}
			for _, closeFails := range []bool{false, true} {
				t.Run(stringBool(alreadyReaped)+"/"+stringBool(stopFails)+"/"+stringBool(closeFails), func(t *testing.T) {
					ctx, cancel := context.WithCancel(context.WithValue(context.Background(), exitContextKey{}, "original"))
					cancel()
					original := ErrEngineeringInterrupted
					stopFailure := errors.New("test-only stop failure")
					control := &exitController{}
					if closeFails {
						control.closeError = controlclient.ErrCommitOutcomeUnknown
					}
					proof := processscope.Proof{}
					if alreadyReaped {
						proof = exitTestProof()
					}
					calls := 0
					var stopCtx context.Context
					stopping := func(ctx context.Context) (processscope.Proof, error) {
						calls++
						stopCtx = ctx
						assertExitContext(t, ctx, 5*time.Second)
						if stopFails {
							return processscope.Proof{}, stopFailure
						}
						return exitTestProof(), nil
					}
					got, err := settleEngineeringExit(ctx, stopping, proof, control, true, "interrupted", original)
					wantStops := 1
					if alreadyReaped {
						wantStops = 0
					}
					if calls != wantStops || control.closeCalls != 1 || control.invalidCloseContext || !errors.Is(err, original) {
						t.Fatalf("original cause/call count lost: %v", err)
					}
					if errors.Is(err, stopFailure) != stopFails || errors.Is(err, ErrEngineeringStopUnconfirmed) != stopFails {
						t.Fatal("stop uncertainty lost")
					}
					if errors.Is(err, controlclient.ErrCommitOutcomeUnknown) != closeFails || errors.Is(err, ErrEngineeringCloseUnobserved) != closeFails {
						t.Fatal("precise close uncertainty lost")
					}
					if !reflect.DeepEqual(got, control.proof) || got.Quiescent() == stopFails || control.status != "interrupted" {
						t.Fatal("quiescence/status fabricated")
					}
					for _, ended := range []context.Context{stopCtx, control.closeContext} {
						if ended != nil && (ended.Err() != context.Canceled || ended.Value(exitContextKey{}) != "original") {
							t.Fatal("cleanup lifetime not joined/released")
						}
					}
				})
			}
		}
	}
}
func stringBool(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
func TestEngineeringExitNeverClosesUnstartedControlOrInventsSuccess(t *testing.T) {
	calls := 0
	stop := func(context.Context) (processscope.Proof, error) { calls++; return processscope.Proof{}, nil }
	control := &exitController{}
	_, err := settleEngineeringExit(context.Background(), stop, processscope.Proof{}, control, false, "", nil)
	if calls != 1 || control.closeCalls != 0 || !errors.Is(err, ErrEngineeringStopUnconfirmed) {
		t.Fatal("unconfirmed stop silently succeeded")
	}
	_, err = settleEngineeringExit(context.Background(), stop, exitTestProof(), control, true, "", nil)
	if calls != 1 || control.closeCalls != 1 || err != nil || control.status != "unknown" {
		t.Fatal("successful close or unknown status changed")
	}
}
func TestControlReportFailureKeepsProviderCallError(t *testing.T) {
	ctx := context.WithValue(context.Background(), exitContextKey{}, "original")
	control := &exitController{fixtureController: &fixtureController{command: &LiveControl{ID: "original", Kind: "INTERRUPT", ThreadID: "thread", TurnID: "turn"}}, reportError: controlclient.ErrPostOutcomeUnobserved}
	// Uninitialized Adapter rejects before sending. Even this precise no-call
	// failure must not be erased when Core cannot acknowledge NOT_APPLIED.
	err := pumpControls(ctx, ctx, &Adapter{}, "thread", "turn", control)
	if !errors.Is(err, ErrLifecycle) || !errors.Is(err, controlclient.ErrPostOutcomeUnobserved) || control.reportCalls != 1 || control.claimed != 1 {
		t.Fatalf("call/report failure erased or replayed: %v", err)
	}
	if control.reportContext.Value(exitContextKey{}) != "original" || control.reportContext.Err() != context.Canceled {
		t.Fatal("report lifecycle values/cleanup lost")
	}
}

type claimFailureController struct {
	*exitController
	claimError error
	claimCalls int
}

func (c *claimFailureController) Claim(context.Context) (*LiveControl, error) {
	c.claimCalls++
	return nil, c.claimError
}
func TestEngineeringDeliveryFailureJoinsCancelledObservation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	client, remote := pair(t)
	defer client.Close()
	defer remote.Close()
	adapter, err := NewAdapter(client, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	adapter.initialized, adapter.thread, adapter.turn = true, "thread", "turn"
	failed := errors.New("test-only control claim error")
	controller := &claimFailureController{exitController: &exitController{fixtureController: &fixtureController{bound: make(chan struct{})}}, claimError: failed}
	// No terminal notification is sent. The failed delivery must cancel and JOIN
	// the real event-reader goroutine; returning only its own error loses that fact.
	value, err := observeControlledEngineering(ctx, adapter, "thread", "turn", controller)
	if !errors.Is(err, failed) || !errors.Is(err, context.Canceled) || value.Status != "" || controller.claimCalls != 1 || controller.reportCalls != 0 {
		t.Fatalf("delivery/observation cause lost or control replayed: %+v %v", value, err)
	}
}
