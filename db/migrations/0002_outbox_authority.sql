-- Quiesce all pre-v2 dispatchers before upgrade. Old owner-only ACK code is not
-- compatible with this authority contract. Do not downgrade and run it again.
BEGIN;
SELECT pg_advisory_xact_lock(741260924002);
CREATE TABLE IF NOT EXISTS core_schema_migrations (
    version integer PRIMARY KEY,
    applied_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM core_schema_migrations WHERE version=2) THEN
        ALTER TABLE outbox_events ADD COLUMN lease_recovery_epoch bigint
            CHECK (lease_recovery_epoch >= 0);
        ALTER TABLE outbox_events ALTER COLUMN risk_class DROP DEFAULT;
        -- The old default erased whether OBSERVE was explicit. Only trusted
        -- Core topics can be classified automatically; unknown queued messages
        -- are quarantined for assessment, NOT silently replayed.
        UPDATE outbox_events
        SET state='DEAD_LETTER',lease_owner=NULL,lease_until=NULL,
            last_error='outbox-v2 migration: legacy topic requires risk/reconciliation review'
        WHERE state='LEASED' OR (state='PENDING' AND topic NOT IN ('run.started','work.created'));
        UPDATE outbox_events
        SET risk_class=CASE WHEN topic='run.started' THEN 'CONTROLLED_MUTATION' ELSE 'OBSERVE' END,
            state='PENDING',lease_owner=NULL,lease_until=NULL,
            last_error='outbox-v2 migration: reclassified Core topic; old lease revoked'
        WHERE state='PENDING' AND topic IN ('run.started','work.created');
        -- NOT VALID retains historical terminal evidence without rewriting it;
        -- it still enforces this invariant on every new insert/update.
        ALTER TABLE outbox_events ADD CONSTRAINT outbox_core_topic_risk_check CHECK (
            (topic <> 'run.started' OR risk_class='CONTROLLED_MUTATION') AND
            (topic <> 'work.created' OR risk_class='OBSERVE')
        ) NOT VALID;
        INSERT INTO core_schema_migrations(version) VALUES(2);
    END IF;
END $$;
COMMIT;
