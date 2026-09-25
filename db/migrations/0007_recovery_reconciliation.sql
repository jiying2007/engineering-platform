-- Recovery completion requires an immutable, database-generated reconciliation proof.
-- No historical proof is synthesized. One proof is allowed per exact recovery epoch.
BEGIN;
SELECT pg_advisory_xact_lock(741260924002);
DO $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM core_schema_migrations WHERE version=7) THEN
  CREATE TABLE recovery_reconciliation_proofs (
   recovery_epoch bigint PRIMARY KEY CHECK (recovery_epoch > 0),
   reconciler text NOT NULL,
   facts_digest text NOT NULL CHECK (facts_digest ~ '^sha256:[0-9a-f]{64}$'),
   audit_sequence bigint NOT NULL CHECK (audit_sequence >= 0),
   audit_digest text,
   proof_json jsonb NOT NULL,
   created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
   CHECK ((audit_sequence=0 AND audit_digest IS NULL) OR
          (audit_sequence>0 AND audit_digest ~ '^sha256:[0-9a-f]{64}$'))
  );
  INSERT INTO core_schema_migrations(version) VALUES(7);
 END IF;
END $$;
COMMIT;
