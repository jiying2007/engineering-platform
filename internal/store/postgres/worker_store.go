package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jiying2007/engineering-platform/internal/canonical"
	corestore "github.com/jiying2007/engineering-platform/internal/store"
	"github.com/jiying2007/engineering-platform/internal/workerqueue"
)

var _ workerqueue.Repository = (*Store)(nil)

const inboxColumns = `inbox_id,run_id,state,lease_generation,COALESCE(lease_owner,''),
 COALESCE(lease_recovery_epoch,0),intent_digest,intent_json,worker_profile,receipt_json`

type inboxRecord struct {
	id                                  int64
	runID, state, owner, digest, profile string
	generation, epoch                   uint64
	intent                              workerqueue.Intent
	receipt                             *workerqueue.Receipt
}

func scanInbox(row pgx.Row) (inboxRecord, error) {
	var r inboxRecord
	var raw, receipt []byte
	err := row.Scan(&r.id, &r.runID, &r.state, &r.generation, &r.owner, &r.epoch, &r.digest, &raw, &r.profile, &receipt)
	if err != nil {
		return r, err
	}
	if err = json.Unmarshal(raw, &r.intent); err != nil {
		return r, workerqueue.ErrIdentity
	}
	digest, err := r.intent.Digest()
	if err != nil || digest != r.digest || r.intent.RunID != r.runID {
		return r, workerqueue.ErrIdentity
	}
	if len(receipt) > 0 {
		var value workerqueue.Receipt
		if err = json.Unmarshal(receipt, &value); err != nil {
			return r, err
		}
		r.receipt = &value
	}
	return r, nil
}
func (r inboxRecord) token() workerqueue.Token {
	return workerqueue.Token{InboxID: r.id, Generation: r.generation, RecoveryEpoch: r.epoch, Profile: r.profile}
}
func (s *Store) workerTransaction(ctx context.Context) (pgx.Tx, error) {
	if s == nil || s.pool == nil {
		return nil, fmt.Errorf("PostgreSQL store required")
	}
	return s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
}
func lockWorkerPlatform(ctx context.Context, tx pgx.Tx) (uint64, string, error) {
	var epoch uint64
	var mode string
	err := tx.QueryRow(ctx, `SELECT recovery_epoch,recovery_mode FROM platform_state WHERE singleton_id=true FOR SHARE`).Scan(&epoch, &mode)
	if err != nil {
		return 0, "", err
	}
	if mode != "NORMAL" && mode != "RECOVERY_RECONCILIATION" {
		return 0, "", workerqueue.ErrRecovery
	}
	return epoch, mode, nil
}
func workerRunActive(ctx context.Context, tx pgx.Tx, r inboxRecord) error {
	var task, input, state, owner, sessionOwner string
	var epoch, sessionEpoch uint64
	var paused bool
	err := tx.QueryRow(ctx, `SELECT task_contract_digest,run_input_manifest_digest,current_epoch,state,control_owner
 FROM runs WHERE run_id=$1 FOR SHARE`, r.runID).Scan(&task, &input, &epoch, &state, &owner)
	if err != nil {
		return err
	}
	err = tx.QueryRow(ctx, `SELECT execution_epoch,control_owner,paused FROM sessions WHERE run_id=$1 FOR SHARE`, r.runID).Scan(&sessionEpoch, &sessionOwner, &paused)
	if err != nil {
		return err
	}
	if epoch != r.intent.ExecutionEpoch || task != r.intent.TaskDigest || input != r.intent.InputDigest || sessionEpoch != epoch {
		return workerqueue.ErrIdentity
	}
	if owner != "RUNTIME" || sessionOwner != "RUNTIME" || (state != "RUNNING" && state != "PAUSED") {
		return workerqueue.ErrInactive
	}
	if state == "PAUSED" || paused {
		return workerqueue.ErrPaused
	}
	return nil
}
func loadWorkerAssignment(ctx context.Context, tx pgx.Tx, r inboxRecord) (workerqueue.Assignment, error) {
	a := workerqueue.Assignment{Token: r.token(), Intent: r.intent, IntentDigest: r.digest}
	var input, task []byte
	if err := tx.QueryRow(ctx, `SELECT m.manifest_json,t.contract_json FROM run_input_manifests m
 JOIN task_contracts t ON t.content_digest=m.task_contract_digest
 WHERE m.run_input_manifest_digest=$1 AND m.run_id=$2`, r.intent.InputDigest, r.runID).Scan(&input, &task); err != nil {
		return a, err
	}
	if json.Unmarshal(input, &a.Input) != nil || json.Unmarshal(task, &a.Task) != nil {
		return a, workerqueue.ErrIdentity
	}
	if a.Input.WorkerProfile != r.profile {
		return a, workerqueue.ErrIdentity
	}
	if _, err := workerqueue.Validate(a); err != nil {
		return a, err
	}
	return a, nil
}
func workerAudit(ctx context.Context, tx pgx.Tx, event, runID string, value any) error {
	record, err := auditInput(event, "Run", runID, value)
	if err != nil {
		return err
	}
	_, err = appendAudit(ctx, tx, record, time.Now().UTC())
	return err
}

func (s *Store) ClaimInput(ctx context.Context, subject, profile string) (*workerqueue.Assignment, error) {
	if subject == "" || len(subject) > 256 || !workerqueue.ValidProfile(profile) {
		return nil, workerqueue.ErrIdentity
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	// Bounded maintenance: revoked intents cannot permanently starve later work.
	for n := 0; n < 16; n++ {
		a, revoked, err := s.claimInput(ctx, subject, profile)
		if err != nil || !revoked {
			return a, err
		}
	}
	return nil, nil
}
func (s *Store) claimInput(ctx context.Context, subject, profile string) (*workerqueue.Assignment, bool, error) {
	tx, err := s.workerTransaction(ctx)
	if err != nil {
		return nil, false, err
	}
	defer rollbackOutbox(tx)
	epoch, mode, err := lockWorkerPlatform(ctx, tx)
	if err != nil {
		return nil, false, err
	}
	if mode != "NORMAL" {
		return nil, false, workerqueue.ErrRecovery
	}
	// Serializes polls from a reused identity. This heartbeat is not evidence of
	// a runtime provider, firmware capability or a trusted device.
	_, err = tx.Exec(ctx, `INSERT INTO workers(worker_id,protocol_version,runtime_providers,status,attributes_json,last_seen_at)
 VALUES($1,$2,'[]','ONLINE',jsonb_build_object('worker_profile',$3::text),clock_timestamp())
 ON CONFLICT(worker_id) DO UPDATE SET protocol_version=EXCLUDED.protocol_version,status='ONLINE',
 attributes_json=EXCLUDED.attributes_json,last_seen_at=clock_timestamp(),updated_at=clock_timestamp()`, subject, workerqueue.Protocol, profile)
	if err != nil {
		return nil, false, err
	}
	var busy bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM worker_inbox WHERE state='LEASED' AND lease_owner=$1 AND lease_until>clock_timestamp())`, subject).Scan(&busy); err != nil {
		return nil, false, err
	}
	if busy {
		return nil, false, tx.Commit(ctx)
	}
	r, err := scanInbox(tx.QueryRow(ctx, `SELECT `+inboxColumns+` FROM worker_inbox i WHERE worker_profile=$1
 AND (state='PENDING' OR (state='LEASED' AND lease_until<=clock_timestamp()))
 AND NOT EXISTS(SELECT 1 FROM runs r JOIN sessions s USING(run_id) WHERE r.run_id=i.run_id AND r.current_epoch=(i.intent_json->>'execution_epoch')::bigint AND r.control_owner='RUNTIME'
 AND (r.state='PAUSED' OR (r.state='RUNNING' AND s.paused)))
 ORDER BY inbox_id LIMIT 1 FOR UPDATE OF i SKIP LOCKED`, profile))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, tx.Commit(ctx)
	}
	if err != nil {
		return nil, false, err
	}
	if err := workerRunActive(ctx, tx, r); err != nil {
		// A pause committed after candidate selection is transient. Roll back the
		// attempted claim, not the durable intent; a later resume may claim it.
		if errors.Is(err, workerqueue.ErrPaused) {
			return nil, false, nil
		}
		if !errors.Is(err, workerqueue.ErrIdentity) && !errors.Is(err, workerqueue.ErrInactive) {
			return nil, false, err
		}
		if _, err := tx.Exec(ctx, `UPDATE worker_inbox SET state='REVOKED',lease_owner=NULL,lease_until=NULL,updated_at=clock_timestamp() WHERE inbox_id=$1`, r.id); err != nil {
			return nil, false, err
		}
		if err := workerAudit(ctx, tx, "worker.input.revoked", r.runID, r.intent); err != nil {
			return nil, false, err
		}
		return nil, true, tx.Commit(ctx)
	}
	r.generation++
	r.epoch = epoch
	r.owner = subject
	a, err := loadWorkerAssignment(ctx, tx, r)
	if err != nil {
		return nil, false, err
	}
	err = tx.QueryRow(ctx, `UPDATE worker_inbox SET state='LEASED',lease_owner=$2,lease_generation=$3,
 lease_recovery_epoch=$4,lease_started_at=clock_timestamp(),lease_until=clock_timestamp()+interval '30 seconds',updated_at=clock_timestamp()
 WHERE inbox_id=$1 RETURNING lease_until`, r.id, subject, r.generation, epoch).Scan(&a.LeaseUntil)
	if err != nil {
		return nil, false, err
	}
	if err := workerAudit(ctx, tx, "worker.input.claimed", r.runID, struct {
		Worker string            `json:"worker_subject"`
		Token  workerqueue.Token `json:"token"`
	}{subject, a.Token}); err != nil {
		return nil, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, false, err
	}
	return &a, false, nil
}
func lockWorkerLease(ctx context.Context, tx pgx.Tx, subject string, token workerqueue.Token) (inboxRecord, uint64, string, error) {
	if !token.Valid() || subject == "" {
		return inboxRecord{}, 0, "", workerqueue.ErrLease
	}
	epoch, mode, err := lockWorkerPlatform(ctx, tx)
	if err != nil {
		return inboxRecord{}, 0, "", err
	}
	r, err := scanInbox(tx.QueryRow(ctx, `SELECT `+inboxColumns+` FROM worker_inbox WHERE inbox_id=$1 FOR UPDATE`, token.InboxID))
	if errors.Is(err, pgx.ErrNoRows) {
		return r, epoch, mode, workerqueue.ErrLease
	}
	if err != nil {
		return r, epoch, mode, err
	}
	if r.owner != subject || r.token() != token {
		return r, epoch, mode, workerqueue.ErrLease
	}
	return r, epoch, mode, nil
}
func liveWorkerLease(ctx context.Context, tx pgx.Tx, r inboxRecord, epoch uint64, mode string) error {
	if mode != "NORMAL" || r.epoch != epoch {
		return workerqueue.ErrRecovery
	}
	if r.state != "LEASED" {
		return workerqueue.ErrLease
	}
	if err := workerRunActive(ctx, tx, r); err != nil {
		return err
	}
	var live bool
	if err := tx.QueryRow(ctx, `SELECT lease_until>clock_timestamp() FROM worker_inbox WHERE inbox_id=$1`, r.id).Scan(&live); err != nil {
		return err
	}
	if !live {
		return workerqueue.ErrLease
	}
	return nil
}
func (s *Store) RenewInput(ctx context.Context, subject string, token workerqueue.Token) (time.Time, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.workerTransaction(ctx)
	if err != nil {
		return time.Time{}, err
	}
	defer rollbackOutbox(tx)
	r, epoch, mode, err := lockWorkerLease(ctx, tx, subject, token)
	if err != nil {
		return time.Time{}, err
	}
	if err := liveWorkerLease(ctx, tx, r, epoch, mode); err != nil {
		return time.Time{}, err
	}
	if err := workerAudit(ctx, tx, "worker.input.renewed", r.runID, struct {
		Worker string            `json:"worker_subject"`
		Token  workerqueue.Token `json:"token"`
	}{subject, token}); err != nil {
		return time.Time{}, err
	}
	var until time.Time
	err = tx.QueryRow(ctx, `UPDATE worker_inbox SET lease_until=LEAST(clock_timestamp()+interval '30 seconds',lease_started_at+interval '2 minutes'),updated_at=clock_timestamp()
 WHERE inbox_id=$1 AND lease_until>clock_timestamp() AND lease_started_at+interval '2 minutes'>clock_timestamp() RETURNING lease_until`, r.id).Scan(&until)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, workerqueue.ErrLease
	}
	if err != nil {
		return time.Time{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return time.Time{}, err
	}
	return until, nil
}
func (s *Store) ReportInput(ctx context.Context, subject string, report workerqueue.Report) (workerqueue.Receipt, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.workerTransaction(ctx)
	if err != nil {
		return workerqueue.Receipt{}, err
	}
	defer rollbackOutbox(tx)
	r, epoch, mode, err := lockWorkerLease(ctx, tx, subject, report.Token)
	if err != nil {
		return workerqueue.Receipt{}, err
	}
	// A duplicate acknowledgement is a read of retained history, never authority
	// to execute. Return it unchanged even if recovery has since begun.
	if r.state == workerqueue.Validated && r.receipt != nil {
		digest, err := canonical.Digest(r.receipt.Validation)
		if err != nil || digest != report.ValidationDigest || r.receipt.Token != report.Token || r.receipt.Worker != subject {
			return workerqueue.Receipt{}, workerqueue.ErrIdentity
		}
		return *r.receipt, nil
	}
	if err := liveWorkerLease(ctx, tx, r, epoch, mode); err != nil {
		return workerqueue.Receipt{}, err
	}
	a, err := loadWorkerAssignment(ctx, tx, r)
	if err != nil {
		return workerqueue.Receipt{}, err
	}
	expected, err := workerqueue.NewReport(a)
	if err != nil || expected != report {
		return workerqueue.Receipt{}, workerqueue.ErrIdentity
	}
	v, err := workerqueue.Validate(a)
	if err != nil {
		return workerqueue.Receipt{}, err
	}
	receipt := workerqueue.Receipt{Token: report.Token, Worker: subject, Kind: workerqueue.Validated, Validation: v}
	if err := tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&receipt.ReceivedAt); err != nil {
		return receipt, err
	}
	raw, err := json.Marshal(receipt)
	if err != nil {
		return receipt, err
	}
	if err := workerAudit(ctx, tx, "worker.input.validated", r.runID, receipt); err != nil {
		return receipt, err
	}
	tag, err := tx.Exec(ctx, `UPDATE worker_inbox SET state='INPUT_VALIDATED',receipt_json=$2::jsonb,lease_until=NULL,updated_at=clock_timestamp()
 WHERE inbox_id=$1 AND lease_until>clock_timestamp()`, r.id, string(raw))
	if err != nil {
		return receipt, err
	}
	if tag.RowsAffected() != 1 {
		return receipt, workerqueue.ErrLease
	}
	if err := tx.Commit(ctx); err != nil {
		return receipt, err
	}
	return receipt, nil
}
func (s *Store) GetInbox(ctx context.Context, runID string) (workerqueue.Status, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if s == nil || s.pool == nil {
		return workerqueue.Status{}, fmt.Errorf("PostgreSQL store required")
	}
	r, err := scanInbox(s.pool.QueryRow(ctx, `SELECT `+inboxColumns+` FROM worker_inbox WHERE run_id=$1`, runID))
	if errors.Is(err, pgx.ErrNoRows) {
		return workerqueue.Status{}, corestore.ErrNotFound
	}
	if err != nil {
		return workerqueue.Status{}, err
	}
	return workerqueue.Status{InboxID: r.id, RunID: r.runID, State: r.state, Generation: r.generation, Receipt: r.receipt}, nil
}
