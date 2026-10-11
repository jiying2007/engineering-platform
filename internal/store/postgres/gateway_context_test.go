package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/action"
	"github.com/jiying2007/engineering-platform/internal/gateway"
)

func TestGatewayRequestContextCancelsPostgresAuthorityLocks(t *testing.T) {
	s := newIsolatedIntegrationStore(t)
	runID := setupPostgresActionRun(t, s, "gateway-context")
	ctx, stop := context.WithTimeout(context.Background(), 15*time.Second)
	defer stop()
	auth := gateway.NewAuthority(s)
	tests := []struct {
		name  string
		table string
		call  func(context.Context) error
	}{
		{
			name:  "authorize",
			table: "runs",
			call: func(c context.Context) error {
				return auth.Authorize(c, action.Request{RunID: runID, Action: "ci.dispatch", Capability: "ci.dispatch", RiskClass: action.ControlledMutation})
			},
		},
		{
			name:  "run-epoch",
			table: "runs",
			call: func(c context.Context) error { return auth.CheckRunEpoch(c, runID, 1) },
		},
		{
			name:  "recovery-epoch",
			table: "platform_state",
			call: func(c context.Context) error { return auth.CheckRecoveryEpoch(c, 0, action.ControlledMutation) },
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			blocker, err := s.pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer rollbackOutbox(blocker)
			if _, err := blocker.Exec(ctx, "LOCK TABLE "+tc.table+" IN ACCESS EXCLUSIVE MODE"); err != nil {
				t.Fatal(err)
			}
			caller, cancel := context.WithCancel(ctx)
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- tc.call(caller) }()
			select {
			case err := <-done:
				t.Fatalf("gateway passed locked Core authority: %v", err)
			case <-time.After(80 * time.Millisecond):
			}
			cancel()
			select {
			case err := <-done:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("cancelled gateway did not propagate caller Context: %v", err)
				}
			case <-ctx.Done():
				t.Fatal("gateway did not cancel before authority lock release")
			}
			if err := blocker.Rollback(ctx); err != nil {
				t.Fatal(err)
			}
		})
	}
}
