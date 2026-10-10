package postgres

import (
	"context"
	"errors"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func deadlineConfig(t *testing.T) *pgxpool.Config {
	t.Helper()
	c, err := pgxpool.ParseConfig("postgres://reader:secret@127.0.0.1:5432/test?sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestProductionConnectionDeadlinesNeverSilentlyWiden(t *testing.T) {
	base := deadlineConfig(t)
	if err := applyConnectionDeadlines(base); err != nil {
		t.Fatal(err)
	}
	for _, d := range databaseDeadlines {
		ms := base.ConnConfig.RuntimeParams[d.name]
		got, err := pgParameterDuration(ms)
		if err != nil || got != d.limit {
			t.Fatalf("missing database safety ceiling %s: %s %v", d.name, ms, err)
		}
	}
	if err := applyConnectionDeadlines(base); err != nil {
		t.Fatalf("exact repeated setup changed bounds: %v", err)
	}

	for _, setting := range databaseDeadlines {
		t.Run(setting.name, func(t *testing.T) {
			stricter := deadlineConfig(t)
			stricter.ConnConfig.RuntimeParams[setting.name] = "100ms"
			if err := applyConnectionDeadlines(stricter); err != nil ||
				stricter.ConnConfig.RuntimeParams[setting.name] != "100ms" {
				t.Fatalf("explicit tighter bound widened: %v", err)
			}
			for _, bad := range []string{"0", "0ms", "-1", "bad", "2m", "3600000"} {
				looser := deadlineConfig(t)
				looser.ConnConfig.RuntimeParams[setting.name] = bad
				if applyConnectionDeadlines(looser) == nil {
					t.Fatalf("%s allowed unbounded/unsupported timeout %q", setting.name, bad)
				}
			}
		})
	}
	var nilConfig *pgxpool.Config
	if applyConnectionDeadlines(nilConfig) == nil || applyConnectionDeadlines(&pgxpool.Config{}) == nil {
		t.Fatal("unconfigured connection policy accepted")
	}
}

func TestProductionOpenActuallyUsesDatabaseSideLockDeadline(t *testing.T) {
	if os.Getenv("POSTGRES_TEST_URL") == "" {
		t.Skip("POSTGRES_TEST_URL required")
	}
	// A private random schema prevents this test from modifying another test's
	// database. The production Open path consumes its own parsed startup params.
	isolated := newIsolatedIntegrationStore(t)
	name := isolated.pool.Config().ConnConfig.RuntimeParams["search_path"]
	if !strings.HasPrefix(name, "ep_test_") {
		t.Fatal("not an isolated test schema")
	}
	u, err := url.Parse(os.Getenv("POSTGRES_TEST_URL"))
	if err != nil {
		t.Fatal(err)
	}
	query := u.Query()
	query.Set("search_path", name)
	query.Set("lock_timeout", "200ms") // explicit stricter bound must survive Open
	u.RawQuery = query.Encode()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	bounded, err := Open(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer bounded.Close()
	for _, d := range databaseDeadlines {
		var raw string
		if err := bounded.pool.QueryRow(ctx, "SELECT current_setting($1)", d.name).Scan(&raw); err != nil {
			t.Fatal(err)
		}
		got, err := pgParameterDuration(raw)
		if err != nil || got <= 0 || got > d.limit {
			t.Fatalf("server ignored nonzero bounded database timeout %s=%q: %v", d.name, raw, err)
		}
	}
	tx, err := isolated.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rollbackOutbox(tx)
	if _, err := tx.Exec(ctx, "UPDATE platform_state SET updated_at=clock_timestamp() WHERE singleton_id=true"); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	// Deliberately uncancelled caller context: the PostgreSQL server, not the
	// HTTP or Go client, must bound this independent row-lock wait.
	_, err = bounded.pool.Exec(context.Background(),
		"UPDATE platform_state SET updated_at=clock_timestamp() WHERE singleton_id=true")
	elapsed := time.Since(start)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "55P03" || elapsed > 2*time.Second {
		t.Fatalf("PostgreSQL failed to enforce actual server-side lock deadline: err=%v duration=%s", err, elapsed)
	}
}
