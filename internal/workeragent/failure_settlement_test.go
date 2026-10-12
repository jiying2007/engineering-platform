package workeragent

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/controlclient"
)

type settlementKey struct{}
type settlementTransport struct {
	calls       int
	raw         string
	failure     error
	parentValue any
	seen        context.Context
}

func (c *settlementTransport) Subject() string { return "urn:engineering-platform:worker:test" }
func (c *settlementTransport) Call(ctx context.Context, method, path string, in, out any) error {
	c.calls++
	c.seen = ctx
	if ctx.Err() != nil {
		return errors.New("settlement inherited original cancellation")
	}
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) > 6*time.Second || time.Until(deadline) <= 0 {
		return errors.New("unbounded settlement context")
	}
	if ctx.Value(settlementKey{}) != c.parentValue {
		return errors.New("request identity value was discarded")
	}
	if method != http.MethodPost {
		return errors.New("unexpected method")
	}
	if c.failure != nil {
		return c.failure
	}
	return json.Unmarshal([]byte(c.raw), out)
}

func TestFailureSettlementKeepsOriginalUncertaintyAndNeverRetries(t *testing.T) {
	for _, endpoint := range []string{"/api/v1/worker/offline/fail", "/api/v1/worker/codex/fail"} {
		for _, tc := range []struct {
			name, raw string
			failure   error
			valid     bool
		}{
			{name: "ack", raw: `{"recorded":true}`, valid: true},
			{name: "missing", raw: `{}`}, {name: "false", raw: `{"recorded":false}`}, {name: "null", raw: `null`},
			{name: "lost-commit-ack", failure: controlclient.ErrCommitOutcomeUnknown},
			{name: "transport-loss", failure: controlclient.ErrPostOutcomeUnobserved},
		} {
			t.Run(endpoint+"/"+tc.name, func(t *testing.T) {
				parent, cancel := context.WithCancel(context.WithValue(context.Background(), settlementKey{}, "request-value"))
				cancel()
				transport := &settlementTransport{raw: tc.raw, failure: tc.failure, parentValue: "request-value"}
				original := errors.New("original execution failure")
				err := errors.Join(original, settleExecutionFailure(parent, transport, endpoint, struct{}{}))
				if !errors.Is(err, original) || transport.calls != 1 {
					t.Fatalf("original failure/retry boundary changed: %v calls=%d", err, transport.calls)
				}
				if errors.Is(err, ErrFailureReportUnobserved) == tc.valid {
					t.Fatalf("incorrect settlement result: %v", err)
				}
				if tc.failure != nil && !errors.Is(err, tc.failure) {
					t.Fatal("lost precise uncertainty classification")
				}
				if transport.seen == nil || transport.seen.Err() != context.Canceled {
					t.Fatal("settlement context/timer not released after join")
				}
			})
		}
	}
}
