package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/codexexec"
	"github.com/jiying2007/engineering-platform/internal/offline"
	"github.com/jiying2007/engineering-platform/internal/recovery"
	corestore "github.com/jiying2007/engineering-platform/internal/store"
)

func reconcileRequest(kind, run, execution string, epoch uint64) recovery.ExecutionAbandonRequest {
	return recovery.ExecutionAbandonRequest{
		Version: 1, ExecutionKind: kind, RunID: run, ExecutionID: execution,
		RecoveryEpoch: epoch, ObservationDigest: canonical.BytesDigest([]byte("independent TEST observation:" + kind)),
		Disposition: recovery.AbandonNoReplay,
	}
}

func TestReconcileExpiredOfflineExecutionAbandonsWithoutReplay(t *testing.T) {
	s := integrationStore(t)
	ctx := context.Background()
	start := offlineFixture(t, s)
	permit, err := s.StartOffline(ctx, "offline-worker", start)
	workerOK(t, err)
	workerSQL(t, s, "UPDATE worker_offline_executions SET lease_until=clock_timestamp()-interval '1 second'")
	state, err := s.BeginRecovery(permit.Token.RecoveryEpoch)
	workerOK(t, err)
	request := reconcileRequest(recovery.ExecutionOffline, start.RunID, permit.Token.ID, state.Epoch)
	receipt, err := s.ReconcileExecutionAbandoned(ctx, "reconciler", request)
	workerOK(t, err)
	workerOK(t, receipt.Validate())
	if receipt.PreviousState != offline.Authorized || receipt.ReplayAuthorized || receipt.ExecutionAuthorized {
		t.Fatal("offline abandonment promoted authority", receipt)
	}
	stored, err := s.GetOffline(ctx, start.RunID)
	workerOK(t, err)
	if stored.State != recovery.ExecutionAbandoned || stored.Receipt != nil {
		t.Fatal("offline abandonment became a result", stored)
	}
	again, err := s.ReconcileExecutionAbandoned(ctx, "reconciler", request)
	workerOK(t, err)
	a, _ := json.Marshal(receipt)
	b, _ := json.Marshal(again)
	if string(a) != string(b) {
		t.Fatal("exact reconciliation retry changed receipt")
	}
	changed := request
	changed.ObservationDigest = canonical.BytesDigest([]byte("different observation"))
	if _, err = s.ReconcileExecutionAbandoned(ctx, "reconciler", changed); !errors.Is(err, corestore.ErrConflict) {
		t.Fatalf("changed observation rewrote reconciliation: %v", err)
	}
	proof, err := s.CreateRecoveryProof(ctx, state.Epoch, "reconciler")
	workerOK(t, err)
	if !proof.Facts.Clear() {
		t.Fatal("abandoned execution still blocks proof", proof.Facts)
	}
	_, err = s.CompleteRecovery(state.Epoch, true)
	workerOK(t, err)
	if _, err = s.StartOffline(ctx, "offline-worker", start); err == nil {
		t.Fatal("abandoned execution was replayed")
	}
	assertCount(t, s, "SELECT count(*) FROM audit_events WHERE event_type='recovery.execution.abandoned'", 1)
}

func TestReconcileUnknownCodexWaitsForLeaseAndClearsRecovery(t *testing.T) {
	s := integrationStore(t)
	ctx := context.Background()
	start := codexFixture(t, s)
	permit, err := s.StartCodex(ctx, codexTestWorker, start)
	workerOK(t, err)
	workerOK(t, s.FailCodex(ctx, codexTestWorker, permit.Token))
	state, err := s.BeginRecovery(permit.Token.RecoveryEpoch)
	workerOK(t, err)
	request := reconcileRequest(recovery.ExecutionCodex, start.RunID, permit.Token.ID, state.Epoch)
	if _, err = s.ReconcileExecutionAbandoned(ctx, "reconciler", request); !errors.Is(err, corestore.ErrConflict) {
		t.Fatalf("live UNKNOWN Codex lease reconciled early: %v", err)
	}
	workerSQL(t, s, "UPDATE worker_codex_executions SET lease_until=clock_timestamp()-interval '1 second'")
	receipt, err := s.ReconcileExecutionAbandoned(ctx, "reconciler", request)
	workerOK(t, err)
	if receipt.PreviousState != codexexec.Unknown {
		t.Fatal(receipt)
	}
	status, err := s.GetCodex(ctx, start.RunID)
	workerOK(t, err)
	if status.State != recovery.ExecutionAbandoned || status.Receipt != nil {
		t.Fatal("Codex abandonment became successful execution", status)
	}
	proof, err := s.CreateRecoveryProof(ctx, state.Epoch, "reconciler")
	workerOK(t, err)
	if !proof.Facts.Clear() {
		t.Fatal(proof.Facts)
	}
}

func TestExecutionReconciliationRejectsNormalModeFinishedAndWrongIdentity(t *testing.T) {
	s := integrationStore(t)
	ctx := context.Background()
	start := offlineFixture(t, s)
	permit, err := s.StartOffline(ctx, "offline-worker", start)
	workerOK(t, err)
	request := reconcileRequest(recovery.ExecutionOffline, start.RunID, permit.Token.ID, 1)
	if _, err = s.ReconcileExecutionAbandoned(ctx, "reconciler", request); !errors.Is(err, recovery.ErrStaleEpoch) && !errors.Is(err, recovery.ErrReconciliationRequired) {
		t.Fatalf("normal-mode reconciliation accepted: %v", err)
	}
	workerSQL(t, s, "UPDATE worker_offline_executions SET lease_until=clock_timestamp()-interval '1 second'")
	state, err := s.BeginRecovery(permit.Token.RecoveryEpoch)
	workerOK(t, err)
	request.RecoveryEpoch = state.Epoch
	request.ExecutionID = strings.Repeat("f", 64)
	if _, err = s.ReconcileExecutionAbandoned(ctx, "reconciler", request); !errors.Is(err, corestore.ErrNotFound) {
		t.Fatalf("wrong execution identity accepted: %v", err)
	}
}
