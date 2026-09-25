-- Independent Review is mandatory for Closure. Historical closures predate the
-- ReviewReport authority and cannot be silently promoted or backfilled.
BEGIN;
SELECT pg_advisory_xact_lock(741260924002);
DO $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM core_schema_migrations WHERE version=6) THEN
  IF EXISTS(SELECT 1 FROM closure_receipts) THEN
   RAISE EXCEPTION 'migration 6 requires operator reconciliation of legacy closure_receipts';
  END IF;

  CREATE TABLE review_reports (
   review_report_id text PRIMARY KEY,
   delivery_receipt_id text NOT NULL REFERENCES delivery_receipts(delivery_receipt_id),
   verification_report_id text NOT NULL UNIQUE REFERENCES verification_reports(verification_report_id),
   task_contract_digest text NOT NULL REFERENCES task_contracts(content_digest),
   subject_digest text NOT NULL,
   reviewer text NOT NULL,
   result text NOT NULL CHECK(result IN ('PASS','FAIL')),
   report_json jsonb NOT NULL,
   created_at timestamptz NOT NULL
  );
  CREATE INDEX review_subject_idx ON review_reports(subject_digest);

  ALTER TABLE closure_receipts
    ALTER COLUMN review_report_id SET NOT NULL;
  ALTER TABLE closure_receipts
    ADD CONSTRAINT closure_review_report_fk
    FOREIGN KEY (review_report_id) REFERENCES review_reports(review_report_id);

  INSERT INTO core_schema_migrations(version) VALUES(6);
 END IF;
END $$;
COMMIT;
