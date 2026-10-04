package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jiying2007/engineering-platform/internal/action"
	"sync"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/codexexec"
	"github.com/jiying2007/engineering-platform/internal/run"
	"github.com/jiying2007/engineering-platform/internal/testsupport"
)

const continuationOwner = "urn:engineering-platform:engineer:continuation-test"

func continuationFixture(t *testing.T, s *Store) (codexexec.ContinueRequest, codexexec.Permit) {
	t.Helper()
	ctx := context.Background()
	p, b, i := liveControlFixture(t, s)
	work, e := s.GetWork(p.Assignment.Task.WorkItemID)
	workerOK(t, e)
	version := work.Version
	work.HumanOwner = continuationOwner
	workerOK(t, s.UpdateWork(work.ID, version, work))
	i.Text = ""
	i.Actor = continuationOwner
	_, e = s.QueueCodexControl(ctx, p.Token.RunID, codexexec.ControlInterrupt, i)
	workerOK(t, e)
	_, e = s.ClaimCodexControl(ctx, codexTestWorker, b)
	workerOK(t, e)
	_, e = s.SettleCodexControl(ctx, codexTestWorker, codexexec.ControlSettlement{Binding: b, ID: i.ID, Outcome: codexexec.ControlInterruptACK})
	workerOK(t, e)
	tr, e := s.CloseCodexControl(ctx, codexTestWorker, codexexec.ControlClose{Binding: b, TurnStatus: "interrupted", ProcessScope: testsupport.ProcessScopeFixture()})
	workerOK(t, e)
	td, e := tr.Digest()
	workerOK(t, e)
	cp := codexexec.SourceCheckpoint{Version: 1, Binding: b, TaskDigest: p.Assignment.Intent.TaskDigest, InputDigest: p.Assignment.Intent.InputDigest, BaseCommit: p.Preparation.Facts.BaseCommit, BaselineDigest: p.Preparation.Facts.SourceDigest, TranscriptDigest: td, ArchiveDigest: canonical.BytesDigest([]byte("TEST-ONLY bytes")), ArchiveSize: 1, SnapshotDigest: canonical.BytesDigest([]byte("TEST-ONLY tree"))}
	_, e = s.SaveCodexCheckpoint(ctx, codexTestWorker, cp)
	workerOK(t, e)
	workerOK(t, s.FailCodex(ctx, codexTestWorker, p.Token))
	r, _, e := s.GetExecution(p.Token.RunID)
	workerOK(t, e)
	work, e = s.GetWork(work.ID)
	workerOK(t, e)
	cd, e := cp.Digest()
	workerOK(t, e)
	return codexexec.ContinueRequest{SourceRunID: p.Token.RunID, RunID: "successor", AttemptID: "successor-attempt", CheckpointDigest: cd, ExpectedRunVersion: r.Version, ExpectedWorkVersion: work.Version, ExecutionEpoch: r.CurrentEpoch, RecoveryEpoch: p.Token.RecoveryEpoch, Reason: "Explicit reviewed source continuation"}, p
}
func TestCodexContinuationAtomicAndIdempotent(t *testing.T) {
	s := integrationStore(t)
	ctx := context.Background()
	q, p := continuationFixture(t, s)
	before, e := s.GetCodex(ctx, q.SourceRunID)
	workerOK(t, e)
	beforeBytes, _ := json.Marshal(before.Runtime)
	receipt, e := s.ContinueCodex(ctx, continuationOwner, q)
	workerOK(t, e)
	workerOK(t, receipt.Validate())
	again, e := s.ContinueCodex(ctx, continuationOwner, q)
	workerOK(t, e)
	a, _ := json.Marshal(receipt)
	b, _ := json.Marshal(again)
	if string(a) != string(b) {
		t.Fatal("retry mutated receipt")
	}
	old, _, e := s.GetExecution(q.SourceRunID)
	workerOK(t, e)
	if old.State != run.Aborted || old.CurrentEpoch != q.ExecutionEpoch+1 {
		t.Fatal("old Run not fenced")
	}
	after, e := s.GetCodex(ctx, q.SourceRunID)
	workerOK(t, e)
	afterBytes, _ := json.Marshal(after.Runtime)
	if after.State != codexexec.Stopped || after.Receipt != nil || string(beforeBytes) != string(afterBytes) || *after.SourceCheckpoint != *before.SourceCheckpoint {
		t.Fatal("historical proofs changed or success promoted")
	}
	workerOK(t, s.FailCodex(ctx, codexTestWorker, p.Token))
	if _, e = s.StartCodex(ctx, codexTestWorker, codexexec.Start{RunID: q.SourceRunID, WorkerProfile: p.Token.WorkerProfile, Profile: p.Profile}); e == nil {
		t.Fatal("old model replay")
	}
	if _, e = s.RenewCodex(ctx, codexTestWorker, p.Token); e == nil {
		t.Fatal("old renewal allowed")
	}
	work, e := s.GetWork(p.Assignment.Task.WorkItemID)
	workerOK(t, e)
	if work.ActiveRunID != q.RunID || work.ActiveTaskContractDigest != p.Assignment.Intent.TaskDigest || work.Version != q.ExpectedWorkVersion+1 {
		t.Fatal("wrong Work transition")
	}
	assertCount(t, s, "SELECT count(*) FROM audit_events WHERE event_type='run.source-continuation-authorized'", 1)
	assertCount(t, s, "SELECT count(*) FROM outbox_events WHERE aggregate_id='successor' AND topic='run.started'", 1)
	changed := q
	changed.Reason = "different"
	if _, e = s.ContinueCodex(ctx, continuationOwner, changed); e == nil {
		t.Fatal("changed retry accepted")
	}
	changed = q
	changed.RunID = "another"
	if _, e = s.ContinueCodex(ctx, continuationOwner, changed); e == nil {
		t.Fatal("second successor")
	}
	workerOK(t, s.ApplyCoreMigration(ctx))
	read, e := s.GetCodexContinuation(ctx, q.SourceRunID)
	workerOK(t, e)
	bytes, _ := json.Marshal(read)
	if string(bytes) != string(a) {
		t.Fatal("migration/readback changed decision")
	}
	relayWorkerFixture(t, s)
	next, e := s.ClaimInput(ctx, codexTestWorker, p.Token.WorkerProfile)
	workerOK(t, e)
	if next == nil || next.Intent.RunID != q.RunID || next.Input.Continuation == nil || next.Intent.TaskDigest != p.Assignment.Intent.TaskDigest {
		t.Fatal("new input missing authorized source")
	}
}
func TestCodexContinuationRejectsStaleForeignAndUnreconciled(t *testing.T) {
	cases := []string{"owner", "run-version", "work-version", "epoch", "recovery", "checkpoint", "active", "no-interrupt", "unknown-control", "external"}
	for _, mode := range cases {
		t.Run(mode, func(t *testing.T) {
			s := integrationStore(t)
			ctx := context.Background()
			q, p := continuationFixture(t, s)
			actor := continuationOwner
			switch mode {
			case "owner":
				actor = "urn:engineering-platform:engineer:other"
			case "run-version":
				q.ExpectedRunVersion++
			case "work-version":
				q.ExpectedWorkVersion++
			case "epoch":
				q.ExecutionEpoch++
			case "recovery":
				q.RecoveryEpoch++
			case "checkpoint":
				q.CheckpointDigest = canonical.BytesDigest([]byte("wrong"))
			case "active":
				_, e := s.pool.Exec(ctx, `UPDATE worker_codex_executions SET state='AUTHORIZED' WHERE run_id=$1`, q.SourceRunID)
				workerOK(t, e)
			case "no-interrupt", "unknown-control":
				rt, e := s.GetCodexControlRuntime(ctx, q.SourceRunID)
				workerOK(t, e)
				if mode == "no-interrupt" {
					rt.Transcript.Deliveries = nil
				} else {
					rt.Transcript.Deliveries[0].State = codexexec.ControlUnknown
				}
				digest, e := rt.Transcript.Digest()
				workerOK(t, e)
				raw, _ := json.Marshal(rt.Transcript)
				_, e = s.pool.Exec(ctx, `UPDATE worker_codex_runtime SET transcript_json=$2::jsonb,transcript_digest=$3 WHERE execution_id=$1`, p.Token.ID, string(raw), digest)
				workerOK(t, e)
			case "external":
				_, e := s.pool.Exec(ctx, `INSERT INTO external_operations(operation_id,run_id,execution_epoch,recovery_epoch,action,risk_class,capability,idempotency_key,request_digest,state) VALUES('pending-effect',$1,1,0,'test','CONTROLLED_MUTATION','test','pending-test',$2,'UNKNOWN')`, q.SourceRunID, canonical.BytesDigest([]byte("test")))
				workerOK(t, e)
			}
			if _, e := s.ContinueCodex(ctx, actor, q); e == nil {
				t.Fatal("unsafe continuation accepted", mode)
			}
			assertCount(t, s, "SELECT count(*) FROM codex_continuations", 0)
			assertCount(t, s, "SELECT count(*) FROM runs WHERE run_id='successor'", 0)
		})
	}
}
func TestCodexContinuationConcurrentDecisionOnlyOneSuccessor(t *testing.T) {
	s := integrationStore(t)
	q, _ := continuationFixture(t, s)
	ctx := context.Background()
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, id := range []string{"first", "second"} {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			copy := q
			copy.RunID = id
			_, e := s.ContinueCodex(ctx, continuationOwner, copy)
			results <- e
		}(id)
	}
	wg.Wait()
	close(results)
	success := 0
	for e := range results {
		if e == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatal("concurrent decision split", success)
	}
	assertCount(t, s, "SELECT count(*) FROM codex_continuations", 1)
	assertCount(t, s, "SELECT count(*) FROM audit_events WHERE event_type='run.source-continuation-authorized'", 1)
}

func TestCodexContinuationAndLateActionCannotBothCommit(t *testing.T) {
	for i := 0; i < 5; i++ {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			s := integrationStore(t)
			q, _ := continuationFixture(t, s)
			op := action.NewWithRequestDigest(action.Request{ID: "racing-action", RunID: q.SourceRunID, ExecutionEpoch: q.ExecutionEpoch, RecoveryEpoch: q.RecoveryEpoch, Action: "test", RiskClass: action.ControlledMutation, Capability: "test", IdempotencyKey: "racing-action"}, canonical.BytesDigest([]byte("test")), time.Now().UTC())
			results := make(chan error, 2)
			start := make(chan struct{})
			go func() { <-start; results <- s.Create(*op) }()
			go func() { <-start; _, err := s.ContinueCodex(context.Background(), continuationOwner, q); results <- err }()
			close(start)
			one, two := <-results, <-results
			if (one == nil) == (two == nil) {
				t.Fatalf("action/continuation fence failed: %v / %v", one, two)
			}
		})
	}
}
func TestCodexContinuationRejectsActionAuthorizedBeforeFence(t *testing.T) {
	s := integrationStore(t)
	q, _ := continuationFixture(t, s)
	op := action.NewWithRequestDigest(action.Request{ID: "late-action", RunID: q.SourceRunID, ExecutionEpoch: q.ExecutionEpoch, RecoveryEpoch: q.RecoveryEpoch, Action: "test", RiskClass: action.ControlledMutation, Capability: "test", IdempotencyKey: "late-action"}, canonical.BytesDigest([]byte("test")), time.Now().UTC())
	_, err := s.ContinueCodex(context.Background(), continuationOwner, q)
	workerOK(t, err)
	if s.Create(*op) == nil {
		t.Fatal("previous authorization crossed continuation fence")
	}
	op.ExecutionEpoch++
	if s.Create(*op) == nil {
		t.Fatal("new epoch reopened retired source actions")
	}
	assertCount(t, s, "SELECT count(*) FROM external_operations", 0)
}
