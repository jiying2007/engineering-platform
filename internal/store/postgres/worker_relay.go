package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jiying2007/engineering-platform/internal/action"
	"github.com/jiying2007/engineering-platform/internal/outbox"
	"github.com/jiying2007/engineering-platform/internal/strictjson"
	"github.com/jiying2007/engineering-platform/internal/workerqueue"
)

// RelayRunStarts is a LOCAL atomic transfer, not an external callback. It reuses
// outbox receipt/audit helpers and never opens a second transaction while holding
// the first. Unregistered topics remain untouched for their own recipients.
func (s *Store) RelayRunStarts(ctx context.Context, owner string, limit int) (int, error) {
	if s == nil || s.pool == nil || owner == "" || limit < 1 || limit > 64 {
		return 0, fmt.Errorf("invalid local relay configuration")
	}
	transferred := 0
	for i := 0; i < limit; i++ {
		step, cancel := context.WithTimeout(ctx, 5*time.Second)
		found, err := s.relayRunStart(step, owner)
		cancel()
		if err != nil {
			return transferred, err
		}
		if !found {
			return transferred, nil
		}
		transferred++
	}
	return transferred, nil
}
func (s *Store) relayRunStart(ctx context.Context, owner string) (bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer rollbackOutbox(tx)
	var epoch uint64
	var mode string
	if err := tx.QueryRow(ctx, `SELECT recovery_epoch,recovery_mode FROM platform_state WHERE singleton_id=true FOR SHARE`).Scan(&epoch, &mode); err != nil {
		return false, err
	}
	if mode != "NORMAL" {
		if mode == "RECOVERY_RECONCILIATION" {
			return false, nil
		}
		return false, workerqueue.ErrRecovery
	}
	m, err := scanOutbox(tx.QueryRow(ctx, `SELECT `+outboxColumns+` FROM outbox_events
  WHERE topic='run.started' AND next_attempt_at<=clock_timestamp()
  AND (state='PENDING' OR (state='LEASED' AND lease_until<=clock_timestamp()))
  ORDER BY outbox_id LIMIT 1 FOR UPDATE SKIP LOCKED`))
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if m.RiskClass != action.ControlledMutation {
		return false, outbox.ErrRiskClass
	}
	m.LeaseOwner = owner
	m.AttemptCount++
	m.LeaseRecoveryEpoch = epoch
	if _, err := tx.Exec(ctx, `UPDATE outbox_events SET state='LEASED',lease_owner=$2,attempt_count=$3,
  lease_until=clock_timestamp()+interval '30 seconds',lease_recovery_epoch=$4 WHERE outbox_id=$1`, m.ID, owner, m.AttemptCount, epoch); err != nil {
		return false, err
	}
	var intent workerqueue.Intent
	resolution := outbox.Resolution{State: outbox.Dispatched}
	digest := ""
	if strictjson.Decode(m.Payload, &intent) != nil || m.AggregateType != "Run" || m.AggregateID != intent.RunID || m.Key != "run:"+intent.RunID+":started" {
		resolution = outbox.Resolution{State: outbox.DeadLetter, LastError: "invalid run intent"}
	} else {
		digest, err = intent.Digest()
		if err == nil {
			err = s.insertWorkerIntent(ctx, tx, m.Key, intent, digest)
		}
		if errors.Is(err, workerqueue.ErrIdentity) || errors.Is(err, workerqueue.ErrInactive) {
			resolution = outbox.Resolution{State: outbox.DeadLetter, LastError: "run intent identity revoked or mismatched"}
		} else if err != nil {
			return false, err
		}
	}
	if err := settleOutbox(ctx, tx, m.Lease(), resolution); err != nil {
		return false, err
	}
	record, err := auditInput("worker.inbox.transferred", "Run", m.AggregateID, struct {
		Key    string       `json:"outbox_key"`
		Digest string       `json:"intent_digest"`
		State  outbox.State `json:"outbox_state"`
	}{m.Key, digest, resolution.State})
	if err != nil {
		return false, err
	}
	if _, err := appendAudit(ctx, tx, record, time.Now().UTC()); err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}
func (s *Store) insertWorkerIntent(ctx context.Context, tx pgx.Tx, key string, intent workerqueue.Intent, digest string) error {
	// A paused Run may receive durable intent but cannot be claimed. The receiver
	// checks its current Run/Session authority again at claim, renew and report.
	var taskDigest, inputDigest, state, owner string
	var epoch uint64
	if err := tx.QueryRow(ctx, `SELECT task_contract_digest,run_input_manifest_digest,current_epoch,state,control_owner
  FROM runs WHERE run_id=$1 FOR SHARE`, intent.RunID).Scan(&taskDigest, &inputDigest, &epoch, &state, &owner); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return workerqueue.ErrIdentity
		}
		return err
	}
	if taskDigest != intent.TaskDigest || inputDigest != intent.InputDigest || epoch != intent.ExecutionEpoch {
		return workerqueue.ErrIdentity
	}
	if owner != "RUNTIME" || (state != "RUNNING" && state != "PAUSED") {
		return workerqueue.ErrInactive
	}
	var profile string
	if err := tx.QueryRow(ctx, `SELECT worker_profile FROM run_input_manifests WHERE run_input_manifest_digest=$1 AND run_id=$2`, inputDigest, intent.RunID).Scan(&profile); err != nil {
		return err
	}
	if !workerqueue.ValidProfile(profile) {
		return workerqueue.ErrIdentity
	}
	payload, err := json.Marshal(intent)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO worker_inbox(outbox_key,run_id,intent_digest,intent_json,worker_profile)
  VALUES($1,$2,$3,$4::jsonb,$5) ON CONFLICT DO NOTHING`, key, intent.RunID, digest, string(payload), profile)
	if err != nil {
		return err
	}
	var storedDigest, storedRun, storedProfile string
	if err := tx.QueryRow(ctx, `SELECT intent_digest,run_id,worker_profile FROM worker_inbox WHERE outbox_key=$1`, key).Scan(&storedDigest, &storedRun, &storedProfile); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return workerqueue.ErrIdentity
		}
		return err
	}
	if storedDigest != digest || storedRun != intent.RunID || storedProfile != profile {
		return workerqueue.ErrIdentity
	}
	return nil
}
func (s *Store) CheckWorkerSchema(ctx context.Context) error {
	if s == nil || s.pool == nil {
		return fmt.Errorf("PostgreSQL store required")
	}
	var present bool
	if err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM core_schema_migrations WHERE version=3)`).Scan(&present); err != nil {
		return err
	}
	if !present {
		return fmt.Errorf("worker inbox migration 0003 is required")
	}
	return nil
}
