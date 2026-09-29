# Relay provider repository prequalification v1

Date: 2026-09-29

Status: **REPOSITORY-SIDE PREQUALIFICATION ONLY — NOT LIVE QUALIFICATION**

This contract advances issue #106 without requiring a user account, relay
credential or live model call. It deliberately cannot admit `company-relay`
into Provider v3.

## Why this stage exists

Codex supports explicit custom model providers through user-level
`model_provider` / `model_providers.<id>` configuration. The current supported
custom-provider wire protocol is the Responses API. Custom providers can source
credentials from a dedicated environment key or a command that returns a Bearer
token.

Engineering Platform must freeze those security-relevant choices before any
credential is supplied. A repository prequalification therefore captures the
Codex-facing provider contract while keeping all live authority false.

## Contract

`eng relay-prequalification --contract CONTRACT.json` accepts a strict JSON
contract containing:

- `provider_id=company-relay`;
- one non-reserved Codex custom provider ID;
- exact base URL without embedded credentials/query/fragment; HTTPS by default;
- optional insecure HTTP only when explicitly enabled and the host is a literal private/loopback IP;
- `wire_api=responses`;
- exactly one credential *locator*:
  - a dedicated relay environment variable; or
  - an absolute token-command path;
- exact requested model slug;
- request and stream retries fixed to zero for the first qualification;
- bounded stream idle timeout;
- WebSocket and standalone web-search surfaces disabled;
- SHA-256 digests of the archived gateway policy, privacy/retention policy and
  model-mapping policy.

The contract never contains a bearer token, API key, saved ChatGPT login,
workload assertion or account identifier.

## Operator pack

Operators do not need to hand-calculate policy digests or hand-author the
contract. The non-secret pack command derives both the canonical Contract and
its Assessment:

```sh
eng relay-prequalification-pack \
  --codex-provider company-relay-v1 \
  --base-url https://relay.example.com/v1 \
  --auth-env-key COMPANY_RELAY_TOKEN \
  --model relay-model-slug \
  --gateway-policy ./gateway-policy.md \
  --privacy-policy ./privacy-policy.md \
  --model-mapping ./model-mapping.md
```

Use `--auth-command /absolute/path/to/token-helper` instead of
`--auth-env-key` when the future live lane will obtain a Bearer token through
a host-owned command. The command is recorded only as a credential locator; it
is not executed by repository prequalification.

For an internal lab relay that intentionally uses cleartext HTTP, the operator
must add `--allow-insecure-private-http`. That exception is accepted only when
the URL host is a literal RFC1918/ULA private or loopback IP. Public IPs,
link-local addresses and DNS hostnames remain rejected under HTTP. The exception
bit is part of the canonical Contract and therefore changes the
`provider_config_digest`.

The three policy files are read as bounded regular files (symlinks are rejected)
and hashed by exact bytes. They should describe, at minimum:

- gateway protocol/security/version and transformation policy;
- privacy, retention, operator access and egress policy;
- requested-model to effective-upstream/model mapping and downgrade policy.

The pack output is deterministic JSON containing both `contract` and
`assessment`. Redirect it to an operator-controlled file if a retained
prequalification artifact is desired. The output contains no credential
material or account identifier.

## Output semantics

A successful command emits a deterministic assessment containing the
`provider_config_digest` plus explicit negative authority facts:

- `account_verified=false`;
- `live_model_turn_executed=false`;
- `provider_admitted=false`;
- next gate = `live-read-only-no-tool-provider-qualification`.

This is intentional. Repository prequalification proves only that the proposed
relay configuration is complete, bounded and deterministic enough to qualify
later.

## Fail-closed constraints

The v1 prequalification rejects:

- built-in/reserved Codex provider IDs;
- HTTP endpoints unless the explicit private-HTTP exception is enabled for a literal private/loopback IP;
- public, link-local or DNS-hosted cleartext HTTP endpoints;
- credential-bearing provider URLs;
- non-Responses wire modes;
- reuse of `OPENAI_*` or `CODEX_*` environment credentials for a relay;
- automatic request/stream retries;
- WebSocket or standalone web-search capability during the first qualification;
- missing policy/model-mapping digests.

These restrictions minimize ambiguity around model-turn replay, provider
substitution and tool/network surfaces before the relay is trusted.

## What remains deliberately deferred

No account or credential verification is required at this stage.

Before `company-relay` can be added to `internal/provideridentity`, #106 still
requires one concrete live provider qualification that proves, for the exact
contract digest:

1. the configured relay endpoint accepts the intended credential;
2. one read-only, no-tool model turn succeeds;
3. requested and effective upstream/model identity are retained or the lane is
   rejected if the gateway cannot prove them;
4. gateway request/operation correlation is retained;
5. prompt/response transformation and retention claims match the archived
   policy digests;
6. timeout/ambiguous outcomes map to Recovery rather than blind replay.

Only after that evidence exists may a separate change admit an exact Provider v3
identity. A future unattended relay identity needs its own qualification before
it can satisfy #105.
