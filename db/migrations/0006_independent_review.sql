BEGIN;

CREATE TABLE IF NOT EXISTS review_reports (
    review_report_id        text PRIMARY KEY,
    delivery_receipt_id     text NOT NULL REFERENCES delivery_receipts(delivery_receipt_id),
    verification_report_id  text NOT NULL REFERENCES verification_reports(verification_report_id),
    subject_digest          text NOT NULL,
    reviewer                text NOT NULL,
    result                  text NOT NULL CHECK (result IN ('PASS','REJECT')),
    report_json             jsonb NOT NULL,
    created_at              timestamptz NOT NULL
);

CREATE INDEX IF NOT EXISTS review_subject_idx
    ON review_reports(subject_digest);

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM closure_receipts c
        WHERE c.review_report_id IS NULL
           OR NOT EXISTS (
                SELECT 1
                FROM review_reports r
                WHERE r.review_report_id = c.review_report_id
                  AND r.delivery_receipt_id = c.delivery_receipt_id
                  AND r.verification_report_id = c.verification_report_id
                  AND r.subject_digest = c.subject_digest
                  AND r.result = 'PASS'
           )
    ) THEN
        RAISE EXCEPTION 'historical closure receipts require operator review reconciliation before review authority migration';
    END IF;
END $$;

ALTER TABLE closure_receipts
    ALTER COLUMN review_report_id SET NOT NULL;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'closure_receipts_review_report_fk'
          AND conrelid = 'closure_receipts'::regclass
    ) THEN
        ALTER TABLE closure_receipts
            ADD CONSTRAINT closure_receipts_review_report_fk
            FOREIGN KEY (review_report_id)
            REFERENCES review_reports(review_report_id);
    END IF;
END $$;

COMMIT;
