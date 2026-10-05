package postgres

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Every fault is confined to this helper's random isolated schema. These are
// test-admin DDL operations, never a service-startup migration/repair policy.
func TestCoreSchemaStartupCompatibility(t *testing.T) {
	s := newIsolatedIntegrationStore(t)
	ctx := context.Background()
	workerOK(t, s.CheckWorkerSchema(ctx))
	for _, version := range currentMigrationVersions() {
		t.Run(fmt.Sprintf("missing-%02d", version), func(t *testing.T) {
			workerSQL(t, s, "DELETE FROM core_schema_migrations WHERE version=$1", version)
			if err := s.CheckWorkerSchema(ctx); err == nil {
				t.Fatal("missing migration accepted")
			}
			assertCount(t, s, fmt.Sprintf("SELECT count(*) FROM core_schema_migrations WHERE version=%d", version), 0)
			workerSQL(t, s, "INSERT INTO core_schema_migrations(version) VALUES($1)", version)
		})
	}
	t.Run("future-binary-downgrade", func(t *testing.T) {
		workerSQL(t, s, "INSERT INTO core_schema_migrations(version) VALUES(999)")
		if err := s.CheckWorkerSchema(ctx); err == nil {
			t.Fatal("future schema accepted by old binary")
		}
		assertCount(t, s, "SELECT count(*) FROM core_schema_migrations WHERE version=999", 1)
		workerSQL(t, s, "DELETE FROM core_schema_migrations WHERE version=999")
	})
	for _, name := range startupSchemaTables {
		t.Run("missing-table-"+name, func(t *testing.T) {
			// Renaming, rather than CASCADE-dropping, preserves the original data,
			// constraints and migration facts for independent absence/readback.
			workerSQL(t, s, "ALTER TABLE "+pgx.Identifier{name}.Sanitize()+" RENAME TO startup_test_hidden")
			if err := s.CheckWorkerSchema(ctx); err == nil {
				t.Fatal("missing required table accepted")
			}
			workerSQL(t, s, "ALTER TABLE startup_test_hidden RENAME TO "+pgx.Identifier{name}.Sanitize())
		})
	}
	for table, columns := range startupSchemaColumns {
		for _, column := range columns {
			t.Run("missing-column-"+table+"-"+column, func(t *testing.T) {
				qualified := pgx.Identifier{table}.Sanitize()
				workerSQL(t, s, "ALTER TABLE "+qualified+" RENAME COLUMN "+pgx.Identifier{column}.Sanitize()+" TO startup_test_hidden")
				if err := s.CheckWorkerSchema(ctx); err == nil {
					t.Fatal("missing current column accepted")
				}
				workerSQL(t, s, "ALTER TABLE "+qualified+" RENAME COLUMN startup_test_hidden TO "+pgx.Identifier{column}.Sanitize())
			})
		}
	}
	t.Run("view-is-not-current-table", func(t *testing.T) {
		workerSQL(t, s, "ALTER TABLE codex_continuations RENAME TO startup_test_hidden")
		workerSQL(t, s, "CREATE VIEW codex_continuations AS SELECT * FROM startup_test_hidden")
		if err := s.CheckWorkerSchema(ctx); err == nil {
			t.Fatal("view accepted as migrated Core table")
		}
		workerSQL(t, s, "DROP VIEW codex_continuations")
		workerSQL(t, s, "ALTER TABLE startup_test_hidden RENAME TO codex_continuations")
	})
	workerOK(t, s.CheckWorkerSchema(ctx))
	assertCount(t, s, "SELECT count(*) FROM audit_events", 0)
	assertCount(t, s, "SELECT count(*) FROM outbox_events", 0)
	assertCount(t, s, "SELECT count(*) FROM worker_codex_executions", 0)
	assertCount(t, s, "SELECT count(*) FROM platform_state WHERE recovery_epoch=0 AND recovery_mode='NORMAL'", 1)
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err := s.CheckWorkerSchema(cancelled); err == nil {
		t.Fatal("cancelled schema check accepted")
	}
}

func TestCoreSchemaRejectsSearchPathFallbackAndTemporaryShadow(t *testing.T) {
	full := newIsolatedIntegrationStore(t)
	empty := newIsolatedIntegrationStore(t)
	resetIsolatedSchema(t, empty)
	ctx := context.Background()
	config := full.pool.Config().Copy()
	config.MaxConns = 1
	fullSchema := config.ConnConfig.RuntimeParams["search_path"]
	emptySchema := empty.pool.Config().ConnConfig.RuntimeParams["search_path"]
	config.ConnConfig.RuntimeParams["search_path"] = emptySchema + "," + fullSchema
	pool, err := pgxpool.NewWithConfig(ctx, config)
	workerOK(t, err)
	defer pool.Close()
	fallback := New(pool)
	if fallback.CheckWorkerSchema(ctx) == nil {
		t.Fatal("fallback to another schema satisfied empty current schema")
	}
	pool.Close()
	config.ConnConfig.RuntimeParams["search_path"] = fullSchema
	pool, err = pgxpool.NewWithConfig(ctx, config)
	workerOK(t, err)
	defer pool.Close()
	shadow := New(pool)
	workerOK(t, shadow.CheckWorkerSchema(ctx))
	workerSQL(t, shadow, "CREATE TEMP TABLE codex_continuations(dummy text)")
	if shadow.CheckWorkerSchema(ctx) == nil {
		t.Fatal("temporary table shadow accepted as current permanent relation")
	}
	workerSQL(t, shadow, "DROP TABLE pg_temp.codex_continuations")
	workerOK(t, shadow.CheckWorkerSchema(ctx))
}

func TestCoreSchemaCheckHonorsLockDeadline(t *testing.T) {
	s := newIsolatedIntegrationStore(t)
	ctx := context.Background()
	blocker, err := s.pool.Begin(ctx)
	workerOK(t, err)
	defer rollbackOutbox(blocker)
	_, err = blocker.Exec(ctx, "LOCK TABLE core_schema_migrations IN ACCESS EXCLUSIVE MODE")
	workerOK(t, err)
	check, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	if err := s.CheckWorkerSchema(check); err == nil || check.Err() == nil {
		t.Fatalf("blocked compatibility check did not honor its caller deadline: %v", err)
	}
	workerOK(t, blocker.Rollback(ctx))
	workerOK(t, s.CheckWorkerSchema(ctx))
}
