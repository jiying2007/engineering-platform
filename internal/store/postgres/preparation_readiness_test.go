package postgres

import (
	"context"
	"testing"
)

func TestPreparationStoreReadinessDoesNotConsumeUnmigratedInputs(t *testing.T) {
	s := integrationStore(t)
	startWorkerFixture(t, s, "preparation-readiness")
	relayWorkerFixture(t, s)
	workerSQL(t, s, "DROP TABLE worker_preparations")
	workerSQL(t, s, "DELETE FROM core_schema_migrations WHERE version=4")
	if err := s.PreparationReady(context.Background()); err == nil {
		t.Fatal("missing version-4 schema accepted")
	}
	assertCount(t, s, "SELECT count(*) FROM worker_inbox WHERE state='PENDING' AND lease_generation=0", 1)
	workerOK(t, s.ApplyCoreMigration(context.Background()))
	workerOK(t, s.PreparationReady(context.Background()))
	assertCount(t, s, "SELECT count(*) FROM worker_inbox WHERE state='PENDING' AND lease_generation=0", 1)
}
