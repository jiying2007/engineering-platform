BEGIN;

CREATE TABLE IF NOT EXISTS platform_state (
    singleton_id        boolean PRIMARY KEY DEFAULT true CHECK (singleton_id),
    recovery_epoch      bigint NOT NULL DEFAULT 0 CHECK (recovery_epoch >= 0),
    recovery_mode       text NOT NULL DEFAULT 'NORMAL'
                        CHECK (recovery_mode IN ('NORMAL', 'RECOVERY_RECONCILIATION')),
    updated_at          timestamptz NOT NULL DEFAULT now()
);

INSERT INTO platform_state (singleton_id)
VALUES (true)
ON CONFLICT (singleton_id) DO NOTHING;

CREATE TABLE IF NOT EXISTS work_items (
    work_item_id        text PRIMARY KEY,
    source_ref          text,
    title               text NOT NULL,
    human_owner         text NOT NULL,
    target_id           text,
    assurance_class     text,
    active_task_contract_digest text,
    active_run_id        text,
    state               text NOT NULL,
    version             bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at          timestamptz NOT NULL,
    updated_at          timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS verification_plans (
    verification_plan_id  text NOT NULL,
    plan_digest           text PRIMARY KEY,
    plan_json             jsonb NOT NULL,
    created_at            timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS task_contracts (
    task_contract_id    text NOT NULL,
    revision            bigint NOT NULL CHECK (revision > 0),
    work_item_id        text NOT NULL REFERENCES work_items(work_item_id),
    task_type           text NOT NULL,
    content_digest      text NOT NULL,
    repository          text NOT NULL,
    base_commit         text NOT NULL,
    target_id           text,
    verification_plan_id text NOT NULL,
    verification_plan_digest text NOT NULL REFERENCES verification_plans(plan_digest),
    contract_json       jsonb NOT NULL,
    created_at          timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (task_contract_id, revision),
    UNIQUE (content_digest)
);

CREATE TABLE IF NOT EXISTS run_input_manifests (
    run_input_manifest_digest text PRIMARY KEY,
    run_id                    text NOT NULL UNIQUE,
    task_contract_digest      text NOT NULL REFERENCES task_contracts(content_digest),
    runtime_profile           text NOT NULL,
    tool_profile              text NOT NULL,
    worker_profile            text NOT NULL,
    policy_profile            text NOT NULL,
    manifest_json             jsonb NOT NULL,
    created_at                timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS runs (
    run_id                  text PRIMARY KEY,
    task_contract_digest    text NOT NULL REFERENCES task_contracts(content_digest),
    run_input_manifest_digest text NOT NULL REFERENCES run_input_manifests(run_input_manifest_digest),
    state                   text NOT NULL,
    version                 bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    current_epoch           bigint NOT NULL DEFAULT 0 CHECK (current_epoch >= 0),
    current_attempt_id      text,
    control_owner           text NOT NULL DEFAULT 'RUNTIME'
                            CHECK (control_owner IN ('RUNTIME', 'HUMAN')),
    created_at              timestamptz NOT NULL DEFAULT now(),
    updated_at              timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS run_attempts (
    run_id              text NOT NULL REFERENCES runs(run_id),
    attempt_id          text NOT NULL,
    execution_epoch     bigint NOT NULL CHECK (execution_epoch > 0),
    worker_id           text,
    disposition         text,
    started_at          timestamptz NOT NULL,
    ended_at            timestamptz,
    PRIMARY KEY (run_id, attempt_id),
    UNIQUE (run_id, execution_epoch)
);

CREATE TABLE IF NOT EXISTS sessions (
    run_id              text PRIMARY KEY REFERENCES runs(run_id),
    execution_epoch     bigint NOT NULL CHECK (execution_epoch > 0),
    control_owner       text NOT NULL CHECK (control_owner IN ('RUNTIME', 'HUMAN')),
    last_steering_sequence bigint NOT NULL DEFAULT 0 CHECK (last_steering_sequence >= 0),
    paused              boolean NOT NULL DEFAULT false,
    updated_at          timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS steering_commands (
    steering_command_id text PRIMARY KEY,
    run_id              text NOT NULL REFERENCES runs(run_id),
    execution_epoch     bigint NOT NULL CHECK (execution_epoch > 0),
    sequence            bigint NOT NULL CHECK (sequence > 0),
    actor               text NOT NULL,
    content_digest      text NOT NULL,
    delivery_state      text NOT NULL DEFAULT 'RECORDED',
    created_at          timestamptz NOT NULL,
    UNIQUE (run_id, execution_epoch, sequence)
);

CREATE TABLE IF NOT EXISTS checkpoints (
    checkpoint_id           text PRIMARY KEY,
    run_id                  text NOT NULL REFERENCES runs(run_id),
    task_contract_digest    text NOT NULL REFERENCES task_contracts(content_digest),
    run_input_manifest_digest text NOT NULL REFERENCES run_input_manifests(run_input_manifest_digest),
    execution_epoch         bigint NOT NULL CHECK (execution_epoch > 0),
    source_tree_digest      text NOT NULL,
    diff_digest             text,
    checkpoint_json         jsonb NOT NULL,
    content_digest          text NOT NULL UNIQUE,
    created_at              timestamptz NOT NULL
);

CREATE TABLE IF NOT EXISTS external_operations (
    operation_id        text PRIMARY KEY,
    run_id              text NOT NULL REFERENCES runs(run_id),
    execution_epoch     bigint NOT NULL CHECK (execution_epoch > 0),
    recovery_epoch      bigint NOT NULL CHECK (recovery_epoch >= 0),
    action              text NOT NULL,
    risk_class          text NOT NULL CHECK (risk_class IN ('OBSERVE', 'CONTROLLED_MUTATION', 'HIGH_RISK')),
    capability          text NOT NULL,
    idempotency_key     text NOT NULL UNIQUE,
    request_digest      text NOT NULL,
    state               text NOT NULL CHECK (
        state IN (
            'PLANNED',
            'DISPATCHED',
            'CONFIRMED',
            'UNKNOWN',
            'RECONCILING',
            'SAFE_TO_RETRY',
            'MANUAL'
        )
    ),
    external_ref        text,
    observed_state      text,
    receipt_json        jsonb,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS artifacts (
    artifact_id         text PRIMARY KEY,
    content_digest      text NOT NULL,
    media_type          text,
    locator             text,
    metadata_json       jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at          timestamptz NOT NULL DEFAULT now(),
    UNIQUE (content_digest, artifact_id)
);

CREATE TABLE IF NOT EXISTS delivery_receipts (
    delivery_receipt_id     text PRIMARY KEY,
    work_item_id            text NOT NULL REFERENCES work_items(work_item_id),
    task_contract_digest    text NOT NULL REFERENCES task_contracts(content_digest),
    run_id                  text NOT NULL REFERENCES runs(run_id),
    target_id               text,
    base_commit             text NOT NULL,
    result_commit           text,
    subject_digest          text NOT NULL UNIQUE,
    receipt_json            jsonb NOT NULL,
    created_at              timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS delivery_run_idx
    ON delivery_receipts(run_id);

CREATE TABLE IF NOT EXISTS evidence (
    evidence_id         text PRIMARY KEY,
    delivery_receipt_id text NOT NULL REFERENCES delivery_receipts(delivery_receipt_id),
    subject_digest      text NOT NULL,
    issuer              text NOT NULL,
    procedure_ref       text NOT NULL,
    result              text NOT NULL,
    applicable          boolean NOT NULL DEFAULT true,
    evidence_json       jsonb NOT NULL,
    created_at          timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS evidence_subject_idx
    ON evidence(subject_digest);

CREATE TABLE IF NOT EXISTS verification_reports (
    verification_report_id text PRIMARY KEY,
    delivery_receipt_id    text NOT NULL REFERENCES delivery_receipts(delivery_receipt_id),
    verification_plan_id   text NOT NULL,
    verification_plan_digest text NOT NULL REFERENCES verification_plans(plan_digest),
    subject_digest         text NOT NULL,
    result                 text NOT NULL,
    verifier               text,
    report_json            jsonb NOT NULL,
    created_at             timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS verification_subject_idx
    ON verification_reports(subject_digest);

CREATE TABLE IF NOT EXISTS closure_receipts (
    closure_receipt_id      text PRIMARY KEY,
    work_item_id            text NOT NULL REFERENCES work_items(work_item_id),
    task_contract_digest    text NOT NULL REFERENCES task_contracts(content_digest),
    run_id                  text NOT NULL REFERENCES runs(run_id),
    delivery_receipt_id     text NOT NULL REFERENCES delivery_receipts(delivery_receipt_id),
    verification_report_id  text NOT NULL REFERENCES verification_reports(verification_report_id),
    review_report_id        text,
    subject_digest          text NOT NULL,
    result                  text NOT NULL,
    receipt_json            jsonb NOT NULL,
    created_at              timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS audit_journal_state (
    singleton_id        boolean PRIMARY KEY DEFAULT true CHECK (singleton_id),
    last_sequence       bigint NOT NULL DEFAULT 0 CHECK (last_sequence >= 0),
    last_digest         text NOT NULL DEFAULT ''
);

INSERT INTO audit_journal_state (singleton_id)
VALUES (true)
ON CONFLICT (singleton_id) DO NOTHING;

CREATE TABLE IF NOT EXISTS audit_events (
    sequence            bigint PRIMARY KEY CHECK (sequence > 0),
    event_type          text NOT NULL,
    aggregate_type      text,
    aggregate_id        text,
    payload_digest      text NOT NULL,
    previous_digest     text,
    event_digest        text NOT NULL UNIQUE,
    correlation_id      text,
    causation_id        text,
    created_at          timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS outbox_events (
    outbox_id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    outbox_key          text NOT NULL UNIQUE,
    topic               text NOT NULL,
    aggregate_type      text,
    aggregate_id        text,
    risk_class          text NOT NULL DEFAULT 'OBSERVE'
                        CHECK (risk_class IN ('OBSERVE', 'CONTROLLED_MUTATION', 'HIGH_RISK')),
    payload_json        jsonb NOT NULL,
    state               text NOT NULL DEFAULT 'PENDING'
                        CHECK (state IN ('PENDING', 'LEASED', 'DISPATCHED', 'DEAD_LETTER')),
    lease_owner         text,
    lease_until         timestamptz,
    attempt_count       bigint NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
    next_attempt_at     timestamptz NOT NULL DEFAULT now(),
    last_error          text,
    created_at          timestamptz NOT NULL DEFAULT now(),
    dispatched_at       timestamptz
);

CREATE INDEX IF NOT EXISTS outbox_claim_idx
    ON outbox_events(next_attempt_at,outbox_id)
    WHERE state IN ('PENDING','LEASED');

CREATE TABLE IF NOT EXISTS workers (
    worker_id           text PRIMARY KEY,
    protocol_version    text NOT NULL,
    runtime_providers   jsonb NOT NULL DEFAULT '[]'::jsonb,
    status              text NOT NULL,
    attributes_json     jsonb NOT NULL DEFAULT '{}'::jsonb,
    last_seen_at        timestamptz NOT NULL,
    updated_at          timestamptz NOT NULL DEFAULT now()
);

COMMIT;
