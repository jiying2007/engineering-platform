package dbschema

import _ "embed"

// CurrentSchemaVersion is the latest migration understood by this binary.
// Migration 0001 predates the ledger; recorded versions start at 0002.
const CurrentSchemaVersion = 12

//go:embed migrations/0001_core.sql
var coreMigration string

//go:embed migrations/0002_outbox_authority.sql
var outboxAuthorityMigration string

//go:embed migrations/0003_worker_inbox.sql
var workerInboxMigration string

//go:embed migrations/0004_worker_preparation.sql
var workerPreparationMigration string

//go:embed migrations/0005_offline_execution.sql
var offlineExecutionMigration string

//go:embed migrations/0006_review_reports.sql
var reviewReportsMigration string

//go:embed migrations/0007_recovery_reconciliation.sql
var recoveryReconciliationMigration string

//go:embed migrations/0008_codex_execution.sql
var codexExecutionMigration string

//go:embed migrations/0009_codex_controls.sql
var codexControlsMigration string

//go:embed migrations/0010_codex_source_checkpoints.sql
var codexSourceCheckpointsMigration string

//go:embed migrations/0011_codex_continuations.sql
var codexContinuationsMigration string

//go:embed migrations/0012_execution_reconciliation.sql
var executionReconciliationMigration string

func CoreMigration() string {
	return coreMigration + "\n" + outboxAuthorityMigration + "\n" + workerInboxMigration + "\n" + workerPreparationMigration + "\n" + offlineExecutionMigration + "\n" + reviewReportsMigration + "\n" + recoveryReconciliationMigration + "\n" + codexExecutionMigration + "\n" + codexControlsMigration + "\n" + codexSourceCheckpointsMigration + "\n" + codexContinuationsMigration + "\n" + executionReconciliationMigration
}
