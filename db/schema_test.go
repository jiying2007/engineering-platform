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
