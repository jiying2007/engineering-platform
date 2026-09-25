-- One reservation per Run: an ambiguous launch is NEVER automatically replayed.
-- Existing historical input/preparation receipts are not updated or promoted.
BEGIN;
SELECT pg_advisory_xact_lock(741260924002);
DO $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM core_schema_migrations WHERE version=5) THEN
CREATE TABLE worker_offline_executions (
 execution_id text PRIMARY KEY,
 run_id text NOT NULL UNIQUE REFERENCES runs(run_id),
 inbox_id bigint NOT NULL REFERENCES worker_inbox(inbox_id),
 worker_subject text NOT NULL,
 token_json jsonb NOT NULL,
 profile_json jsonb NOT NULL,
 preparation_digest text NOT NULL,
 state text NOT NULL CHECK(state IN ('AUTHORIZED','FINISHED','UNKNOWN')),
 started_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 lease_until timestamptz NOT NULL,
 receipt_json jsonb,
 CHECK ((state='FINISHED') = (receipt_json IS NOT NULL))
);
INSERT INTO core_schema_migrations(version) VALUES(5);
 END IF;
END $$;
COMMIT;
