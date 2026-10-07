-- Explicit Recovery reconciliation for non-replayable Worker executions.
-- ABANDONED_RECONCILED is not FINISHED and never carries a result receipt.
BEGIN;
SELECT pg_advisory_xact_lock(741260924002);
DO $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM core_schema_migrations WHERE version=12) THEN
  ALTER TABLE worker_offline_executions ADD COLUMN reconciliation_json jsonb;
  ALTER TABLE worker_offline_executions DROP CONSTRAINT worker_offline_executions_state_check;
  ALTER TABLE worker_offline_executions
   ADD CONSTRAINT worker_offline_executions_state_check
    CHECK(state IN ('AUTHORIZED','FINISHED','UNKNOWN','ABANDONED_RECONCILED'));
  ALTER TABLE worker_offline_executions
   ADD CONSTRAINT worker_offline_executions_reconciliation_check
    CHECK ((state='ABANDONED_RECONCILED') = (reconciliation_json IS NOT NULL));

  ALTER TABLE worker_codex_executions ADD COLUMN reconciliation_json jsonb;
  ALTER TABLE worker_codex_executions DROP CONSTRAINT worker_codex_executions_state_check;
  ALTER TABLE worker_codex_executions
   ADD CONSTRAINT worker_codex_executions_state_check
    CHECK(state IN ('AUTHORIZED','FINISHED','UNKNOWN','STOPPED_NO_DELIVERY','ABANDONED_RECONCILED'));
  ALTER TABLE worker_codex_executions
   ADD CONSTRAINT worker_codex_executions_reconciliation_check
    CHECK ((state='ABANDONED_RECONCILED') = (reconciliation_json IS NOT NULL));

  INSERT INTO core_schema_migrations(version) VALUES(12);
 END IF;
END $$;
COMMIT;
