package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/codexexec"
	corestore "github.com/jiying2007/engineering-platform/internal/store"
	"github.com/jiying2007/engineering-platform/internal/workerqueue"
)

var _ codexexec.ControlRepository = (*Store)(nil)

func controlRuntime(row pgx.Row) (*codexexec.ControlRuntime, error) {
	var r codexexec.ControlRuntime
	var binding, transcript []byte
	var digest *string
	if err := row.Scan(&binding, &r.State, &transcript, &digest); err != nil {
		return nil, mapReadError(err)
	}
	if json.Unmarshal(binding, &r.Binding) != nil || r.Binding.Validate() != nil {
		return nil, workerqueue.ErrIdentity
	}
	if r.State == "SEALED" {
		r.Transcript = &codexexec.ControlTranscript{}
		if json.Unmarshal(transcript, r.Transcript) != nil || digest == nil {
			return nil, workerqueue.ErrIdentity
		}
		actual, err := r.Transcript.Digest()
		if err != nil || actual != *digest || r.Transcript.Close.Binding != r.Binding {
			return nil, workerqueue.ErrIdentity
		}
		r.Digest = actual
	} else if r.State != "ACTIVE" || len(transcript) > 0 || digest != nil {
		return nil, workerqueue.ErrIdentity
	}
	return &r, nil
}

const runtimeColumns = `binding_json,state,transcript_json,transcript_digest`

func (s *Store) GetCodexControlRuntime(ctx context.Context, runID string) (*codexexec.ControlRuntime, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	r, err := controlRuntime(s.pool.QueryRow(ctx, `SELECT r.binding_json,r.state,r.transcript_json,r.transcript_digest FROM worker_codex_runtime r JOIN worker_codex_executions e USING(execution_id) WHERE e.run_id=$1`, runID))
	if errors.Is(err, corestore.ErrNotFound) {
		return nil, nil
	}
	return r, err
}
func lockControlRuntime(ctx context.Context, tx pgx.Tx, b codexexec.ControlBinding) (*codexexec.ControlRuntime, error) {
	r, err := controlRuntime(tx.QueryRow(ctx, `SELECT `+runtimeColumns+` FROM worker_codex_runtime WHERE execution_id=$1 FOR UPDATE`, b.Token.ID))
	if err != nil {
		return nil, err
	}
	if r.Binding != b {
		return nil, workerqueue.ErrIdentity
	}
	return r, nil
}
func (s *Store) BindCodexControl(ctx context.Context, subject string, b codexexec.ControlBinding) error {
	if b.Validate() != nil {
		return workerqueue.ErrIdentity
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.workerTransaction(ctx)
	if err != nil {
		return err
	}
	defer rollbackOutbox(tx)
	_, inbox, err := s.lockCodex(ctx, tx, subject, b.Token, true)
	if err != nil {
		return err
	}
	if b.ExecutionEpoch != inbox.intent.ExecutionEpoch {
		return workerqueue.ErrIdentity
	}
	old, err := controlRuntime(tx.QueryRow(ctx, `SELECT `+runtimeColumns+` FROM worker_codex_runtime WHERE execution_id=$1 FOR UPDATE`, b.Token.ID))
	if err == nil {
		if old.Binding != b || old.State != "ACTIVE" {
			return corestore.ErrConflict
		}
		return nil
	}
	if !errors.Is(err, corestore.ErrNotFound) {
		return err
	}
	raw, _ := json.Marshal(b)
	if _, err = tx.Exec(ctx, `INSERT INTO worker_codex_runtime(execution_id,binding_json,state) VALUES($1,$2::jsonb,'ACTIVE')`, b.Token.ID, string(raw)); err != nil {
		return mapWriteError(err)
	}
	if err = workerAudit(ctx, tx, "worker.codex.control-bound", b.Token.RunID, b); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

const controlColumns = `steering_command_id,run_id,execution_epoch,sequence,actor,content_digest,created_at,control_payload,delivery_state,dispatched_at,resolved_at`

func scanControl(row pgx.Row) (codexexec.ControlDelivery, error) {
	var d codexexec.ControlDelivery
	var payload []byte
	c := &d.Command
	if err := row.Scan(&c.ID, &c.RunID, &c.ExecutionEpoch, &c.Sequence, &c.Actor, &c.ContentDigest, &c.CreatedAt, &payload, &d.State, &d.DispatchedAt, &d.ResolvedAt); err != nil {
		return d, mapReadError(err)
	}
	if json.Unmarshal(payload, &d.Payload) != nil || d.Validate() != nil {
		return d, workerqueue.ErrIdentity
	}
	return d, nil
}
func (s *Store) GetCodexControl(ctx context.Context, id string) (codexexec.ControlDelivery, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return scanControl(s.pool.QueryRow(ctx, `SELECT `+controlColumns+` FROM steering_commands WHERE steering_command_id=$1 AND execution_id IS NOT NULL`, id))
}
func matchesControl(d codexexec.ControlDelivery, runID, kind string, i codexexec.ControlInput) bool {
	return d.Command.RunID == runID && d.Command.ID == i.ID && d.Command.ExecutionEpoch == i.ExecutionEpoch && d.Command.Sequence == i.Sequence && d.Command.Actor == i.Actor && d.Payload.Kind == kind && d.Payload.Text == i.Text && d.Payload.Binding.ThreadID == i.ThreadID && d.Payload.Binding.TurnID == i.TurnID
}
func (s *Store) QueueCodexControl(ctx context.Context, runID, kind string, i codexexec.ControlInput) (codexexec.ControlDelivery, error) {
	var empty codexexec.ControlDelivery
	if i.Validate(kind) != nil {
		return empty, workerqueue.ErrIdentity
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.workerTransaction(ctx)
	if err != nil {
		return empty, err
	}
	defer rollbackOutbox(tx)
	// Observation-only idempotency permits an exact retry after the turn ended.
	old, err := scanControl(tx.QueryRow(ctx, `SELECT `+controlColumns+` FROM steering_commands WHERE steering_command_id=$1 AND execution_id IS NOT NULL`, i.ID))
	if err == nil {
		if !matchesControl(old, runID, kind, i) {
			return empty, corestore.ErrConflict
		}
		return old, nil
	}
	if !errors.Is(err, corestore.ErrNotFound) {
		return empty, err
	}
	row, err := scanCodex(tx.QueryRow(ctx, `SELECT `+codexColumns+` FROM worker_codex_executions WHERE run_id=$1`, runID))
	if err != nil {
		return empty, err
	}
	_, inbox, err := s.lockCodex(ctx, tx, row.worker, row.token, true)
	if err != nil {
		return empty, err
	}
	b := codexexec.ControlBinding{Token: row.token, ExecutionEpoch: i.ExecutionEpoch, ThreadID: i.ThreadID, TurnID: i.TurnID}
	runtime, err := lockControlRuntime(ctx, tx, b)
	if err != nil {
		return empty, err
	}
	if runtime.State != "ACTIVE" || inbox.intent.ExecutionEpoch != i.ExecutionEpoch {
		return empty, corestore.ErrConflict
	}
	var count int
	var stopping bool
	if err = tx.QueryRow(ctx, `SELECT count(*),COALESCE(bool_or(control_payload->>'kind'='INTERRUPT'),false) FROM steering_commands WHERE execution_id=$1`, b.Token.ID).Scan(&count, &stopping); err != nil {
		return empty, err
	}
	if count >= codexexec.MaxControls || (kind == codexexec.ControlSteer && count >= codexexec.MaxControls-1) || stopping {
		return empty, corestore.ErrConflict
	}
	d := codexexec.ControlDelivery{Payload: codexexec.ControlPayload{Binding: b, Kind: kind, Text: i.Text}, State: codexexec.ControlQueued}
	d.Command.ID = i.ID
	d.Command.RunID = runID
	d.Command.ExecutionEpoch = i.ExecutionEpoch
	d.Command.Sequence = i.Sequence
	d.Command.Actor = i.Actor
	d.Command.ContentDigest, err = canonical.Digest(d.Payload)
	if err != nil {
		return empty, err
	}
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&d.Command.CreatedAt); err != nil {
		return empty, err
	}
	payload, _ := json.Marshal(d.Payload)
	_, err = tx.Exec(ctx, `INSERT INTO steering_commands(steering_command_id,run_id,execution_epoch,sequence,actor,content_digest,created_at,delivery_state,execution_id,control_payload) VALUES($1,$2,$3,$4,$5,$6,$7,'QUEUED',$8,$9::jsonb)`, i.ID, runID, i.ExecutionEpoch, i.Sequence, i.Actor, d.Command.ContentDigest, d.Command.CreatedAt, b.Token.ID, string(payload))
	if err != nil {
		return empty, mapWriteError(err)
	}
	tag, err := tx.Exec(ctx, `UPDATE sessions SET last_steering_sequence=$1,updated_at=now() WHERE run_id=$2 AND execution_epoch=$3 AND control_owner='RUNTIME' AND paused=false AND last_steering_sequence<$1`, i.Sequence, runID, i.ExecutionEpoch)
	if err != nil {
		return empty, err
	}
	if tag.RowsAffected() != 1 {
		return empty, corestore.ErrConflict
	}
	if _, err = tx.Exec(ctx, `UPDATE runs SET version=version+1,updated_at=now() WHERE run_id=$1`, runID); err != nil {
		return empty, err
	}
	if err = workerAudit(ctx, tx, "steering.queued", runID, d); err != nil {
		return empty, err
	}
	if err = tx.Commit(ctx); err != nil {
		return empty, err
	}
	return d, nil
}
func (s *Store) ClaimCodexControl(ctx context.Context, subject string, b codexexec.ControlBinding) (*codexexec.ControlDelivery, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.workerTransaction(ctx)
	if err != nil {
		return nil, err
	}
	defer rollbackOutbox(tx)
	if _, _, err = s.lockCodex(ctx, tx, subject, b.Token, true); err != nil {
		return nil, err
	}
	runtime, err := lockControlRuntime(ctx, tx, b)
	if err != nil {
		return nil, err
	}
	if runtime.State != "ACTIVE" {
		return nil, corestore.ErrConflict
	}
	var pending bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM steering_commands WHERE execution_id=$1 AND delivery_state IN ('DISPATCHING','INTERRUPT_ACKNOWLEDGED','UNKNOWN'))`, b.Token.ID).Scan(&pending); err != nil {
		return nil, err
	}
	if pending {
		return nil, corestore.ErrConflict
	}
	d, err := scanControl(tx.QueryRow(ctx, `SELECT `+controlColumns+` FROM steering_commands WHERE execution_id=$1 AND delivery_state='QUEUED' ORDER BY sequence LIMIT 1 FOR UPDATE`, b.Token.ID))
	if errors.Is(err, corestore.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	d, err = scanControl(tx.QueryRow(ctx, `UPDATE steering_commands SET delivery_state='DISPATCHING',dispatched_at=clock_timestamp() WHERE steering_command_id=$1 RETURNING `+controlColumns, d.Command.ID))
	if err != nil {
		return nil, err
	}
	if err = workerAudit(ctx, tx, "steering.dispatching", b.Token.RunID, d); err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &d, nil
}
func (s *Store) SettleCodexControl(ctx context.Context, subject string, r codexexec.ControlSettlement) (codexexec.ControlDelivery, error) {
	var empty codexexec.ControlDelivery
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.workerTransaction(ctx)
	if err != nil {
		return empty, err
	}
	defer rollbackOutbox(tx)
	// Recording an observation after revocation grants no new execution authority.
	if _, _, err = s.lockCodex(ctx, tx, subject, r.Binding.Token, false); err != nil {
		return empty, err
	}
	runtime, err := lockControlRuntime(ctx, tx, r.Binding)
	if err != nil {
		return empty, err
	}
	d, err := scanControl(tx.QueryRow(ctx, `SELECT `+controlColumns+` FROM steering_commands WHERE steering_command_id=$1 AND execution_id=$2 FOR UPDATE`, r.ID, r.Binding.Token.ID))
	if err != nil {
		return empty, err
	}
	if d.Payload.Binding != r.Binding {
		return empty, workerqueue.ErrIdentity
	}
	allowed := r.Outcome == codexexec.ControlNotApplied || r.Outcome == codexexec.ControlUnknown || (r.Outcome == codexexec.ControlAccepted && d.Payload.Kind == codexexec.ControlSteer) || (r.Outcome == codexexec.ControlInterruptACK && d.Payload.Kind == codexexec.ControlInterrupt)
	if !allowed {
		return empty, workerqueue.ErrIdentity
	}
	if d.State == r.Outcome {
		return d, nil
	}
	if runtime.State != "ACTIVE" || d.State != codexexec.ControlDispatching {
		return empty, corestore.ErrConflict
	}
	d, err = scanControl(tx.QueryRow(ctx, `UPDATE steering_commands SET delivery_state=$2,resolved_at=clock_timestamp() WHERE steering_command_id=$1 RETURNING `+controlColumns, r.ID, r.Outcome))
	if err != nil {
		return empty, err
	}
	if err = workerAudit(ctx, tx, "steering.observed", r.Binding.Token.RunID, d); err != nil {
		return empty, err
	}
	if err = tx.Commit(ctx); err != nil {
		return empty, err
	}
	return d, nil
}
func (s *Store) CloseCodexControl(ctx context.Context, subject string, c codexexec.ControlClose) (codexexec.ControlTranscript, error) {
	var empty codexexec.ControlTranscript
	if c.Validate() != nil {
		return empty, workerqueue.ErrIdentity
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.workerTransaction(ctx)
	if err != nil {
		return empty, err
	}
	defer rollbackOutbox(tx)
	if _, _, err = s.lockCodex(ctx, tx, subject, c.Binding.Token, false); err != nil {
		return empty, err
	}
	runtime, err := lockControlRuntime(ctx, tx, c.Binding)
	if err != nil {
		return empty, err
	}
	if runtime.State == "SEALED" {
		if runtime.Transcript.Close != c {
			return empty, corestore.ErrConflict
		}
		return *runtime.Transcript, nil
	}
	// Only untouched QUEUED inputs are definitely not applied. A dispatched call
	// without a receipt remains UNKNOWN, even when a terminal event was observed.
	_, err = tx.Exec(ctx, `UPDATE steering_commands SET delivery_state=CASE WHEN delivery_state='QUEUED' THEN 'NOT_APPLIED' WHEN delivery_state='DISPATCHING' THEN 'UNKNOWN' WHEN delivery_state='INTERRUPT_ACKNOWLEDGED' AND $2='interrupted' THEN 'TURN_INTERRUPTED' ELSE delivery_state END,resolved_at=clock_timestamp() WHERE execution_id=$1 AND delivery_state IN ('QUEUED','DISPATCHING','INTERRUPT_ACKNOWLEDGED')`, c.Binding.Token.ID, c.TurnStatus)
	if err != nil {
		return empty, err
	}
	rows, err := tx.Query(ctx, `SELECT `+controlColumns+` FROM steering_commands WHERE execution_id=$1 ORDER BY sequence`, c.Binding.Token.ID)
	if err != nil {
		return empty, err
	}
	t := codexexec.ControlTranscript{Version: 1, Close: c, Deliveries: []codexexec.ControlDelivery{}}
	for rows.Next() {
		d, e := scanControl(rows)
		if e != nil {
			rows.Close()
			return empty, e
		}
		t.Deliveries = append(t.Deliveries, d)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return empty, err
	}
	digest, err := t.Digest()
	if err != nil {
		return empty, err
	}
	raw, _ := json.Marshal(t)
	if _, err = tx.Exec(ctx, `UPDATE worker_codex_runtime SET state='SEALED',transcript_json=$2::jsonb,transcript_digest=$3 WHERE execution_id=$1`, c.Binding.Token.ID, string(raw), digest); err != nil {
		return empty, err
	}
	if err = workerAudit(ctx, tx, "worker.codex.controls-sealed", c.Binding.Token.RunID, t); err != nil {
		return empty, err
	}
	if err = tx.Commit(ctx); err != nil {
		return empty, err
	}
	return t, nil
}
