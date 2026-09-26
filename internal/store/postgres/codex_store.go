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
	"github.com/jiying2007/engineering-platform/internal/codexexec"
	"github.com/jiying2007/engineering-platform/internal/preparation"
	corestore "github.com/jiying2007/engineering-platform/internal/store"
	"github.com/jiying2007/engineering-platform/internal/workerqueue"
)

var _ codexexec.Repository = (*Store)(nil)

type codexRow struct {
	token                      codexexec.Token
	profile                    codexexec.Profile
	worker, preparation, state string
	until                      time.Time
	receipt                    *codexexec.Receipt
}

func scanCodex(row pgx.Row) (codexRow, error) {
	var r codexRow
	var token, profile, receipt []byte
	err := row.Scan(&token, &profile, &r.worker, &r.preparation, &r.state, &r.until, &receipt)
	if err != nil {
		return r, mapReadError(err)
	}
	if json.Unmarshal(token, &r.token) != nil || json.Unmarshal(profile, &r.profile) != nil ||
		!r.token.Valid() || r.profile.Validate() != nil {
		return r, workerqueue.ErrIdentity
	}
	digest, err := r.profile.Digest()
	if err != nil || digest != r.token.ProfileDigest {
		return r, workerqueue.ErrIdentity
	}
	if len(receipt) > 0 {
		r.receipt = &codexexec.Receipt{}
		if json.Unmarshal(receipt, r.receipt) != nil {
			return r, workerqueue.ErrIdentity
		}
	}
	return r, nil
}

const codexColumns = `token_json,profile_json,worker_subject,preparation_digest,state,lease_until,receipt_json`

func (s *Store) StartCodex(ctx context.Context, subject string, req codexexec.Start) (codexexec.Permit, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	empty := codexexec.Permit{}
	profileDigest, err := req.Profile.Digest()
	if err != nil || subject == "" || req.RunID == "" || !workerqueue.ValidProfile(req.WorkerProfile) {
		return empty, workerqueue.ErrIdentity
	}
	tx, err := s.workerTransaction(ctx)
	if err != nil {
		return empty, err
	}
	defer rollbackOutbox(tx)
	var ready bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM core_schema_migrations WHERE version=8) AND to_regclass('worker_codex_executions') IS NOT NULL`).Scan(&ready); err != nil || !ready {
		return empty, fmt.Errorf("Codex execution schema version 8 is unavailable")
	}
	epoch, mode, err := lockWorkerPlatform(ctx, tx)
	if err != nil {
		return empty, err
	}
	if mode != "NORMAL" {
		return empty, workerqueue.ErrRecovery
	}
	if _, err = tx.Exec(ctx, `SELECT worker_id FROM workers WHERE worker_id=$1 FOR UPDATE`, subject); err != nil {
		return empty, err
	}
	var busy bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(
 SELECT 1 FROM worker_codex_executions WHERE worker_subject=$1 AND state='AUTHORIZED'
 UNION ALL
 SELECT 1 FROM worker_offline_executions WHERE worker_subject=$1 AND state='AUTHORIZED'
)`, subject).Scan(&busy); err != nil {
		return empty, err
	}
	if busy {
		return empty, corestore.ErrConflict
	}
	inbox, err := scanInbox(tx.QueryRow(ctx, `SELECT `+inboxColumns+` FROM worker_inbox WHERE run_id=$1 FOR UPDATE`, req.RunID))
	if err != nil {
		return empty, mapReadError(err)
	}
	if inbox.owner != subject || inbox.profile != req.WorkerProfile || inbox.state != workerqueue.Validated {
		return empty, workerqueue.ErrIdentity
	}
	if err = workerRunActive(ctx, tx, inbox); err != nil {
		return empty, err
	}
	assignment, err := loadWorkerAssignment(ctx, tx, inbox)
	if err != nil {
		return empty, err
	}
	if err = codexexec.CheckAssignment(assignment, req.Profile); err != nil {
		return empty, err
	}
	var prepRaw []byte
	var prep preparation.Receipt
	if err = tx.QueryRow(ctx, `SELECT receipt_json FROM worker_preparations WHERE inbox_id=$1`, inbox.id).Scan(&prepRaw); err != nil {
		return empty, mapReadError(err)
	}
	if json.Unmarshal(prepRaw, &prep) != nil || preparation.Verify(assignment, prep, prep.Facts, subject) != nil {
		return empty, workerqueue.ErrIdentity
	}
	nonce := make([]byte, 32)
	if _, err = rand.Read(nonce); err != nil {
		return empty, err
	}
	token := codexexec.Token{
		ID: hex.EncodeToString(nonce), RunID: inbox.runID, WorkerProfile: inbox.profile,
		ProfileDigest: profileDigest, RecoveryEpoch: epoch,
	}
	tokenRaw, _ := json.Marshal(token)
	profileRaw, _ := json.Marshal(req.Profile)
	if err = workerAudit(ctx, tx, "worker.codex.authorized", inbox.runID, token); err != nil {
		return empty, err
	}
	var until time.Time
	err = tx.QueryRow(ctx, `INSERT INTO worker_codex_executions(
 execution_id,run_id,inbox_id,worker_subject,token_json,profile_json,preparation_digest,state,lease_until)
 VALUES($1,$2,$3,$4,$5::jsonb,$6::jsonb,$7,'AUTHORIZED',clock_timestamp()+interval '30 seconds')
 RETURNING lease_until`,
		token.ID, inbox.runID, inbox.id, subject, string(tokenRaw), string(profileRaw), prep.FactsDigest).Scan(&until)
	if err != nil {
		return empty, mapWriteError(err)
	}
	if err = tx.Commit(ctx); err != nil {
		return empty, err
	}
	return codexexec.Permit{Token: token, Assignment: assignment, Preparation: prep, Profile: req.Profile, LeaseUntil: until}, nil
}

func (s *Store) lockCodex(ctx context.Context, tx pgx.Tx, subject string, token codexexec.Token, live bool) (codexRow, inboxRecord, error) {
	if subject == "" || !token.Valid() {
		return codexRow{}, inboxRecord{}, workerqueue.ErrIdentity
	}
	epoch, mode, err := lockWorkerPlatform(ctx, tx)
	if err != nil {
		return codexRow{}, inboxRecord{}, err
	}
	inbox, err := scanInbox(tx.QueryRow(ctx, `SELECT `+inboxColumns+` FROM worker_inbox WHERE run_id=$1 FOR UPDATE`, token.RunID))
	if err != nil {
		return codexRow{}, inboxRecord{}, mapReadError(err)
	}
	if inbox.owner != subject || inbox.profile != token.WorkerProfile {
		return codexRow{}, inbox, workerqueue.ErrIdentity
	}
	if live {
		if mode != "NORMAL" || epoch != token.RecoveryEpoch {
			return codexRow{}, inbox, workerqueue.ErrRecovery
		}
		if err = workerRunActive(ctx, tx, inbox); err != nil {
			return codexRow{}, inbox, err
		}
	}
	row, err := scanCodex(tx.QueryRow(ctx, `SELECT `+codexColumns+` FROM worker_codex_executions WHERE run_id=$1 FOR UPDATE`, token.RunID))
	if err != nil {
		return row, inbox, err
	}
	if row.token != token || row.worker != subject {
		return row, inbox, workerqueue.ErrIdentity
	}
	if live {
		var valid bool
		if err = tx.QueryRow(ctx, `SELECT state='AUTHORIZED' AND lease_until>clock_timestamp()
 FROM worker_codex_executions WHERE execution_id=$1`, token.ID).Scan(&valid); err != nil {
			return row, inbox, err
		}
		if !valid {
			return row, inbox, workerqueue.ErrLease
		}
	}
	return row, inbox, nil
}

func (s *Store) RenewCodex(ctx context.Context, subject string, token codexexec.Token) (time.Time, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.workerTransaction(ctx)
	if err != nil {
		return time.Time{}, err
	}
	defer rollbackOutbox(tx)
	if _, _, err = s.lockCodex(ctx, tx, subject, token, true); err != nil {
		return time.Time{}, err
	}
	if err = workerAudit(ctx, tx, "worker.codex.renewed", token.RunID, token); err != nil {
		return time.Time{}, err
	}
	var until time.Time
	err = tx.QueryRow(ctx, `UPDATE worker_codex_executions
 SET lease_until=LEAST(clock_timestamp()+interval '30 seconds',started_at+interval '15 minutes')
 WHERE execution_id=$1 AND lease_until>clock_timestamp()
   AND started_at+interval '15 minutes'>clock_timestamp()
 RETURNING lease_until`, token.ID).Scan(&until)
	if errors.Is(err, pgx.ErrNoRows) {
		return until, workerqueue.ErrLease
	}
	if err != nil {
		return until, err
	}
	return until, tx.Commit(ctx)
}

func (s *Store) FinishCodex(ctx context.Context, subject string, report codexexec.Report) (codexexec.Receipt, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	empty := codexexec.Receipt{}
	tx, err := s.workerTransaction(ctx)
	if err != nil {
		return empty, err
	}
	defer rollbackOutbox(tx)
	row, inbox, err := s.lockCodex(ctx, tx, subject, report.Token, false)
	if err != nil {
		return empty, err
	}
	assignment, err := loadWorkerAssignment(ctx, tx, inbox)
	if err != nil {
		return empty, err
	}
	var prepRaw []byte
	var prep preparation.Receipt
	if err = tx.QueryRow(ctx, `SELECT receipt_json FROM worker_preparations WHERE inbox_id=$1`, inbox.id).Scan(&prepRaw); err != nil {
		return empty, mapReadError(err)
	}
	if json.Unmarshal(prepRaw, &prep) != nil || preparation.Verify(assignment, prep, prep.Facts, subject) != nil {
		return empty, workerqueue.ErrIdentity
	}
	permit := codexexec.Permit{Token: row.token, Assignment: assignment, Preparation: prep, Profile: row.profile, LeaseUntil: row.until}
	if report.Result.Validate(row.profile, permit) != nil {
		return empty, workerqueue.ErrIdentity
	}
	digest, err := canonical.Digest(report.Result)
	if err != nil {
		return empty, err
	}
	if row.state == codexexec.Finished && row.receipt != nil {
		actual, actualErr := canonical.Digest(row.receipt.Result)
		if actualErr != nil || actual != digest || row.receipt.ResultDigest != digest ||
			row.receipt.Token != report.Token || row.receipt.Worker != subject || row.receipt.Kind != codexexec.Kind {
			return empty, workerqueue.ErrIdentity
		}
		return *row.receipt, nil
	}
	if _, _, err = s.lockCodex(ctx, tx, subject, report.Token, true); err != nil {
		return empty, err
	}
	var now time.Time
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return empty, err
	}
	receipt := codexexec.Receipt{
		Kind: codexexec.Kind, Token: report.Token, Worker: subject,
		PreparationDigest: row.preparation, Result: report.Result, ResultDigest: digest, ReceivedAt: now,
	}
	if err = receipt.Verify(subject, permit, report.Result); err != nil {
		return empty, err
	}
	if err = workerAudit(ctx, tx, "worker.codex.finished", report.Token.RunID, receipt); err != nil {
		return empty, err
	}
	raw, _ := json.Marshal(receipt)
	tag, err := tx.Exec(ctx, `UPDATE worker_codex_executions
 SET state='FINISHED',receipt_json=$2::jsonb
 WHERE execution_id=$1 AND state='AUTHORIZED' AND lease_until>clock_timestamp()`, report.Token.ID, string(raw))
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

func (s *Store) FailCodex(ctx context.Context, subject string, token codexexec.Token) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.workerTransaction(ctx)
	if err != nil {
		return err
	}
	defer rollbackOutbox(tx)
	row, _, err := s.lockCodex(ctx, tx, subject, token, false)
	if err != nil {
		return err
	}
	if row.state != codexexec.Authorized {
		return nil
	}
	if err = workerAudit(ctx, tx, "worker.codex.unknown", token.RunID, token); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE worker_codex_executions SET state='UNKNOWN' WHERE execution_id=$1`, token.ID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) GetCodex(ctx context.Context, runID string) (codexexec.Status, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	row, err := scanCodex(s.pool.QueryRow(ctx, `SELECT `+codexColumns+` FROM worker_codex_executions WHERE run_id=$1`, runID))
	if err != nil {
		return codexexec.Status{}, err
	}
	state := row.state
	if state == codexexec.Authorized {
		var expired bool
		if err = s.pool.QueryRow(ctx, `SELECT lease_until<=clock_timestamp() FROM worker_codex_executions WHERE run_id=$1`, runID).Scan(&expired); err != nil {
			return codexexec.Status{}, err
		}
		if expired {
			state = "EXPIRED_UNRECONCILED"
		}
	}
	return codexexec.Status{Token: row.token, State: state, LeaseUntil: row.until, Receipt: row.receipt}, nil
}
