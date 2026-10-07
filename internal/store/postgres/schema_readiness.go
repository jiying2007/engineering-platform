package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	dbschema "github.com/jiying2007/engineering-platform/db"
)

// Required permanent tables, not optional extensions. The manifest regression
// checks this set against every CREATE TABLE in the unchanged embedded migrations.
// The probes below cover columns added to an existing table by later migrations.
var startupSchemaTables = []string{
	"artifacts", "audit_events", "audit_journal_state", "checkpoints",
	"closure_receipts", "codex_continuations", "core_schema_migrations",
	"delivery_receipts", "evidence", "external_operations", "outbox_events",
	"platform_state", "recovery_reconciliation_proofs", "review_reports",
	"run_attempts", "run_input_manifests", "runs", "sessions", "steering_commands",
	"task_contracts", "verification_plans", "verification_reports", "work_items",
	"worker_codex_executions", "worker_codex_runtime", "worker_codex_source_checkpoints",
	"worker_inbox", "worker_offline_executions", "worker_preparations", "workers",
}

var startupSchemaColumns = map[string][]string{
	"outbox_events":             {"lease_recovery_epoch", "risk_class"},
	"closure_receipts":          {"review_report_id"},
	"steering_commands":         {"execution_id", "control_payload", "dispatched_at", "resolved_at"},
	"worker_offline_executions": {"reconciliation_json"},
	"worker_codex_executions":   {"reconciliation_json"},
}

// CheckWorkerSchema is the existing pre-listen startup gate. It now requires the
// complete current Core ledger and required relations, not just migration 0003.
// It is a bounded, read-only compatibility check: no migration, repair, replay,
// advisory-lock ownership, or production-readiness claim follows from success.
// Operators must still quiesce all services before schema changes. This snapshot
// is not a fence against privileged DDL after the check or proof of every index,
// constraint, database permission, data invariant or deployment capability.
func (s *Store) CheckWorkerSchema(ctx context.Context) error {
	if s == nil || s.pool == nil {
		return fmt.Errorf("PostgreSQL store required")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return fmt.Errorf("begin read-only schema check: %w", err)
	}
	defer rollbackOutbox(tx)
	var schema string
	if err := tx.QueryRow(ctx, `SELECT pg_catalog.current_schema()`).Scan(&schema); err != nil || schema == "" {
		return fmt.Errorf("current Core schema is unavailable")
	}
	for _, name := range startupSchemaTables {
		var namespace, kind, persistence string
		// Resolve as the actual unqualified application SQL does: a temporary
		// shadow or search_path fallback must not satisfy another schema's ledger.
		err := tx.QueryRow(ctx, `SELECT n.nspname,c.relkind::text,c.relpersistence::text
FROM pg_catalog.pg_class c JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace
WHERE c.oid=pg_catalog.to_regclass($1)`, name).Scan(&namespace, &kind, &persistence)
		if err != nil || namespace != schema || kind != "r" || persistence != "p" {
			return fmt.Errorf("Core schema requires permanent table %s in the current schema", name)
		}
		for _, column := range startupSchemaColumns[name] {
			var present bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_catalog.pg_attribute
WHERE attrelid=pg_catalog.to_regclass($1) AND attname=$2 AND attnum>0 AND NOT attisdropped)`, name, column).Scan(&present); err != nil || !present {
				return fmt.Errorf("Core schema requires column %s.%s", name, column)
			}
		}
	}
	// One extra row detects an unsupported future version or duplicate ledger,
	// without reading an unbounded table. Initial migration 0001 was untracked.
	rows, err := tx.Query(ctx, `SELECT version FROM `+pgx.Identifier{schema, "core_schema_migrations"}.Sanitize()+` ORDER BY version LIMIT $1`, dbschema.CurrentSchemaVersion)
	if err != nil {
		return fmt.Errorf("read Core migration ledger: %w", err)
	}
	versions, err := pgx.CollectRows(rows, pgx.RowTo[int])
	if err != nil {
		return fmt.Errorf("decode Core migration ledger: %w", err)
	}
	if err := checkCoreMigrationVersions(versions); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("finish read-only schema check: %w", err)
	}
	return nil
}

func checkCoreMigrationVersions(versions []int) error {
	if len(versions) != dbschema.CurrentSchemaVersion-1 {
		return fmt.Errorf("Core requires exactly recorded migrations 0002..%04d; migrate or select a compatible binary explicitly", dbschema.CurrentSchemaVersion)
	}
	for i, version := range versions {
		if version != i+2 {
			return fmt.Errorf("Core migration ledger has a missing, duplicate or unsupported version; require 0002..%04d", dbschema.CurrentSchemaVersion)
		}
	}
	return nil
}
