package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/codexexec"
	"github.com/jiying2007/engineering-platform/internal/core"
	"github.com/jiying2007/engineering-platform/internal/run"
	"github.com/jiying2007/engineering-platform/internal/session"
	corestore "github.com/jiying2007/engineering-platform/internal/store"
	"github.com/jiying2007/engineering-platform/internal/workerqueue"
)

var _ codexexec.ContinuationRepository = (*Store)(nil)

func readContinuation(row pgx.Row) (codexexec.ContinuationReceipt, error) {
	var c codexexec.ContinuationReceipt
	var raw []byte
	var rd, actor, qd, source, successor string
	if err := row.Scan(&raw, &rd, &actor, &qd, &source, &successor); err != nil {
		return c, mapReadError(err)
	}
	if json.Unmarshal(raw, &c) != nil || c.Validate() != nil {
		return c, workerqueue.ErrIdentity
	}
	actual, err := canonical.Digest(c)
	request, _ := canonical.Digest(c.Request)
	if err != nil || actual != rd || request != qd || c.Actor != actor || c.Request.SourceRunID != source || c.Run.ID != successor {
		return c, workerqueue.ErrIdentity
	}
	return c, nil
}

const continuationColumns = `receipt_json,receipt_digest,actor,request_digest,source_run_id,successor_run_id`

func (s *Store) GetCodexContinuation(ctx context.Context, source string) (codexexec.ContinuationReceipt, error) {
	return readContinuation(s.pool.QueryRow(ctx, `SELECT `+continuationColumns+` FROM codex_continuations WHERE source_run_id=$1`, source))
}

// ContinueCodex performs one local transaction, not a model call. It fences the
// old Run, records its proven unsuccessful stop, and starts one new Run intent.
// Same-ID observation can return its immutable receipt even after later changes.
func (s *Store) ContinueCodex(ctx context.Context, actor string, q codexexec.ContinueRequest) (codexexec.ContinuationReceipt, error) {
	var empty codexexec.ContinuationReceipt
	if q.Validate() != nil || !strings.HasPrefix(actor, "urn:engineering-platform:") {
		return empty, workerqueue.ErrIdentity
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.workerTransaction(ctx)
	if err != nil {
		return empty, err
	}
	defer rollbackOutbox(tx)
	epoch, mode, err := lockWorkerPlatform(ctx, tx)
	if err != nil {
		return empty, err
	}
	// Match normal execution lock order: platform -> inbox -> Run -> Session ->
	// execution -> Runtime -> Work. Concurrent decisions serialize on the inbox.
	inbox, err := scanInbox(tx.QueryRow(ctx, `SELECT `+inboxColumns+` FROM worker_inbox WHERE run_id=$1 FOR UPDATE`, q.SourceRunID))
	if err != nil {
		return empty, mapReadError(err)
	}
	old, err := readContinuation(tx.QueryRow(ctx, `SELECT `+continuationColumns+` FROM codex_continuations WHERE source_run_id=$1`, q.SourceRunID))
	if err == nil {
		if old.Actor != actor || old.Request != q {
			return empty, corestore.ErrConflict
		}
		return old, nil
	}
	if err != corestore.ErrNotFound {
		return empty, err
	}
	if mode != "NORMAL" || epoch != q.RecoveryEpoch {
		return empty, workerqueue.ErrRecovery
	}
	var state, owner, task, input, attempt string
	var version, currentEpoch uint64
	err = tx.QueryRow(ctx, `SELECT state,control_owner,version,current_epoch,task_contract_digest,run_input_manifest_digest,current_attempt_id FROM runs WHERE run_id=$1 FOR UPDATE`, q.SourceRunID).Scan(&state, &owner, &version, &currentEpoch, &task, &input, &attempt)
	if err != nil {
		return empty, mapReadError(err)
	}
	if state != "RUNNING" || owner != "RUNTIME" || version != q.ExpectedRunVersion || currentEpoch != q.ExecutionEpoch || currentEpoch != inbox.intent.ExecutionEpoch || task != inbox.intent.TaskDigest || input != inbox.intent.InputDigest {
		return empty, corestore.ErrConflict
	}
	var se uint64
	var so string
	var paused bool
	if err = tx.QueryRow(ctx, `SELECT execution_epoch,control_owner,paused FROM sessions WHERE run_id=$1 FOR UPDATE`, q.SourceRunID).Scan(&se, &so, &paused); err != nil {
		return empty, err
	}
	if se != currentEpoch || so != "RUNTIME" || paused {
		return empty, corestore.ErrConflict
	}
	row, err := scanCodex(tx.QueryRow(ctx, `SELECT `+codexColumns+` FROM worker_codex_executions WHERE run_id=$1 FOR UPDATE`, q.SourceRunID))
	if err != nil {
		return empty, err
	}
	if row.state != codexexec.Unknown || row.receipt != nil || row.worker != inbox.owner || row.token.RecoveryEpoch != epoch {
		return empty, corestore.ErrConflict
	}
	runtime, err := controlRuntime(tx.QueryRow(ctx, `SELECT `+runtimeColumns+` FROM worker_codex_runtime WHERE execution_id=$1 FOR UPDATE`, row.token.ID))
	if err != nil {
		return empty, err
	}
	if runtime.State != "SEALED" || runtime.Transcript == nil || !runtime.Transcript.Continuable() || runtime.Binding.Token != row.token || runtime.Binding.ExecutionEpoch != currentEpoch {
		return empty, fmt.Errorf("observed explicit interruption and quiescent sealed controls required")
	}
	checkpoint, err := checkpointRow(tx.QueryRow(ctx, `SELECT checkpoint_json,checkpoint_digest FROM worker_codex_source_checkpoints WHERE execution_id=$1`, row.token.ID))
	if err != nil {
		return empty, err
	}
	if checkpoint == nil {
		return empty, corestore.ErrNotFound
	}
	cd, err := checkpoint.Digest()
	if err != nil || cd != q.CheckpointDigest || checkpoint.Binding != runtime.Binding || checkpoint.TranscriptDigest != runtime.Digest || checkpoint.TaskDigest != task || checkpoint.InputDigest != input {
		return empty, workerqueue.ErrIdentity
	}
	assignment, err := loadWorkerAssignment(ctx, tx, inbox)
	if err != nil {
		return empty, err
	}
	if checkpoint.BaseCommit != strings.ToLower(assignment.Task.BaseCommit) || codexexec.CheckAssignment(assignment, row.profile) != nil {
		return empty, workerqueue.ErrIdentity
	}
	var workVersion uint64
	var workState, workOwner, workRun, workTask string
	if err = tx.QueryRow(ctx, `SELECT version,state,human_owner,COALESCE(active_run_id,''),COALESCE(active_task_contract_digest,'') FROM work_items WHERE work_item_id=$1 FOR UPDATE`, assignment.Task.WorkItemID).Scan(&workVersion, &workState, &workOwner, &workRun, &workTask); err != nil {
		return empty, mapReadError(err)
	}
	if workVersion != q.ExpectedWorkVersion || workState != string(core.WorkExecuting) || workOwner != actor || workRun != q.SourceRunID || workTask != task {
		return empty, corestore.ErrConflict
	}
	// Do not hide unresolved external effects or an already-delivered subject.
	// The locked source Run prevents another action from being authorized now.
	var unresolved bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM external_operations WHERE run_id=$1 AND state<>'CONFIRMED') OR EXISTS(SELECT 1 FROM delivery_receipts WHERE run_id=$1) OR EXISTS(SELECT 1 FROM worker_offline_executions WHERE run_id=$1)`, q.SourceRunID).Scan(&unresolved); err != nil {
		return empty, err
	}
	if unresolved {
		return empty, fmt.Errorf("source Run has unresolved operations, delivery, or another execution lane")
	}
	ref, err := checkpoint.ContinuationRef()
	if err != nil {
		return empty, err
	}
	manifest := assignment.Input
	manifest.RunID = q.RunID
	manifest.Continuation = &ref
	digest, err := manifest.Digest()
	if err != nil {
		return empty, err
	}
	value := run.New(q.RunID, task, digest)
	var now time.Time
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return empty, err
	}
	nextAttempt, err := value.StartAttempt(q.AttemptID, now)
	if err != nil {
		return empty, err
	}
	nextSession := session.New(q.RunID, nextAttempt.Epoch)
	receipt := codexexec.ContinuationReceipt{Version: 1, Actor: actor, Request: q, Run: *value, Attempt: nextAttempt, Session: *nextSession, Input: manifest, SourceDisposition: codexexec.Stopped, CreatedAt: now}
	if err = receipt.Validate(); err != nil {
		return empty, err
	}
	if err = insertExecution(ctx, tx, *value, nextAttempt, *nextSession, manifest); err != nil {
		return empty, err
	}
	if _, err = tx.Exec(ctx, `UPDATE runs SET state='ABORTED',version=version+1,current_epoch=current_epoch+1,updated_at=clock_timestamp() WHERE run_id=$1`, q.SourceRunID); err != nil {
		return empty, err
	}
	if _, err = tx.Exec(ctx, `UPDATE sessions SET execution_epoch=execution_epoch+1,paused=true,updated_at=clock_timestamp() WHERE run_id=$1`, q.SourceRunID); err != nil {
		return empty, err
	}
	if _, err = tx.Exec(ctx, `UPDATE run_attempts SET ended_at=$3,disposition='STOPPED_NO_DELIVERY' WHERE run_id=$1 AND attempt_id=$2`, q.SourceRunID, attempt, now); err != nil {
		return empty, err
	}
	if _, err = tx.Exec(ctx, `UPDATE worker_codex_executions SET state='STOPPED_NO_DELIVERY' WHERE execution_id=$1`, row.token.ID); err != nil {
		return empty, err
	}
	if _, err = tx.Exec(ctx, `UPDATE work_items SET active_run_id=$2,version=version+1,updated_at=clock_timestamp() WHERE work_item_id=$1`, assignment.Task.WorkItemID, q.RunID); err != nil {
		return empty, err
	}
	raw, _ := json.Marshal(receipt)
	rd, _ := canonical.Digest(receipt)
	qd, _ := canonical.Digest(q)
	if _, err = tx.Exec(ctx, `INSERT INTO codex_continuations(source_run_id,successor_run_id,actor,request_digest,receipt_json,receipt_digest) VALUES($1,$2,$3,$4,$5::jsonb,$6)`, q.SourceRunID, q.RunID, actor, qd, string(raw), rd); err != nil {
		return empty, mapWriteError(err)
	}
	if err = workerAudit(ctx, tx, "run.source-continuation-authorized", q.SourceRunID, receipt); err != nil {
		return empty, err
	}
	if _, err = insertOutbox(ctx, tx, OutboxMessage{Key: "run:" + q.RunID + ":started", Topic: "run.started", AggregateType: "Run", AggregateID: q.RunID, Payload: workerqueue.Intent{RunID: q.RunID, TaskDigest: task, InputDigest: digest, ExecutionEpoch: 1}}); err != nil {
		return empty, err
	}
	if err = tx.Commit(ctx); err != nil {
		return empty, err
	}
	return receipt, nil
}
