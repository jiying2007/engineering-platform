-- Recovery-only no-replay disposition of a pre-dispatch Action reservation.
-- Never rewrite historical external receipts and never grant a new dispatch.
-- This migration is additive to existing evidence. Production operators must
-- quiesce services and complete external migration acceptance before activation.
BEGIN;
SELECT pg_advisory_xact_lock(741260924002);
DO $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM core_schema_migrations WHERE version=13) THEN
  ALTER TABLE external_operations ADD COLUMN reconciliation_json jsonb;
  ALTER TABLE external_operations DROP CONSTRAINT external_operations_state_check;
  ALTER TABLE external_operations
   ADD CONSTRAINT external_operations_state_check CHECK (
    state IN ('PLANNED','DISPATCHED','CONFIRMED','UNKNOWN','RECONCILING',
              'SAFE_TO_RETRY','MANUAL','ABANDONED_RECONCILED')
   );
  -- A terminal non-success disposition must retain its exact receipt, while
  -- live and historical nonterminal states may not impersonate that receipt.
  ALTER TABLE external_operations
   ADD CONSTRAINT external_operations_reconciliation_check CHECK (
    (state='ABANDONED_RECONCILED') = (reconciliation_json IS NOT NULL)
   );
  INSERT INTO core_schema_migrations(version) VALUES(13);
 END IF;
END $$;
COMMIT;
