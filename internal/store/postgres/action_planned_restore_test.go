package postgres

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/recovery"
)

// Real same-major pg_dump/pg_restore into a new database. The source schema is
// removed before checking the restored authority; no original live read can
// substitute for the restored row, receipt, audit or migration index.
func TestPlannedActionRestorePreservesBoundReceiptAndRejectsLaterCorruption(t *testing.T) {
	if os.Getenv("EP_RESTORE_DRILL") != "1" {
		t.Skip("EP_RESTORE_DRILL is not set")
	}
	s, op, req := actionPlannedRecoveryFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := s.CreateContext(ctx, *op); err != nil {
		t.Fatal(err)
	}
	if _, err := s.BeginRecoveryContext(ctx, 0); err != nil {
		t.Fatal(err)
	}
	saved, err := s.ReconcilePlannedActionAbandoned(ctx, plannedReconciler, req)
	if err != nil {
		t.Fatal(err)
	}
	proof, err := s.CreateRecoveryProof(ctx, 1, "independent-proof")
	if err != nil {
		t.Fatal(err)
	}
	savedDigest, _ := canonical.Digest(saved)
	proofDigest, _ := canonical.Digest(proof)
	var sequence uint64
	var head string
	var major int
	if err = s.pool.QueryRow(ctx, "SELECT last_sequence,last_digest FROM audit_journal_state WHERE singleton_id=true").Scan(&sequence, &head); err != nil {
		t.Fatal(err)
	}
	if err = s.pool.QueryRow(ctx, "SELECT current_setting('server_version_num')::int/10000").Scan(&major); err != nil {
		t.Fatal(err)
	}
	schema := s.pool.Config().ConnConfig.RuntimeParams["search_path"]
	if !strings.HasPrefix(schema, "ep_test_") || len(schema) != len("ep_test_")+32 {
		t.Fatal("not an isolated schema")
	}
	adminCfg := s.pool.Config()
	adminCfg.ConnConfig.Database = "postgres"
	delete(adminCfg.ConnConfig.RuntimeParams, "search_path")
	admin, err := pgxpool.NewWithConfig(ctx, adminCfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	targetName := restoreDatabaseName(t, "planned")
	if _, err = admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{targetName}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		if _, err := admin.Exec(cleanup, "DROP DATABASE IF EXISTS "+pgx.Identifier{targetName}.Sanitize()+" WITH (FORCE)"); err != nil {
			t.Error(err)
		}
	})
	dir := t.TempDir()
	if err = os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	base := os.Getenv("POSTGRES_TEST_URL")
	publicationPGTool(t, ctx, base, major, []string{"pg_dump", "--format=custom", "--no-owner", "--no-privileges", "--schema=" + schema, "--file=/dump/planned.dump", s.pool.Config().ConnConfig.Database}, dir)
	data, err := os.ReadFile(filepath.Join(dir, "planned.dump"))
	if err != nil || len(data) == 0 {
		t.Fatal("dump missing", err)
	}
	t.Logf("PostgreSQL major=%d dump_bytes=%d digest=%s", major, len(data), canonical.BytesDigest(data))
	publicationPGTool(t, ctx, base, major, []string{"pg_restore", "--no-owner", "--no-privileges", "--exit-on-error", "--dbname=" + targetName, "/dump/planned.dump"}, dir)
	resetIsolatedSchema(t, s)
	targetCfg := s.pool.Config()
	targetCfg.ConnConfig.Database = targetName
	targetPool, err := pgxpool.NewWithConfig(ctx, targetCfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(targetPool.Close)
	target := New(targetPool)
	if err = target.CheckWorkerSchema(ctx); err != nil {
		t.Fatal("restored schema version mismatch", err)
	}
	assertProvenanceIndex(t, target)
	got, err := target.GetPlannedActionAbandonReceipt(ctx, op.ID)
	if err != nil {
		t.Fatal(err)
	}
	gotDigest, _ := canonical.Digest(got)
	if gotDigest != savedDigest {
		t.Fatal("restored receipt drift")
	}
	restoredProof, err := target.GetRecoveryProof(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	gotProofDigest, _ := canonical.Digest(restoredProof)
	if gotProofDigest != proofDigest {
		t.Fatal("restored proof drift")
	}
	var gotSequence uint64
	var gotHead string
	if err = target.pool.QueryRow(ctx, "SELECT last_sequence,last_digest FROM audit_journal_state WHERE singleton_id=true").Scan(&gotSequence, &gotHead); err != nil || gotSequence != sequence || gotHead != head {
		t.Fatal("restored journal drift", err)
	}
	if err = target.AuthorizeCompletion(ctx, plannedReconciler, 1); err == nil {
		t.Fatal("restored reconciler completed own settlement")
	}
	if err = target.AuthorizeCompletion(ctx, "independent-completer", 1); err != nil {
		t.Fatal(err)
	}
	if _, err = target.CompleteRecoveryContext(ctx, 1, true); err != nil {
		t.Fatal(err)
	}
	// Old valid receipts remain valid in subsequent epochs; validation must bind
	// their original epochs rather than require the currently active one.
	if _, err = target.BeginRecoveryContext(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if _, err = target.CreateRecoveryProof(ctx, 2, "next-independent-proof"); err != nil {
		t.Fatal("old valid receipt rejected in later Recovery", err)
	}
	if err = target.AuthorizeCompletion(ctx, "independent-completer", 2); err != nil {
		t.Fatal(err)
	}
	got.Request.IdempotencyKey = "corrupted-after-restore"
	plannedFixtureReceipt(t, target, got)
	if _, err = target.GetPlannedActionAbandonReceipt(ctx, op.ID); !errors.Is(err, ErrRecoveryFactsUnresolved) {
		t.Fatal("restored row accepted substituted receipt", err)
	}
	if _, err = target.ReadOperationalStatus(ctx); !errors.Is(err, ErrRecoveryFactsUnresolved) {
		t.Fatal("restored status ignored historical corruption", err)
	}
	if err = target.AuthorizeCompletion(ctx, "independent-completer", 2); !errors.Is(err, ErrRecoveryFactsUnresolved) {
		t.Fatal("restored authorization ignored corruption", err)
	}
	if _, err = target.CompleteRecoveryContext(ctx, 2, true); !errors.Is(err, ErrRecoveryFactsUnresolved) {
		t.Fatal("restored proof bypassed later corruption", err)
	}
	state, err := target.GetRecoveryContext(ctx)
	if err != nil || state.Mode != recovery.RecoveryReconciliation || state.Epoch != 2 {
		t.Fatal("corruption released Recovery", state, err)
	}
}
