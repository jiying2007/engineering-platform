-- Preserve complete historical provenance checks as operation history grows.
-- This additive lookup index does not change or certify any receipt or audit.
-- Apply with services quiesced, under the existing migration authority budget.
BEGIN;
SELECT pg_advisory_xact_lock(741260924002);
DO $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM core_schema_migrations WHERE version=14) THEN
  CREATE INDEX audit_external_operation_provenance_idx
   ON audit_events(event_type,aggregate_id,causation_id,sequence)
   INCLUDE (payload_digest)
   WHERE aggregate_type='ExternalOperation';
  INSERT INTO core_schema_migrations(version) VALUES(14);
 END IF;
END $$;
COMMIT;
