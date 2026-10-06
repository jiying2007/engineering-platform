//go:build linux

package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	dbschema "github.com/jiying2007/engineering-platform/db"
	"github.com/jiying2007/engineering-platform/internal/access"
	"github.com/jiying2007/engineering-platform/internal/githubpublish"
	pgstore "github.com/jiying2007/engineering-platform/internal/store/postgres"
	"github.com/jiying2007/engineering-platform/internal/testsupport"
)

// The existing native CI selector runs this against INSTALLED Control, not an
// in-process handler. Only the test administrator changes its random schema.
// The remote Publisher configuration is inert: no publication or model is run.
func TestOfflineCommandInstalledCoreSchemaCompatibility(t *testing.T) {
	dsn := os.Getenv("POSTGRES_TEST_URL")
	if dsn == "" {
		if os.Getenv("EP_SANDBOX_INTEGRATION") == "1" {
			t.Fatal("required installed-schema gate needs PostgreSQL")
		}
		t.Skip("POSTGRES_TEST_URL unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	bin := serviceTestBinaries(t, ctx)
	admin, err := pgxpool.New(ctx, dsn)
	commandOK(t, err)
	defer admin.Close()
	var nonce [16]byte
	_, err = rand.Read(nonce[:])
	commandOK(t, err)
	schema := "ep_schema_start_" + hex.EncodeToString(nonce[:])
	quoted := pgx.Identifier{schema}.Sanitize()
	_, err = admin.Exec(ctx, "CREATE SCHEMA "+quoted)
	commandOK(t, err)
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		_, err := admin.Exec(cleanup, "DROP SCHEMA "+quoted+" CASCADE")
		if err != nil {
			t.Errorf("remove only schema-startup fixture: %v", err)
		}
	}()
	u, err := url.Parse(dsn)
	commandOK(t, err)
	if u.Scheme != "postgres" && u.Scheme != "postgresql" {
		t.Fatal("explicit PostgreSQL test URL required")
	}
	query := u.Query()
	query.Set("search_path", schema)
	u.RawQuery = query.Encode()
	pool, err := pgxpool.New(ctx, u.String())
	commandOK(t, err)
	store := pgstore.New(pool)
	defer store.Close()
	exec := func(t *testing.T, sql string) {
		t.Helper()
		_, err := pool.Exec(ctx, sql, pgx.QueryExecModeSimpleProtocol)
		commandOK(t, err)
	}
	count := func(t *testing.T, sql string) int {
		t.Helper()
		var n int
		commandOK(t, pool.QueryRow(ctx, sql).Scan(&n))
		return n
	}
	config := t.TempDir()
	commandOK(t, os.Chmod(config, 0700))
	pki := testsupport.NewPKI(t)
	ca := serviceWrite(t, config, "ca.crt", pki.CAPEM)
	cert := serviceWrite(t, config, "server.crt", pki.ServerCertPEM)
	key := serviceWrite(t, config, "server.key", pki.ServerKeyPEM)
	const subject = "urn:engineering-platform:control:schema-startup-test"
	clientCert, clientKey := serviceClientFiles(t, config, "client", pki.ClientCertificate(t, subject))
	artifacts := filepath.Join(config, "artifacts")
	commandOK(t, os.Mkdir(artifacts, 0700))
	policy := serviceJSON(t, config, "access.json", access.Document{Version: 1, Principals: []access.PrincipalSpec{
		{Subject: subject, Scope: "platform", Capabilities: []string{access.Read}},
	}})
	plan := serviceJSON(t, config, "plan.json", githubpublish.PlanConfiguration{Version: 1, ArtifactRoot: artifacts, Targets: []githubpublish.TargetPolicy{
		{Repository: "example/schema-fixture", BaseRef: "main", BranchPrefix: "test/"},
	}})
	remote := serviceJSON(t, config, "remote.json", githubpublish.RemoteConfiguration{Version: 1, Endpoint: "https://127.0.0.1:1", ClientCertFile: clientCert, ClientKeyFile: clientKey, ServerCAFile: ca})
	env := []string{"DATABASE_URL=" + u.String(), "AUTO_MIGRATE=0", "CONTROL_TLS_CERT_FILE=" + cert, "CONTROL_TLS_KEY_FILE=" + key, "CONTROL_CLIENT_CA_FILE=" + ca, "CONTROL_AUTH_POLICY_FILE=" + policy, "GITHUB_PUBLISHER_PLAN_FILE=" + plan, "GITHUB_PUBLISHER_REMOTE_FILE=" + remote, "LISTEN_HOST=127.0.0.1", "PORT=0"}
	binary, args := filepath.Join(bin, "control-plane"), []string{"--production"}
	// A short, private Unix-datagram path avoids host TMPDIR path-length limits.
	notifyRoot, err := os.MkdirTemp("/tmp", "ep-start-notify-")
	commandOK(t, err)
	defer os.RemoveAll(notifyRoot)
	notifyPath := filepath.Join(notifyRoot, "socket")
	notified, err := net.ListenUnixgram("unixgram", &net.UnixAddr{Name: notifyPath, Net: "unixgram"})
	commandOK(t, err)
	defer notified.Close()
	env = append(env, "NOTIFY_SOCKET="+notifyPath)
	notification := func(t *testing.T, expected bool) {
		t.Helper()
		delay := 20 * time.Millisecond
		if expected {
			delay = time.Second
		}
		commandOK(t, notified.SetReadDeadline(time.Now().Add(delay)))
		raw := make([]byte, 128)
		n, _, readErr := notified.ReadFromUnix(raw)
		if expected {
			commandOK(t, readErr)
			if string(raw[:n]) != "READY=1" {
				t.Fatal("incorrect endpoint notification")
			}
		} else if timeout, ok := readErr.(net.Error); !ok || !timeout.Timeout() {
			t.Fatal("rejected/repeated startup emitted notification", n, readErr)
		}
	}
	reject := func(t *testing.T) {
		t.Helper()
		rejectInstalledProcess(t, ctx, binary, args, env)
		notification(t, false)
	}
	reject(t)
	if count(t, "SELECT count(*) FROM pg_catalog.pg_tables WHERE schemaname='"+schema+"'") != 0 {
		t.Fatal("blank database changed during rejected startup")
	}
	// Apply only the exact first three embedded migrations as a legacy fixture.
	// The old pre-listen predicate (version=3 exists) is deliberately satisfied.
	sql := dbschema.CoreMigration()
	marker := strings.Index(sql, "INSERT INTO core_schema_migrations(version) VALUES(3)")
	if marker < 0 {
		t.Fatal("legacy fixture boundary missing")
	}
	end := strings.Index(sql[marker:], "COMMIT;")
	if end < 0 {
		t.Fatal("legacy fixture transaction boundary missing")
	}
	exec(t, sql[:marker+end+len("COMMIT;")])
	if count(t, "SELECT count(*) FROM core_schema_migrations WHERE version=3") != 1 {
		t.Fatal("legacy fixture does not reach old startup predicate")
	}
	reject(t)
	if count(t, "SELECT count(*) FROM core_schema_migrations") != 2 {
		t.Fatal("service silently migrated legacy schema")
	}
	commandOK(t, store.ApplyCoreMigration(ctx)) // explicit isolated test-admin action
	for _, fault := range []struct{ name, apply, undo string }{
		{"missing-final-version", "DELETE FROM core_schema_migrations WHERE version=11", "INSERT INTO core_schema_migrations(version) VALUES(11)"},
		{"future-version", "INSERT INTO core_schema_migrations(version) VALUES(999)", "DELETE FROM core_schema_migrations WHERE version=999"},
		{"missing-runtime-table", "ALTER TABLE worker_codex_runtime RENAME TO hidden_runtime", "ALTER TABLE hidden_runtime RENAME TO worker_codex_runtime"},
		{"missing-current-column", "ALTER TABLE steering_commands RENAME COLUMN control_payload TO hidden_payload", "ALTER TABLE steering_commands RENAME COLUMN hidden_payload TO control_payload"},
	} {
		t.Run(fault.name, func(t *testing.T) { exec(t, fault.apply); reject(t); exec(t, fault.undo) })
	}
	commandOK(t, store.CheckWorkerSchema(ctx))
	control := startInstalledProcess(t, ctx, binary, args, env)
	defer func() { // reap before database cleanup, including failed assertions
		select {
		case <-control.done:
			return
		default:
			_ = control.cmd.Process.Kill()
		}
		select {
		case <-control.done:
		case <-time.After(5 * time.Second):
			t.Error("schema-test Control did not reap")
		}
	}()
	endpoint := control.endpoint(t, ctx)
	client := serviceClient(t, pki, endpoint, subject)
	var health map[string]string
	commandOK(t, client.Call(ctx, http.MethodGet, "/healthz", nil, &health))
	if health["status"] != "ok" {
		t.Fatal("compatible installed Control did not serve authenticated health")
	}
	notification(t, true)
	notification(t, false)
	control.stop(t, syscall.SIGTERM)
	assertServiceClosed(t, endpoint)
	if count(t, "SELECT count(*) FROM audit_events") != 0 || count(t, "SELECT count(*) FROM outbox_events") != 0 || count(t, "SELECT count(*) FROM worker_codex_executions") != 0 {
		t.Fatal("schema startup created execution or authority events")
	}
	t.Log("installed Control rejects blank/legacy/partial/future/structurally incomplete schemas before listening; explicit test migration permits authenticated startup; exact once-only main readiness after schema/TLS setup; no automatic migration/replay")
}
