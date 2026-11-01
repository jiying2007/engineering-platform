# Operational attention: low-noise read-only projection

Source: `production.OperationalStatus` v6, exact validated database snapshot.

GET `/api/v1/operations/attention` is intended for authenticated read-capable operators. It returns `schema_version=engineering-operational-attention/v1`, `source_scope=database-authority-snapshot/advisory`, `captured_at`, `requires_attention` and typed `items`. It does **not** issue notifications, change Action recovery or grant readiness. Healthy in-flight operations do not create a human attention alert. Unknown effects, manual/reconciling states, expired leases and dead letters remain visible.

Use `GET /api/v1/operations/status` for authority, durability and readiness semantics. Never treat an empty attention array as production-certified or safe to retry external side effects. Tests: `go test ./internal/production ./internal/api -run TestOperationalAttention`. No Codex/Work required for this target selector.
