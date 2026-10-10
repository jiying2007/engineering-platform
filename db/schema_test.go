package dbschema

import (
	"strings"
	"testing"
)

func TestReviewMigrationFailsClosedInsteadOfBackfillingLegacyClosure(t *testing.T) {
	guard := "IF EXISTS(SELECT 1 FROM closure_receipts)"
	raise := "migration 6 requires operator reconciliation of legacy closure_receipts"
	notNull := "ALTER COLUMN review_report_id SET NOT NULL"
	fk := "FOREIGN KEY (review_report_id) REFERENCES review_reports(review_report_id)"
	for _, required := range []string{guard, raise, notNull, fk} {
		if !strings.Contains(reviewReportsMigration, required) {
			t.Fatalf("review migration missing %q", required)
		}
	}
	if strings.Index(reviewReportsMigration, guard) > strings.Index(reviewReportsMigration, notNull) {
		t.Fatal("legacy closure guard runs after destructive tightening")
	}
	if strings.Contains(strings.ToLower(reviewReportsMigration), "insert into review_reports") {
		t.Fatal("review migration must not synthesize historical review authority")
	}
	core := CoreMigration()
	if strings.LastIndex(core, "INSERT INTO core_schema_migrations(version) VALUES(5)") >
		strings.LastIndex(core, "INSERT INTO core_schema_migrations(version) VALUES(6)") {
		t.Fatal("review migration is not ordered after offline execution migration")
	}
}

func TestRecoveryReconciliationMigrationIsAdditiveAndOrdered(t *testing.T) {
	for _, required := range []string{
		"CREATE TABLE recovery_reconciliation_proofs",
		"recovery_epoch bigint PRIMARY KEY",
		"facts_digest text NOT NULL",
		"proof_json jsonb NOT NULL",
		"INSERT INTO core_schema_migrations(version) VALUES(7)",
	} {
		if !strings.Contains(recoveryReconciliationMigration, required) {
			t.Fatalf("recovery migration missing %q", required)
		}
	}
	lower := strings.ToLower(recoveryReconciliationMigration)
	if strings.Contains(lower, "insert into recovery_reconciliation_proofs") {
		t.Fatal("migration must not synthesize reconciliation proof")
	}
	core := CoreMigration()
	if strings.LastIndex(core, "INSERT INTO core_schema_migrations(version) VALUES(6)") >
		strings.LastIndex(core, "INSERT INTO core_schema_migrations(version) VALUES(7)") {
		t.Fatal("recovery proof migration is not ordered after review migration")
	}
}

func TestCodexExecutionMigrationIsAdditiveAndOrdered(t *testing.T) {
	for _, required := range []string{
		"CREATE TABLE worker_codex_executions",
		"run_id text NOT NULL UNIQUE REFERENCES runs(run_id)",
		"state text NOT NULL CHECK(state IN ('AUTHORIZED','FINISHED','UNKNOWN'))",
		"INSERT INTO core_schema_migrations(version) VALUES(8)",
	} {
		if !strings.Contains(codexExecutionMigration, required) {
			t.Fatalf("Codex execution migration missing %q", required)
		}
	}
	lower := strings.ToLower(codexExecutionMigration)
	if strings.Contains(lower, "insert into worker_codex_executions") {
		t.Fatal("migration must not synthesize Codex execution rows")
	}
	core := CoreMigration()
	if strings.LastIndex(core, "INSERT INTO core_schema_migrations(version) VALUES(7)") >
		strings.LastIndex(core, "INSERT INTO core_schema_migrations(version) VALUES(8)") {
		t.Fatal("Codex execution migration is not ordered after recovery proof migration")
	}
}

func TestExecutionReconciliationMigrationPreservesStoppedCodexState(t *testing.T) {
	for _, required := range []string{
		"ADD COLUMN reconciliation_json jsonb",
		"CHECK(state IN ('AUTHORIZED','FINISHED','UNKNOWN','ABANDONED_RECONCILED'))",
		"CHECK(state IN ('AUTHORIZED','FINISHED','UNKNOWN','STOPPED_NO_DELIVERY','ABANDONED_RECONCILED'))",
		"CHECK ((state='ABANDONED_RECONCILED') = (reconciliation_json IS NOT NULL))",
		"INSERT INTO core_schema_migrations(version) VALUES(12)",
	} {
		if !strings.Contains(executionReconciliationMigration, required) {
			t.Fatalf("execution reconciliation migration missing %q", required)
		}
	}
	core := CoreMigration()
	if strings.LastIndex(core, "INSERT INTO core_schema_migrations(version) VALUES(11)") >
		strings.LastIndex(core, "INSERT INTO core_schema_migrations(version) VALUES(12)") {
		t.Fatal("execution reconciliation migration is not ordered after continuation migration")
	}
	if strings.Count(executionReconciliationMigration, "ABANDONED_RECONCILED") < 4 {
		t.Fatal("offline/Codex abandonment constraints are incomplete")
	}
}

func TestActionPlannedReconciliationMigrationIsNoReplayAndOrdered(t *testing.T) {
	for _, required := range []string{
		"ALTER TABLE external_operations ADD COLUMN reconciliation_json jsonb",
		"CHECK (",
		"'ABANDONED_RECONCILED'",
		"(state='ABANDONED_RECONCILED') = (reconciliation_json IS NOT NULL)",
		"INSERT INTO core_schema_migrations(version) VALUES(13)",
	} {
		if !strings.Contains(actionPlannedReconciliationMigration, required) {
			t.Fatalf("planned Action migration missing %q", required)
		}
	}
	core := CoreMigration()
	if strings.LastIndex(core, "INSERT INTO core_schema_migrations(version) VALUES(12)") >
		strings.LastIndex(core, "INSERT INTO core_schema_migrations(version) VALUES(13)") {
		t.Fatal("planned Action reconciliation migration is not ordered after execution reconciliation")
	}
	if strings.Contains(strings.ToLower(actionPlannedReconciliationMigration), "insert into external_operations") ||
		strings.Contains(strings.ToLower(actionPlannedReconciliationMigration), "update external_operations") {
		t.Fatal("schema upgrade must not fabricate historical Action dispositions")
	}
}
