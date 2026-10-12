package postgres

import (
	"context"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jiying2007/engineering-platform/internal/action"
	"github.com/jiying2007/engineering-platform/internal/canonical"
)

// Genuine pg_dump/pg_restore of a newly allocated isolated schema into another
// newly allocated database. The retained Publisher witness is synthetic; this
// proves restore consistency/authority, not a live remote operation or RPO/SLO.
func TestPublicationRecoveryRestoreDrill(t *testing.T) {
	if os.Getenv("EP_RESTORE_DRILL") != "1" {
		t.Skip("EP_RESTORE_DRILL is not set")
	}
	s, req, e := publicationDatabaseFixture(t, action.Manual)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	saved, err := s.ConfirmRecoveredPublication(ctx, plannedReconciler, req, e)
	if err != nil {
		t.Fatal(err)
	}
	proof, err := s.CreateRecoveryProof(ctx, 1, "urn:engineering-platform:operator:proof")
	if err != nil {
		t.Fatal(err)
	}
	savedDigest, _ := canonical.Digest(saved)
	proofDigest, _ := canonical.Digest(proof)
	var sequence uint64
	var head string
	if err = s.pool.QueryRow(ctx, "SELECT last_sequence,last_digest FROM audit_journal_state WHERE singleton_id=true").Scan(&sequence, &head); err != nil {
		t.Fatal(err)
	}
	var major int
	if err = s.pool.QueryRow(ctx, "SELECT current_setting('server_version_num')::int/10000").Scan(&major); err != nil {
		t.Fatal(err)
	}
	schema := s.pool.Config().ConnConfig.RuntimeParams["search_path"]
	if !strings.HasPrefix(schema, "ep_test_") || len(schema) != len("ep_test_")+32 {
		t.Fatal("not an isolated schema")
	}
	base := os.Getenv("POSTGRES_TEST_URL")
	adminCfg := s.pool.Config()
	adminCfg.ConnConfig.Database = "postgres"
	delete(adminCfg.ConnConfig.RuntimeParams, "search_path")
	admin, err := pgxpool.NewWithConfig(ctx, adminCfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	targetName := restoreDatabaseName(t, "publication")
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
	dumpArgs := []string{"pg_dump", "--format=custom", "--no-owner", "--no-privileges", "--schema=" + schema, "--file=/dump/publication.dump", s.pool.Config().ConnConfig.Database}
	publicationPGTool(t, ctx, base, major, dumpArgs, dir)
	data, err := os.ReadFile(filepath.Join(dir, "publication.dump"))
	if err != nil || len(data) == 0 {
		t.Fatal("dump missing", err)
	}
	t.Logf("PostgreSQL major=%d dump_bytes=%d digest=%s", major, len(data), canonical.BytesDigest(data))
	publicationPGTool(t, ctx, base, major, []string{"pg_restore", "--no-owner", "--no-privileges", "--exit-on-error", "--dbname=" + targetName, "/dump/publication.dump"}, dir)
	// Remove only the random original schema's tables. All subsequent authority
	// operations must come from the new restored database, never the live source.
	resetIsolatedSchema(t, s)
	targetCfg := s.pool.Config()
	targetCfg.ConnConfig.Database = targetName
	targetPool, err := pgxpool.NewWithConfig(ctx, targetCfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(targetPool.Close)
	target := New(targetPool)
	got, err := target.GetRecoveredPublication(ctx, req.Identity.OperationID)
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
	restoredDigest, _ := canonical.Digest(restoredProof)
	if restoredDigest != proofDigest {
		t.Fatal("restored proof drift")
	}
	var gotSeq uint64
	var gotHead string
	if err = target.pool.QueryRow(ctx, "SELECT last_sequence,last_digest FROM audit_journal_state WHERE singleton_id=true").Scan(&gotSeq, &gotHead); err != nil || gotSeq != sequence || gotHead != head {
		t.Fatal("restored audit changed", err)
	}
	if target.AuthorizeCompletion(ctx, plannedReconciler, 1) == nil {
		t.Fatal("restored reconciler completed own recovery")
	}
	if err = target.AuthorizeCompletion(ctx, "urn:engineering-platform:operator:completer", 1); err != nil {
		t.Fatal(err)
	}
	if _, err = target.CompleteRecoveryContext(ctx, 1, true); err != nil {
		t.Fatal(err)
	}
	original, err := target.GetByIdempotencyKeyContext(ctx, req.Identity.IdempotencyKey)
	if err != nil || original.State != action.Confirmed || original.RecoveryEpoch != req.Identity.RecoveryEpoch {
		t.Fatal("restored reservation drift", err)
	}
	if _, err = target.GetRecoveredPublication(ctx, req.Identity.OperationID); err != nil {
		t.Fatal("completed restore lost readback", err)
	}
}

// Canonical CI keeps its existing Docker PostgreSQL 17 tooling. A local test
// may explicitly opt into same-major installed native tools; that never makes
// PostgreSQL 18 evidence into PostgreSQL 17 or canonical CI qualification.
func publicationPGTool(t *testing.T, ctx context.Context, raw string, major int, args []string, mount string) {
	t.Helper()
	if os.Getenv("EP_PUBLICATION_PG_NATIVE") != "1" {
		if major != 17 {
			t.Fatal("canonical restore requires PostgreSQL 17")
		}
		runPG17(t, raw, args, mount)
		return
	}
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	host := u.Hostname()
	ip := net.ParseIP(host)
	if host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		t.Fatal("native fixture restore restricted to loopback")
	}
	port := u.Port()
	if port == "" {
		port = "5432"
	}
	tool, err := exec.LookPath(args[0])
	if err != nil {
		t.Fatal(err)
	}
	version, err := exec.CommandContext(ctx, tool, "--version").CombinedOutput()
	if err != nil || !strings.Contains(string(version), "(PostgreSQL) "+strconv.Itoa(major)+".") {
		t.Fatalf("client/server major mismatch: %s %v", version, err)
	}
	command := []string{"-h", host, "-p", port, "-U", u.User.Username()}
	for _, a := range args[1:] {
		command = append(command, strings.ReplaceAll(a, "/dump/", mount+"/"))
	}
	cmd := exec.CommandContext(ctx, tool, command...)
	password, _ := u.User.Password()
	cmd.Env = append(os.Environ(), "PGPASSWORD="+password)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("scoped %s failed: %v\n%s", args[0], err, output)
	}
}
