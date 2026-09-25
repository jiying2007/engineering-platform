-- Additive receipt-only extension. No replay, Run completion or reclassification.
BEGIN;
SELECT pg_advisory_xact_lock(741260924002);
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM core_schema_migrations WHERE version=4) THEN
        CREATE TABLE worker_preparations (
            inbox_id bigint PRIMARY KEY REFERENCES worker_inbox(inbox_id),
            run_id text NOT NULL UNIQUE REFERENCES runs(run_id),
            lease_generation bigint NOT NULL CHECK (lease_generation>0),
            worker_subject text NOT NULL,
            facts_digest text NOT NULL,
            receipt_json jsonb NOT NULL,
            created_at timestamptz NOT NULL DEFAULT clock_timestamp()
        );
        INSERT INTO core_schema_migrations(version) VALUES(4);
    END IF;
END $$;
COMMIT;
