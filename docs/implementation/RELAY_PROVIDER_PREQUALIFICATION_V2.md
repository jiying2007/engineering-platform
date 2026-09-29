# Relay provider repository prequalification v2

Date: 2026-09-30

Status: **REPOSITORY-SIDE PREQUALIFICATION ONLY — NOT LIVE QUALIFICATION**

v2 replaces v1 before any relay provider has been live-qualified or admitted.
There is no v1 compatibility shim on current main.

The purpose is to make the future Provider v3 `provider_config_digest` bind the
exact Codex-facing configuration bytes, not merely a higher-level relay policy
object. Account, credential and live-model verification remain explicitly
deferred.

## Authority model

Repository prequalification has four distinct immutable facts:

1. **Contract digest** — exact non-secret relay policy/configuration fields.
2. **Renderer contract version** — exact repository renderer semantics.
3. **Codex config digest** — SHA-256 of the exact generated user-level TOML.
4. **Provider config digest** — deterministic digest binding the three facts
   above.

Only the final provider config digest is suitable for later Provider v3 identity.
Changing any rendered Codex field changes that digest.

The Assessment still requires:

- `account_verified=false`;
- `live_model_turn_executed=false`;
- `provider_admitted=false`;
- next gate = `live-read-only-no-tool-provider-qualification`.

## Codex configuration surface

The renderer targets Codex's user-level provider configuration. Current Codex
configuration semantics require provider-sensitive keys such as
`model_provider` and `model_providers` to be user-level rather than a
repository project override.

The generated TOML freezes:

- exact requested model;
- exact custom provider ID;
- exact provider base URL;
- `wire_api=responses`;
- `requires_openai_auth=false`;
- request retry count;
- stream retry count;
- stream idle timeout;
- WebSocket support flag;
- standalone web-search support flag;
- exactly one credential locator:
  - `env_key`; or
  - `auth.command` with exact timeout/refresh policy.

No bearer token, API key value, saved ChatGPT login, WIF assertion or account ID
is rendered or retained.

For initial qualification:

- request retries = 0;
- stream retries = 0;
- WebSockets = false;
- standalone web search = false;
- command-token `refresh_interval_ms=0`;
- command-token timeout must be explicitly bounded between 1000 and 30000 ms.

The zero refresh interval prevents scheduled token-helper execution during the
initial qualification posture. The token helper itself is never executed by
repository prequalification.

## Transport policy

HTTPS is the default.

Cleartext HTTP requires the explicit
`allow_insecure_private_http=true` contract bit and is accepted only for a
literal private or loopback IP endpoint. Public IP, link-local and DNS-hosted
HTTP endpoints fail closed.

The exception bit is part of the contract and therefore changes the provider
configuration identity.

## Operator pack

Generate a complete non-secret Contract + Assessment from exact policy files:

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

For a future host-owned token helper:

```sh
eng relay-prequalification-pack \
  --codex-provider company-relay-v1 \
  --base-url https://relay.example.com/v1 \
  --auth-command /usr/local/bin/company-relay-token \
  --auth-command-timeout-ms 5000 \
  --model relay-model-slug \
  --gateway-policy ./gateway-policy.md \
  --privacy-policy ./privacy-policy.md \
  --model-mapping ./model-mapping.md
```

The three policy files are bounded regular files, symlinks are rejected, and
their exact bytes are hashed into the Contract. The operator pack performs no
network access and neither reads nor executes credential material.

For a private lab relay using cleartext HTTP, add
`--allow-insecure-private-http`; the endpoint must still satisfy the literal
private/loopback-IP rule.

## Strict revalidation

An already-materialized v2 Contract can be independently revalidated:

```sh
eng relay-prequalification --contract relay-contract-v2.json
```

This recomputes all identity digests. A v1 JSON contract fails closed because
current main accepts schema v2 only.

## Exact Codex config rendering

Render the exact future user-level provider TOML without contacting the relay:

```sh
eng relay-render-codex-config \
  --contract relay-contract-v2.json \
  --out relay.config.toml
```

The output file is created with exclusive-create semantics and mode 0600. An
existing path is never overwritten. The command reports:

- contract digest;
- renderer contract version;
- Codex config digest;
- provider config digest;
- output path.

The rendered file contains only non-secret configuration and credential
locators. It is not an authentication artifact.

## Fail-closed boundaries

Repository prequalification rejects:

- built-in/reserved Codex provider IDs;
- credential-bearing/query/fragment provider URLs;
- HTTP without the exact private-IP exception;
- non-Responses wire mode;
- relay credential locators reusing `OPENAI_*` or `CODEX_*` variables;
- ambiguous env-key + command auth;
- command auth without an explicit bounded timeout;
- scheduled command-token refresh in the initial posture;
- automatic request/stream retries;
- WebSocket or standalone web-search support;
- missing gateway/privacy/model-mapping digests.

Changing renderer bytes changes `codex_config_digest` and therefore changes
`provider_config_digest`, even when the high-level Contract is otherwise the
same.

## Deferred live gate

No account or credential validation is required in the current phase.

Before `company-relay` can be added to `internal/provideridentity`, #106 still
requires one concrete live qualification for the exact v2 provider config
digest that proves:

1. the exact rendered user-level Codex provider configuration was used;
2. the relay accepts the intended credential;
3. exactly one bounded read-only, no-tool model turn succeeds;
4. requested and effective upstream/model identity are retained, or the lane is
   rejected when the gateway cannot prove them;
5. gateway operation/request correlation is retained;
6. transformation, retention and model-mapping behavior matches the archived
   policy digests;
7. timeout/ambiguous outcomes enter Recovery/reconciliation rather than blind
   model-turn replay.

Only after that evidence exists may a separate change admit one exact relay
Provider v3 identity. A future unattended relay identity still requires its own
qualification before it can satisfy #105.
