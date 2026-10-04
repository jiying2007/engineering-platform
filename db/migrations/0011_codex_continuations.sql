-- Explicitly observed interruption is a stopped, unsuccessful execution. It is
-- never FINISHED and never replayed. Prior transcript/checkpoint bytes are kept.
BEGIN;
SELECT pg_advisory_xact_lock(741260924002);
DO $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM core_schema_migrations WHERE version=11) THEN
  ALTER TABLE worker_codex_executions DROP CONSTRAINT worker_codex_executions_state_check;
  ALTER TABLE worker_codex_executions ADD CONSTRAINT worker_codex_executions_state_check
   CHECK (state IN ('AUTHORIZED','FINISHED','UNKNOWN','STOPPED_NO_DELIVERY'));
  CREATE TABLE codex_continuations (
   source_run_id text PRIMARY KEY REFERENCES runs(run_id),
   successor_run_id text NOT NULL UNIQUE REFERENCES runs(run_id),
   actor text NOT NULL,
   request_digest text NOT NULL CHECK(request_digest ~ '^sha256:[0-9a-f]{64}$'),
   receipt_json jsonb NOT NULL,
   receipt_digest text NOT NULL CHECK(receipt_digest ~ '^sha256:[0-9a-f]{64}$'),
   created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
   CHECK (source_run_id <> successor_run_id)
  );
  INSERT INTO core_schema_migrations(version) VALUES(11);
 END IF;
END $$;
COMMIT;
