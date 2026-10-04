-- Immutable stopped-source artifact observations; not resume/takeover authority.
BEGIN;
SELECT pg_advisory_xact_lock(741260924002);
DO $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM core_schema_migrations WHERE version=10) THEN
 CREATE TABLE worker_codex_source_checkpoints (
  execution_id text PRIMARY KEY REFERENCES worker_codex_executions(execution_id),
  checkpoint_json jsonb NOT NULL,
  checkpoint_digest text NOT NULL,
  received_at timestamptz NOT NULL DEFAULT clock_timestamp()
 );
 INSERT INTO core_schema_migrations(version) VALUES(10);
 END IF;
END $$;
COMMIT;
