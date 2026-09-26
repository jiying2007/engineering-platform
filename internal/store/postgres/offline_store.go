package postgres

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/offline"
	"github.com/jiying2007/engineering-platform/internal/preparation"
	"github.com/jiying2007/engineering-platform/internal/sandbox"
	corestore "github.com/jiying2007/engineering-platform/internal/store"
	"github.com/jiying2007/engineering-platform/internal/workerqueue"
)

var _ offline.Repository = (*Store)(nil)

type offlineRow struct {
	token                      offline.Token
	profile                    sandbox.Profile
	worker, preparation, state string
	until                      time.Time
	receipt                    *offline.Receipt
}

func scanOffline(row pgx.Row) (offlineRow, error) {
	var r offlineRow
	var token, profile, receipt []byte
	err := row.Scan(&token, &profile, &r.worker, &r.preparation, &r.state, &r.until, &receipt)
	if err != nil {
		return r, mapReadError(err)
	}
	if json.Unmarshal(token, &r.token) != nil || json.Unmarshal(profile, &r.profile) != nil || !r.token.Valid() {
		return r, workerqueue.ErrIdentity
	}
	digest, err := r.profile.Digest()
	if err != nil || digest != r.token.ProfileDigest {
		return r, workerqueue.ErrIdentity
	}
	if len(receipt) > 0 {
		r.receipt = &offline.Receipt{}
		if json.Unmarshal(receipt, r.receipt) != nil {
			return r, workerqueue.ErrIdentity
		}
	}
	return r, nil
}

const offlineColumns = `token_json,profile_json,worker_subject,preparation_digest,state,lease_until,receipt_json`

func (s *Store) StartOffline(ctx context.Context, subject string, req offline.Start) (offline.Permit, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	empty := offline.Permit{}
	pd, err := req.Profile.Digest()
	if err != nil || subject == "" || req.RunID == "" || !workerqueue.ValidProfile(req.WorkerProfile) {
		return empty, workerqueue.ErrIdentity
	}
	tx, err := s.workerTransaction(ctx)
	if err != nil {
		return empty, err
	}
	defer rollbackOutbox(tx)
	var ready bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM core_schema_migrations WHERE version=5) AND to_regclass('worker_offline_executions') IS NOT NULL`).Scan(&ready); err != nil || !ready {
		return empty, fmt.Errorf("offline schema version 5 is unavailable")
	}
	epoch, mode, err := lockWorkerPlatform(ctx, tx)
	if err != nil {
		return empty, err
	}
	if mode != "NORMAL" {
		return empty, workerqueue.ErrRecovery
	}
	// Serializing this identity also prevents overlapping offline reservations.
	if _, err = tx.Exec(ctx, `SELECT worker_id FROM workers WHERE worker_id=$1 FOR UPDATE`, subject); err != nil {
		return empty, err
	}
	var busy bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM worker_offline_executions WHERE worker_subject=$1 AND state='AUTHORIZED')`, subject).Scan(&busy); err != nil {
		return empty, err
	}
	if busy {
		return empty, corestore.ErrConflict
	}
	var codexReady bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM core_schema_migrations WHERE version=8)`).Scan(&codexReady); err != nil {
		return empty, err
	}
	if codexReady {
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM worker_codex_executions WHERE worker_subject=$1 AND state='AUTHORIZED')`, subject).Scan(&busy); err != nil {
			return empty, err
		}
		if busy {
			return empty, corestore.ErrConflict
		}
	}
	r, err := scanInbox(tx.QueryRow(ctx, `SELECT `+inboxColumns+` FROM worker_inbox WHERE run_id=$1 FOR UPDATE`, req.RunID))
	if err != nil {
		return empty, mapReadError(err)
	}
	if r.owner != subject || r.profile != req.WorkerProfile || r.state != workerqueue.Validated {
		return empty, workerqueue.ErrIdentity
	}
	if err = workerRunActive(ctx, tx, r); err != nil {
		return empty, err
	}
	a, err := loadWorkerAssignment(ctx, tx, r)
	if err != nil {
		return empty, err
	}
	if err = offline.CheckAssignment(a, req.Profile); err != nil {
		return empty, err
	}
	var raw []byte
	var prep preparation.Receipt
	if err = tx.QueryRow(ctx, `SELECT receipt_json FROM worker_preparations WHERE inbox_id=$1`, r.id).Scan(&raw); err != nil {
		return empty, mapReadError(err)
	}
	if json.Unmarshal(raw, &prep) != nil || preparation.Verify(a, prep, prep.Facts, subject) != nil {
		return empty, workerqueue.ErrIdentity
	}
	nonce := make([]byte, 32)
	if _, err = rand.Read(nonce); err != nil {
		return empty, err
	}
	token := offline.Token{ID: hex.EncodeToString(nonce), RunID: r.runID, WorkerProfile: r.profile, ProfileDigest: pd, RecoveryEpoch: epoch}
	traw, _ := json.Marshal(token)
	praw, _ := json.Marshal(req.Profile)
	if err = workerAudit(ctx, tx, "worker.offline.authorized", r.runID, token); err != nil {
		return empty, err
	}
	var until time.Time
	err = tx.QueryRow(ctx, `INSERT INTO worker_offline_executions(execution_id,run_id,inbox_id,worker_subject,token_json,profile_json,preparation_digest,state,lease_until)
 VALUES($1,$2,$3,$4,$5::jsonb,$6::jsonb,$7,'AUTHORIZED',clock_timestamp()+interval '20 seconds') RETURNING lease_until`, token.ID, r.runID, r.id, subject, string(traw), string(praw), prep.FactsDigest).Scan(&until)
	if err != nil {
		return empty, mapWriteError(err)
	}
	if err = tx.Commit(ctx); err != nil {
		return empty, err
	}
	return offline.Permit{Token: token, Assignment: a, Preparation: prep, Profile: req.Profile, LeaseUntil: until}, nil
}

// Lock order matches preparation: platform, inbox, Run/Session, execution,
// journal. Historical finish replay bypasses only live checks, never identity.
func (s *Store) lockOffline(ctx context.Context, tx pgx.Tx, subject string, t offline.Token, live bool) (offlineRow, error) {
	if subject == "" || !t.Valid() {
		return offlineRow{}, workerqueue.ErrIdentity
	}
	epoch, mode, err := lockWorkerPlatform(ctx, tx)
	if err != nil {
		return offlineRow{}, err
	}
	inbox, err := scanInbox(tx.QueryRow(ctx, `SELECT `+inboxColumns+` FROM worker_inbox WHERE run_id=$1 FOR UPDATE`, t.RunID))
	if err != nil {
		return offlineRow{}, mapReadError(err)
	}
	if inbox.owner != subject || inbox.profile != t.WorkerProfile {
		return offlineRow{}, workerqueue.ErrIdentity
	}
	if live {
		if mode != "NORMAL" || epoch != t.RecoveryEpoch {
			return offlineRow{}, workerqueue.ErrRecovery
		}
		if err = workerRunActive(ctx, tx, inbox); err != nil {
			return offlineRow{}, err
		}
	}
	row, err := scanOffline(tx.QueryRow(ctx, `SELECT `+offlineColumns+` FROM worker_offline_executions WHERE run_id=$1 FOR UPDATE`, t.RunID))
	if err != nil {
		return row, err
	}
	if row.token != t || row.worker != subject {
		return row, workerqueue.ErrIdentity
	}
	if live {
		var valid bool
		if err = tx.QueryRow(ctx, `SELECT state='AUTHORIZED' AND lease_until>clock_timestamp() FROM worker_offline_executions WHERE execution_id=$1`, t.ID).Scan(&valid); err != nil {
			return row, err
		}
		if !valid {
			return row, workerqueue.ErrLease
		}
	}
	return row, nil
}
func (s *Store) RenewOffline(ctx context.Context, subject string, t offline.Token) (time.Time, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.workerTransaction(ctx)
	if err != nil {
		return time.Time{}, err
	}
	defer rollbackOutbox(tx)
	if _, err = s.lockOffline(ctx, tx, subject, t, true); err != nil {
		return time.Time{}, err
	}
	if err = workerAudit(ctx, tx, "worker.offline.renewed", t.RunID, t); err != nil {
		return time.Time{}, err
	}
	var until time.Time
	err = tx.QueryRow(ctx, `UPDATE worker_offline_executions SET lease_until=LEAST(clock_timestamp()+interval '20 seconds',started_at+interval '120 seconds')
 WHERE execution_id=$1 AND lease_until>clock_timestamp() AND started_at+interval '120 seconds'>clock_timestamp() RETURNING lease_until`, t.ID).Scan(&until)
	if errors.Is(err, pgx.ErrNoRows) {
		return until, workerqueue.ErrLease
	}
	if err != nil {
		return until, err
	}
	return until, tx.Commit(ctx)
}
func (s *Store) FinishOffline(ctx context.Context, subject string, report offline.Report) (offline.Receipt, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	empty := offline.Receipt{}
	tx, err := s.workerTransaction(ctx)
	if err != nil {
		return empty, err
	}
	defer rollbackOutbox(tx)
	row, err := s.lockOffline(ctx, tx, subject, report.Token, false)
	if err != nil {
		return empty, err
	}
	if report.Result.Validate(row.profile) != nil {
		return empty, workerqueue.ErrIdentity
	}
	digest, err := canonical.Digest(report.Result)
	if err != nil {
		return empty, err
	}
	if row.state == offline.Finished && row.receipt != nil {
		actual, aerr := canonical.Digest(row.receipt.Result)
		if aerr != nil || actual != digest || row.receipt.ResultDigest != digest || row.receipt.Token != report.Token || row.receipt.Worker != subject || row.receipt.Kind != offline.Kind {
			return empty, workerqueue.ErrIdentity
		}
		return *row.receipt, nil
	}
	// Inbox serialization is retained while the live Run/Session checks are made.
	if _, err = s.lockOffline(ctx, tx, subject, report.Token, true); err != nil {
		return empty, err
	}
	var now time.Time
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return empty, err
	}
	receipt := offline.Receipt{Kind: offline.Kind, Token: report.Token, Worker: subject, PreparationDigest: row.preparation, Result: report.Result, ResultDigest: digest, ReceivedAt: now}
	if err = workerAudit(ctx, tx, "worker.offline.finished", report.Token.RunID, receipt); err != nil {
		return empty, err
	}
	raw, _ := json.Marshal(receipt)
	tag, err := tx.Exec(ctx, `UPDATE worker_offline_executions SET state='FINISHED',receipt_json=$2::jsonb WHERE execution_id=$1 AND state='AUTHORIZED' AND lease_until>clock_timestamp()`, report.Token.ID, string(raw))
	if err != nil {
		return empty, err
	}
	if tag.RowsAffected() != 1 {
		return empty, workerqueue.ErrLease
	}
	if err = tx.Commit(ctx); err != nil {
		return empty, err
	}
	return receipt, nil
}
func (s *Store) FailOffline(ctx context.Context, subject string, t offline.Token) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.workerTransaction(ctx)
	if err != nil {
		return err
	}
	defer rollbackOutbox(tx)
	row, err := s.lockOffline(ctx, tx, subject, t, false)
	if err != nil {
		return err
	}
	if row.state != offline.Authorized {
		return nil
	}
	if err = workerAudit(ctx, tx, "worker.offline.unknown", t.RunID, t); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE worker_offline_executions SET state='UNKNOWN' WHERE execution_id=$1`, t.ID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (s *Store) GetOffline(ctx context.Context, runID string) (offline.Status, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	r, err := scanOffline(s.pool.QueryRow(ctx, `SELECT `+offlineColumns+` FROM worker_offline_executions WHERE run_id=$1`, runID))
	if err != nil {
		return offline.Status{}, err
	}
	if r.state == offline.Authorized {
		var expired bool
		if err = s.pool.QueryRow(ctx, `SELECT lease_until<=clock_timestamp() FROM worker_offline_executions WHERE run_id=$1`, runID).Scan(&expired); err != nil {
			return offline.Status{}, err
		}
		if expired {
			r.state = "EXPIRED_UNRECONCILED"
		}
	}
	return offline.Status{Token: r.token, State: r.state, LeaseUntil: r.until, Receipt: r.receipt}, nil
}
