package dbschema

import _ "embed"

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

func CoreMigration() string {
	return coreMigration + "\n" + outboxAuthorityMigration + "\n" + workerInboxMigration + "\n" + workerPreparationMigration + "\n" + offlineExecutionMigration + "\n" + reviewReportsMigration + "\n" + recoveryReconciliationMigration
}
