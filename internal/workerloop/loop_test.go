package workerloop

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCancellationDoesNotDetachStep(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	err := Run(ctx, 10*time.Millisecond, func(context.Context) error { calls++; cancel(); return nil })
	if !errors.Is(err, context.Canceled) || calls != 1 {
		t.Fatalf("unexpected loop result: %v calls=%d", err, calls)
	}
}
func TestStepFailureStopsAndBadConfigurationFails(t *testing.T) {
	expected := errors.New("storage unavailable")
	if err := Run(context.Background(), time.Second, func(context.Context) error { return expected }); !errors.Is(err, expected) {
		t.Fatal(err)
	}
	if err := Run(context.Background(), 0, nil); err == nil {
		t.Fatal("invalid configuration accepted")
	}
}
