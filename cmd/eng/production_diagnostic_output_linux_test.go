//go:build linux

package main

import (
	"context"
	"syscall"
	"testing"
	"time"
)

func TestProductionDiagnosticBlockedPipeExitsWithinDeadline(t *testing.T) {
	for _, mode := range []string{"deadline", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			_, output := monitorBlockingPipe(t, 4096)
			before := monitorPipeFlags(t, output)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "deadline" {
				var deadlineCancel context.CancelFunc
				ctx, deadlineCancel = context.WithTimeout(ctx, 120*time.Millisecond)
				defer deadlineCancel()
			} else {
				timer := time.AfterFunc(120*time.Millisecond, cancel)
				defer timer.Stop()
			}
			started := time.Now()
			err := writeProductionDiagnostic(ctx, output, map[string]string{
				"state": "LOCAL_COMPONENTS_OBSERVED_PROVIDER_CAPACITY_PENDING",
			})
			if err == nil || time.Since(started) > 2*time.Second {
				t.Fatalf("blocked production JSON output was incorrectly successful/long: elapsed=%v err=%v", time.Since(started), err)
			}
			after := monitorPipeFlags(t, output)
			if before != after || after&syscall.O_NONBLOCK != 0 {
				t.Fatal("stdout shared file flags were changed")
			}
		})
	}
}
