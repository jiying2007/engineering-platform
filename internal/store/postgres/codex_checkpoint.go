package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jiying2007/engineering-platform/internal/codexexec"
	"github.com/jiying2007/engineering-platform/internal/preparation"
	corestore "github.com/jiying2007/engineering-platform/internal/store"
	"github.com/jiying2007/engineering-platform/internal/workerqueue"
)

var _ codexexec.CheckpointRepository = (*Store)(nil)

func checkpointRow(row pgx.Row) (*codexexec.SourceCheckpoint, error) {
	var raw []byte
	var digest string
	if err := row.Scan(&raw, &digest); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	var c codexexec.SourceCheckpoint
	if json.Unmarshal(raw, &c) != nil {
		return nil, workerqueue.ErrIdentity
	}
	actual, err := c.Digest()
	if err != nil || actual != digest {
		return nil, workerqueue.ErrIdentity
	}
	return &c, nil
}
func (s *Store) getCodexCheckpoint(ctx context.Context, execution string) (*codexexec.SourceCheckpoint, error) {
	return checkpointRow(s.pool.QueryRow(ctx, `SELECT checkpoint_json,checkpoint_digest FROM worker_codex_source_checkpoints WHERE execution_id=$1`, execution))
}
func (s *Store) SaveCodexCheckpoint(ctx context.Context, subject string, c codexexec.SourceCheckpoint) (codexexec.SourceCheckpoint, error) {
	empty := codexexec.SourceCheckpoint{}
	digest, err := c.Digest()
	if err != nil {
		return empty, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.workerTransaction(ctx)
	if err != nil {
		return empty, err
	}
	defer rollbackOutbox(tx)
	// Recording a stopped artifact may follow revocation; it grants no execution.
	row, inbox, err := s.lockCodex(ctx, tx, subject, c.Binding.Token, false)
	if err != nil {
		return empty, err
	}
	runtime, err := lockControlRuntime(ctx, tx, c.Binding)
	if err != nil {
		return empty, err
	}
	if runtime.State != "SEALED" || runtime.Transcript == nil || !runtime.Transcript.Close.ProcessScope.Quiescent() || runtime.Digest != c.TranscriptDigest || c.Binding.ExecutionEpoch != inbox.intent.ExecutionEpoch || c.TaskDigest != inbox.intent.TaskDigest || c.InputDigest != inbox.intent.InputDigest {
		return empty, workerqueue.ErrIdentity
	}
	assignment, err := loadWorkerAssignment(ctx, tx, inbox)
	if err != nil {
		return empty, err
	}
	var prepRaw []byte
	var prep preparation.Receipt
	if err := tx.QueryRow(ctx, `SELECT receipt_json FROM worker_preparations WHERE inbox_id=$1`, inbox.id).Scan(&prepRaw); err != nil {
		return empty, mapReadError(err)
	}
	if json.Unmarshal(prepRaw, &prep) != nil || preparation.Verify(assignment, prep, prep.Facts, subject) != nil || c.BaseCommit != prep.Facts.BaseCommit || c.BaselineDigest != prep.Facts.SourceDigest || row.preparation != prep.FactsDigest {
		return empty, workerqueue.ErrIdentity
	}
	old, err := checkpointRow(tx.QueryRow(ctx, `SELECT checkpoint_json,checkpoint_digest FROM worker_codex_source_checkpoints WHERE execution_id=$1`, c.Binding.Token.ID))
	if err != nil {
		return empty, err
	}
	if old != nil {
		if *old != c {
			return empty, corestore.ErrConflict
		}
		return *old, nil
	}
	raw, _ := json.Marshal(c)
	if _, err := tx.Exec(ctx, `INSERT INTO worker_codex_source_checkpoints(execution_id,checkpoint_json,checkpoint_digest) VALUES($1,$2::jsonb,$3)`, c.Binding.Token.ID, string(raw), digest); err != nil {
		return empty, mapWriteError(err)
	}
	if err := workerAudit(ctx, tx, "worker.codex.source-checkpoint-retained", c.Binding.Token.RunID, c); err != nil {
		return empty, err
	}
	if err := tx.Commit(ctx); err != nil {
		return empty, err
	}
	return c, nil
}
