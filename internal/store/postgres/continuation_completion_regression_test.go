package postgres

import (
	"fmt"
	"testing"
)

// The immediate ACK/terminal fixture previously exposed an internal RPC
// cancellation race only after merge. Repeat the actual PostgreSQL/mTLS/two-
// Worker/kernel/Git path without changing continuation gates or fixture timing.
// No real model/account is used; a missing DB remains an explicit local skip.
func TestCodexContinuationImmediateCompletionRegression(t *testing.T) {
	for i := 0; i < 5; i++ {
		t.Run(fmt.Sprintf("immediate-%d", i), TestCodexContinuationMTLSTwoActualWorkerExecutions)
	}
}
