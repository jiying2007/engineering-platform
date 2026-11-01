# Requirements — Actionable-only operational attention

## Problem and scope

The existing production v6 status contains authoritative recovery and queue observations but is intentionally not a READY/production-qualified signal. Operators cannot easily distinguish actionable exceptions from ordinary running work. Add one **read-only advisory projection over the same status**, with no second data authority, no notification sender and no modified Recovery/Action state machine.

- ACC-001: healthy/in-progress activity does not produce an attention item merely because work exists.
- ACC-002: UNKNOWN outcomes, failed/expired leases, dead letters, manual/reconciling actions and active recovery appear as stable typed reasons with non-executable next-action categories.
- ACC-003: empty projected attention is never "READY" or product qualification; PLANNED and DISPATCHED continue to block canonical status as before.
- ACC-004: source must pass the existing status's full validation and freshness; bad/stale/forged data fail closed.
- ACC-005: HTTP GET requires authenticated `access.Read`; missing store/status and canceled reads fail closed; no cache and no side effects.
- ACC-006: no Codex/Work invocation, credential retrieval, provider selection or new runtime.

This is a bounded read-only P1 enhancement, not completion of #228/#229 or external certification.

## Verification

Go tests `go test ./internal/production ./internal/api -run 'TestOperationalAttention'` and `gofmt -l` for changed files. Full native PR CI is not invoked while its Codex CLI qualification conflicts with the existing resource restriction; explicitly retain this as a blocker rather than mark source complete.
