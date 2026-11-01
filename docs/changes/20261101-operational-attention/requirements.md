# Requirements — Actionable-only operational attention

## Problem and scope

The existing production v6 status contains authoritative recovery and queue observations but is intentionally not a READY/production-qualified signal. Operators cannot easily distinguish actionable exceptions from ordinary running work. Add one **read-only advisory projection over the same status**, with no second data authority, no notification sender and no modified Recovery/Action state machine.

- ACC-001: healthy/in-progress activity does not produce an attention item merely because work exists.
- ACC-002: UNKNOWN outcomes, failed/expired leases, dead letters, manual/reconciling actions and active recovery appear as stable typed reasons with non-executable next-action categories.
- ACC-003: empty projected attention is never "READY" or product qualification; PLANNED and DISPATCHED continue to block canonical status as before.
- ACC-004: source must pass the existing status's full validation and freshness; bad/stale/forged data fail closed.
- ACC-005: HTTP GET requires authenticated `access.Read`; missing store/status and canceled reads fail closed; no cache and no side effects.
- ACC-006: the new API does not invoke any Codex/Work runtime, credential, provider, executable or external effect. GitHub's independent CI may run its **separately authorized zero-model-turn** Codex app-server compatibility probe; it must not execute `turn/start`, use credentials or incur model inference quota.

This is a bounded read-only P1 enhancement, not completion of #228/#229 or external certification.

## Verification

- Exact-source branch-only [run #41265017089](https://github.com/jiying2007/engineering-platform/actions/runs/41265017089) at `af27045fea4f83f4dc3b9ae400cfea2ce8499287`: `gofmt`, `go test -count=1 -timeout 6m ./internal/production ./internal/api`, `go vet ./internal/production ./internal/api` all **SUCCESS** under the repository `go.mod` toolchain.
- The temporary branch-only workflow was removed after the run to avoid redundant maintenance. Full normal protected PR and fresh-main CI must be independently observed.
- A required Codex compatibility job must explicitly attest `model_turn_executed=false` and `credential_used=false`; `turn/start` or actual paid model execution is not authorized.
