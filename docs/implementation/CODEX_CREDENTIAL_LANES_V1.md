# Codex credential lanes v1

Date: 2026-09-29

Status: **CURRENT AUTHENTICATION / PROVIDER POLICY**

This document defines which Codex credential/provider lanes are allowed for the
Engineering Platform and what each lane proves.

It does not create a new Runtime authority model. All lanes still feed the same
Core / Worker / Action Gateway / Evidence / Verification / Review / Recovery
chain.

## 1. Default internal development lane

The current default for the R&D Ubuntu development environment is:

```text
trusted Ubuntu host
  -> ChatGPT-authenticated Codex CLI
  -> compatibility-qualified exact Codex binary/profile
  -> isolated Codex HOME
  -> saved-login bootstrap removed before model-reachable work
  -> Core-bound engineering execution
```

This is the **trusted self-hosted saved-ChatGPT-login lane**.

It is already proven by the retained Feature and Debug M1 Closure chains.

The current AiotServer01 deployment may use a ChatGPT Pro-authenticated Codex
CLI. A specific ChatGPT plan is not part of the authority contract; the contract
is the successfully authenticated Codex CLI plus the repository's exact
compatibility qualification, isolation and retained execution evidence.

Use this lane for:

- interactive/internal engineering on trusted Ubuntu servers;
- small-team self-hosted automation where the Linux account/host is explicitly
  trusted;
- retained engineering using one-job ephemeral repository-scoped runners.

This lane does **not** claim a machine/workload identity suitable for arbitrary
unattended infrastructure.

## 2. Managed-workspace WIF lane

Managed-workspace WIF is an **optional unattended/GitHub-hosted enhancement**,
not a prerequisite for normal internal development and not a prerequisite for
the already-proven trusted self-hosted M1 phase 1.

It replaces personal saved-login state with a workload identity:

```text
GitHub Actions OIDC
  -> exact managed-workspace federation rule
  -> short-lived Codex authentication
  -> compatibility-qualified exact Codex binary/profile
  -> same Core-bound execution chain
```

Use this lane when:

- no developer should remain logged in on the execution host;
- GitHub-hosted or other workload-identity execution is required;
- credential lifecycle should be managed centrally by workspace administrators;
- repository/ref/workflow/audience claims need server-side workload fencing.

Issue #103 tracks the first live qualification.

WIF failure must fail closed. It must never silently fall back to a saved
ChatGPT login, API key or relay.

## 3. Company relay / model gateway lane

A company relay, API proxy or multi-model gateway is a **separate provider
lane**, not a variant of WIF and not a fallback credential.

It may be useful for:

- central model/provider routing;
- OpenAI / Anthropic / other-provider policy;
- company-level usage accounting;
- regional egress or audit controls;
- cost/availability policy.

Before admission, a relay implementation must define and prove at least:

- exact upstream provider/model identity semantics;
- request/response integrity and whether the relay mutates prompts or outputs;
- credential ownership and rotation;
- retention/privacy/logging behavior;
- retry and UNKNOWN semantics;
- tool/app-server compatibility;
- rate-limit/error translation;
- audit correlation back to the exact Engineering Platform Run;
- no hidden downgrade to an unqualified model/provider.

A relay must receive its own compatibility/provider qualification and retained
provenance. It must not reuse a WIF receipt, saved-login receipt or exact-model
claim that it cannot independently prove.

## 4. Credential selection is explicit

Exactly one credential/provider lane is selected for a Worker execution.

Allowed high-level modes are:

| Lane | Current status | Default use |
| --- | --- | --- |
| trusted self-hosted saved ChatGPT login | **PROVEN / DEFAULT INTERNAL** | Ubuntu R&D development and trusted self-hosted execution |
| managed-workspace WIF | **READY, EXTERNAL ADMIN QUALIFICATION PENDING** | unattended/GitHub-hosted workload identity |
| company relay / model gateway | **NOT YET QUALIFIED** | optional future multi-provider/company-governed execution |

Selection must be explicit in configuration and evidence. Runtime must not
"try the next credential" after an authentication failure.

## 5. Provider-neutral Core boundary

The Core task/evidence semantics do not depend on how the model authenticated.

Every admitted lane must still preserve:

- exact Task / Run / execution epoch;
- exact approved Context;
- compatibility-qualified runtime/provider identity;
- no publisher credential in the model process;
- retained result commit/bundle and provider execution receipt;
- Action Gateway authority for external mutation;
- exact PR-head CI;
- requirement-bound Evidence;
- independent Verification and Review;
- fail-closed recovery/reconciliation.

Changing provider authentication does not authorize changing those boundaries.

## 6. Current product posture

Current product posture is deliberately three-tiered:

1. **Internal engineering:** trusted self-hosted ChatGPT-authenticated Codex CLI
   is the default and already proven.
2. **Unattended automation:** qualify at least one explicit machine credential
   lane. Managed-workspace WIF is currently the most developed candidate.
3. **Multi-provider/company gateway:** evaluate separately only when there is a
   real organizational requirement. Do not add it merely as a workaround for a
   missing WIF administrator action.

Production readiness remains unclaimed until the selected unattended credential
lane and the operational acceptance criteria in issue #105 have retained
evidence.
