# Design — Project existing snapshot, not another authority

```text
PostgreSQL read-only RepeatableRead status transaction
      ↓ existing production.OperationalStatus v6
status.ValidateAt(now) [all counters/recovery/clock/authority]
      ↓ ProjectOperationalAttention
bounded list {reason, count, next_action} (no command)
      ↓ GET /api/v1/operations/attention
authenticated access.Read + no-store + no mutation
```

This projection never queries database tables independently and never synthesizes READY, production qualification or a completed task. It does not change `EvaluateSnapshot`, `ValidateAt`, migrations, Action Gateway, recovery epochs, retry, outbox or worker admission.

Ordinary runs/queues and in-flight PLANNED/DISPATCHED operations are omitted from the human action list **because aggregate counts alone cannot distinguish active work from aged incidents**. They remain visible and blocking in existing `/operations/status`; callers must not infer healthy operation from an empty list. Future source-specific notifications require exact action IDs, incident identity, lifecycle evidence, dedup/rearm and owner-approved delivery; aggregate counters are insufficient.

Type declarations use static action categories. No free-text messages, raw logs, secrets or other source payloads enter the response. Response identifies the database-status advisory source and captured_at.

## Security

Remote route is allowlisted on the existing authenticated handler with `access.Read`, and the handler checks the identity again. The loopback test handler also rejects unauthorized calls. Cancelled/unavailable/stale source fails with 503. No recovered Action is automatically retried and an empty projection cannot grant any capability.

## Rollback

Revert the new `internal/production/attention*.go` and `internal/api/operational_attention*.go`, plus two additive route lines. No migration, persisted state, secrets, configuration or runtime execution side effects.
