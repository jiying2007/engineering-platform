-- Stop pre-inbox consumers before upgrade. No external execution is enabled.
BEGIN;
SELECT pg_advisory_xact_lock(741260924002);
DO $$
BEGIN
 IF NOT EXISTS (SELECT 1 FROM core_schema_migrations WHERE version=3) THEN
  CREATE TABLE worker_inbox (
   inbox_id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
   outbox_key text NOT NULL UNIQUE REFERENCES outbox_events(outbox_key),
   run_id text NOT NULL UNIQUE REFERENCES runs(run_id),
   intent_digest text NOT NULL CHECK (intent_digest ~ '^sha256:[0-9a-f]{64}$'),
   intent_json jsonb NOT NULL,
   worker_profile text NOT NULL,
   state text NOT NULL DEFAULT 'PENDING'
    CHECK (state IN ('PENDING','LEASED','INPUT_VALIDATED','REVOKED')),
   lease_owner text,
   lease_generation bigint NOT NULL DEFAULT 0 CHECK (lease_generation >= 0),
   lease_recovery_epoch bigint CHECK (lease_recovery_epoch >= 0),
   lease_started_at timestamptz,
   lease_until timestamptz,
   receipt_json jsonb,
   received_at timestamptz NOT NULL DEFAULT clock_timestamp(),
   updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
   CHECK (state <> 'LEASED' OR (lease_owner IS NOT NULL AND lease_generation > 0
    AND lease_recovery_epoch IS NOT NULL AND lease_started_at IS NOT NULL AND lease_until IS NOT NULL)),
   CHECK ((state='INPUT_VALIDATED') = (receipt_json IS NOT NULL))
  );
  CREATE INDEX worker_inbox_claim_idx ON worker_inbox(worker_profile,inbox_id)
   WHERE state IN ('PENDING','LEASED');
  INSERT INTO core_schema_migrations(version) VALUES(3);
 END IF;
END $$;
COMMIT;
