-- Delivery state extends existing steering authority; historical digest-only
-- rows are never made runnable. No control or model call is replayed on restore.
BEGIN;
SELECT pg_advisory_xact_lock(741260924002);
DO $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM core_schema_migrations WHERE version=9) THEN
 CREATE TABLE worker_codex_runtime (
  execution_id text PRIMARY KEY REFERENCES worker_codex_executions(execution_id),
  binding_json jsonb NOT NULL,
  state text NOT NULL CHECK(state IN ('ACTIVE','SEALED')),
  transcript_json jsonb,
  transcript_digest text,
  CHECK ((state='SEALED') = (transcript_json IS NOT NULL)),
  CHECK ((state='SEALED') = (transcript_digest IS NOT NULL))
 );
 ALTER TABLE steering_commands ADD COLUMN execution_id text REFERENCES worker_codex_executions(execution_id);
 ALTER TABLE steering_commands ADD COLUMN control_payload jsonb;
 ALTER TABLE steering_commands ADD COLUMN dispatched_at timestamptz;
 ALTER TABLE steering_commands ADD COLUMN resolved_at timestamptz;
 ALTER TABLE steering_commands ADD CONSTRAINT live_control_shape CHECK (
   (execution_id IS NULL AND control_payload IS NULL AND delivery_state='RECORDED') OR
   (execution_id IS NOT NULL AND control_payload IS NOT NULL AND
    delivery_state IN ('QUEUED','DISPATCHING','STEER_ACCEPTED','INTERRUPT_ACKNOWLEDGED','TURN_INTERRUPTED','NOT_APPLIED','UNKNOWN'))
 );
 CREATE INDEX steering_live_sequence ON steering_commands(execution_id,sequence) WHERE execution_id IS NOT NULL;
 INSERT INTO core_schema_migrations(version) VALUES(9);
 END IF;
END $$;
COMMIT;
