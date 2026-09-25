package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/preparation"
	"github.com/jiying2007/engineering-platform/internal/workerqueue"
)

var _ preparation.Repository = (*Store)(nil)

// ReportPrepared retains a separately attributed Worker filesystem attestation
// and the deterministic input receipt in ONE live-lease/audit transaction. It
// does not complete a Run or claim the server read a remote filesystem.
func (s *Store) ReportPrepared(ctx context.Context, subject string, report preparation.Report) (preparation.Receipt, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.workerTransaction(ctx)
	if err != nil {
		return preparation.Receipt{}, err
	}
	defer rollbackOutbox(tx)
	r, epoch, mode, err := lockWorkerLease(ctx, tx, subject, report.Input.Token)
	if err != nil {
		return preparation.Receipt{}, err
	}
	a, err := loadWorkerAssignment(ctx, tx, r)
	if err != nil {
		return preparation.Receipt{}, err
	}
	expected, err := workerqueue.NewReport(a)
	if err != nil || expected != report.Input || report.Facts.Check(a) != nil {
		return preparation.Receipt{}, workerqueue.ErrIdentity
	}
	digest, err := canonical.Digest(report.Facts)
	if err != nil {
		return preparation.Receipt{}, err
	}
	if r.state == workerqueue.Validated {
		// Exactly identical retry is historical readback, including during recovery.
		var raw []byte
		err := tx.QueryRow(ctx, `SELECT receipt_json FROM worker_preparations WHERE inbox_id=$1`, r.id).Scan(&raw)
		if errors.Is(err, pgx.ErrNoRows) {
			return preparation.Receipt{}, workerqueue.ErrLease
		}
		if err != nil {
			return preparation.Receipt{}, err
		}
		var receipt preparation.Receipt
		if json.Unmarshal(raw, &receipt) != nil || preparation.Verify(a, receipt, report.Facts, subject) != nil {
			return preparation.Receipt{}, workerqueue.ErrIdentity
		}
		return receipt, nil
	}
	if err := liveWorkerLease(ctx, tx, r, epoch, mode); err != nil {
		return preparation.Receipt{}, err
	}
	validation, err := workerqueue.Validate(a)
	if err != nil {
		return preparation.Receipt{}, err
	}
	var now time.Time
	if err := tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return preparation.Receipt{}, err
	}
	receipt := preparation.Receipt{Kind: preparation.Kind, Admission: workerqueue.Receipt{Token: a.Token, Worker: subject, Kind: workerqueue.Validated, Validation: validation, ReceivedAt: now}, Facts: report.Facts, FactsDigest: digest, ReceivedAt: now}
	raw, err := json.Marshal(receipt)
	if err != nil {
		return preparation.Receipt{}, err
	}
	input, err := json.Marshal(receipt.Admission)
	if err != nil {
		return preparation.Receipt{}, err
	}
	if err := workerAudit(ctx, tx, "worker.preparation.recorded", r.runID, receipt); err != nil {
		return preparation.Receipt{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO worker_preparations(inbox_id,run_id,lease_generation,worker_subject,facts_digest,receipt_json)
 VALUES($1,$2,$3,$4,$5,$6::jsonb)`, r.id, r.runID, r.generation, subject, digest, string(raw)); err != nil {
		return preparation.Receipt{}, err
	}
	// Journal/insert waits must not revive an expired lease. All three writes roll
	// back on this last check, including their logical audit sequence.
	tag, err := tx.Exec(ctx, `UPDATE worker_inbox SET state='INPUT_VALIDATED',receipt_json=$2::jsonb,lease_until=NULL,updated_at=clock_timestamp()
 WHERE inbox_id=$1 AND state='LEASED' AND lease_until>clock_timestamp()`, r.id, string(input))
	if err != nil {
		return preparation.Receipt{}, err
	}
	if tag.RowsAffected() != 1 {
		return preparation.Receipt{}, workerqueue.ErrLease
	}
	if err := tx.Commit(ctx); err != nil {
		return preparation.Receipt{}, err
	}
	return receipt, nil
}
func (s *Store) GetPreparation(ctx context.Context, runID string) (preparation.Receipt, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var raw []byte
	if err := s.pool.QueryRow(ctx, `SELECT receipt_json FROM worker_preparations WHERE run_id=$1`, runID).Scan(&raw); err != nil {
		return preparation.Receipt{}, mapReadError(err)
	}
	var receipt preparation.Receipt
	if err := json.Unmarshal(raw, &receipt); err != nil {
		return receipt, err
	}
	digest, err := canonical.Digest(receipt.Facts)
	if err != nil || receipt.Kind != preparation.Kind || digest != receipt.FactsDigest {
		return preparation.Receipt{}, workerqueue.ErrIdentity
	}
	return receipt, nil
}

// Check before claiming so an unapplied additive migration cannot consume work.
func (s *Store) PreparationReady(ctx context.Context) error {
	if s == nil || s.pool == nil {
		return workerqueue.ErrIdentity
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var ready bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM core_schema_migrations WHERE version=4) AND to_regclass('worker_preparations') IS NOT NULL`).Scan(&ready)
	if err != nil {
		return err
	}
	if !ready {
		return workerqueue.ErrIdentity
	}
	return nil
}
