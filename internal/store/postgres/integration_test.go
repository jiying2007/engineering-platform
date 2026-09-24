package postgres

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Destructive fault injection must never reset or corrupt public, nor leak its
// missing singleton/audit state into another test. Each Store gets a fresh schema
// with no public fallback in search_path, on every connection in its pool.
func newIsolatedIntegrationStore(t *testing.T) *Store {
	t.Helper()
	url := os.Getenv("POSTGRES_TEST_URL")
	if url == "" {
		t.Skip("POSTGRES_TEST_URL is not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	admin, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		t.Fatal(err)
	}
	schema := "ep_test_" + hex.EncodeToString(nonce[:])
	quoted := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.pool.Exec(ctx, "CREATE SCHEMA "+quoted); err != nil {
		t.Fatalf("create isolated integration schema: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := admin.pool.Exec(ctx, "DROP SCHEMA "+quoted+" CASCADE"); err != nil {
			t.Errorf("clean isolated integration schema: %v", err)
		}
	})
	config, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	s := New(pool)
	// LIFO: close the test pool, drop only its schema, then close admin pool.
	t.Cleanup(s.Close)
	if err := s.ApplyCoreMigration(ctx); err != nil {
		t.Fatal(err)
	}
	return s
}

// The legacy-upgrade test recreates only its allocated schema, not public.
func resetIsolatedSchema(t *testing.T, s *Store) {
	t.Helper()
	schema := s.pool.Config().ConnConfig.RuntimeParams["search_path"]
	suffix := strings.TrimPrefix(schema, "ep_test_")
	if schema == suffix || len(suffix) != 32 {
		t.Fatal("refuse to reset a non-isolated schema")
	}
	if _, err := hex.DecodeString(suffix); err != nil {
		t.Fatal("refuse invalid isolated schema identifier")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	quoted := pgx.Identifier{schema}.Sanitize()
	if _, err := s.pool.Exec(ctx, "DROP SCHEMA "+quoted+" CASCADE; CREATE SCHEMA "+quoted, pgx.QueryExecModeSimpleProtocol); err != nil {
		t.Fatal(err)
	}
	// Cached statements from the old schema must not survive the reset.
	s.pool.Reset()
}

func TestPostgresFaultInjectionSchemaIsolation(t *testing.T) {
	first := integrationStore(t)
	second := integrationStore(t)
	if first.pool.Config().ConnConfig.RuntimeParams["search_path"] == second.pool.Config().ConnConfig.RuntimeParams["search_path"] {
		t.Fatal("integration stores share a schema")
	}
	if _, err := first.pool.Exec(context.Background(), "DELETE FROM audit_journal_state; DELETE FROM platform_state", pgx.QueryExecModeSimpleProtocol); err != nil {
		t.Fatal(err)
	}
	if _, err := second.BeginRecovery(0); err != nil {
		t.Fatalf("another test's missing state contaminated recovery: %v", err)
	}
	assertCount(t, second, "SELECT count(*) FROM audit_events", 1)
	assertCount(t, first, "SELECT count(*) FROM audit_events", 0)
}
