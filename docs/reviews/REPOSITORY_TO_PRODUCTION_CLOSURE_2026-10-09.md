# Repository-to-production closure gates — 2026-10-09

Status: **REVIEW / NON-AUTHORITATIVE CHECKLIST**. This note must never override [Implementation Status](../status/IMPLEMENTATION_STATUS.md), Core authority, or [Production Terminal Acceptance](../implementation/PRODUCTION_TERMINAL_ACCEPTANCE_V1.md).

## Snapshot and evidence boundary

- Reviewed main: `37dc1ca07ec8a4e6db922fd0de140a7e70178270`.
- Exact-main CI: [37906512639](https://github.com/jiying2007/engineering-platform/actions/runs/37906512639), five successful jobs.
- Retained-ref drift watch: [37911703609](https://github.com/jiying2007/engineering-platform/actions/runs/37911703609), successful.
- GitHub open issues: [#103](https://github.com/jiying2007/engineering-platform/issues/103), [#105](https://github.com/jiying2007/engineering-platform/issues/105), [#106](https://github.com/jiying2007/engineering-platform/issues/106).
- The real historical M1 Feature and Debug chains qualify **only** their retained subjects and trusted self-hosted lane, not the current production environment.
- CI success, source hashes, pre-live fixtures, endpoint health, account lists, and documentation do **not** constitute production READY or engineering-effectiveness proof.

## Workstream A — repository-side safety and regression

For every new Publisher, Worker, Recovery or external-operation change:
1. Record immutable source SHA, exact relevant file/contract diffs, and an explicit negative-case matrix.
2. Exercise timeout-after-effect, timeout-before-effect, HTTP redirects/proxy influence, branch disappearance or PR retargeting, stale credentials and replay-after-UNKNOWN. Do not claim a case covered unless a named reproducible test verifies it.
3. Verify single authoritative settlement and independent readback; conflicting observations must remain UNKNOWN/MANUAL rather than silently issuing a second effect.
4. Require passing exact-PR-head CI and post-merge exact-main CI; record links and job conclusions.
5. If a test cannot be executed without a real provider, classify it `EXTERNAL_NOT_QUALIFIED`, never `PASS`.

These are review requirements, not a claim that every case is currently covered.

## Workstream B — conservative hygiene

- Inventory all non-main refs, open PRs and retained references at one fixed source SHA.
- `engineering-platform/*` and retained-evidence/prototype refs are not generic cleanup candidates.
- Delete only an explicitly authorized ref with exact head and integrated-tree proof under the existing `retire_integrated_branches.py` protected-main workflow and its readback. Do not run unaudited bulk branch deletion.
- Do not remove old Profile/Task/Prompt/Evidence readers while any immutable historical receipt depends on them. Unused compatibility must be demonstrated by consumer and reference searches, not inferred from a version suffix.
- Prefer one current status document and links to historical studies over creating another implementation-status authority.

## Workstream C — embedded engineering effectiveness

Qualify existing agent-dev-kit Skill/method assets by **task results**, not catalog presence. Keep the engineering-platform typed routing and prompt/contract digests; do not introduce a parallel Skill authority.

Select bounded real Linux/BSP, MCU/RTOS, driver integration and Debug tasks. For each, freeze target/platform, source base, material inputs, expected tests, the non-AI baseline and acceptance criteria *before* execution. Retain: exact changes, CI/host/board evidence type, reviewer decision, manual interventions, defects/rollback, elapsed wall time, worker CPU/RAM/disk and provider token/cost if available. Record failure and missing board proof explicitly. Report per-class sample sizes and confidence limitations. Only independent repeated results can promote Skill maturity or prove cost savings.

## Workstream D — unattended provider

Select exactly one explicit credential/provider lane; [#103](https://github.com/jiying2007/engineering-platform/issues/103) WIF or a separately justified [#106](https://github.com/jiying2007/engineering-platform/issues/106) relay. The optional lane is **not** an automatic fallback.

External administrator credentials/identity, live model proof, exact effective model and configuration, expiry/rotation, emergency revocation, privacy policy, and UNKNOWN recovery are not reproducible from mocks. Never emit `QUALIFIED` without independently retained live evidence.

## Workstream E — production terminal

[#105](https://github.com/jiying2007/engineering-platform/issues/105) is blocked until at least one unattended lane is qualified. Continue the frozen terminal acceptance contract; do not add a second workflow authority.

Require production-host service identities, isolated credentials, exact canary deployment, observed end-to-end service readiness, backup/restore, publisher/provider kill switches, exact-head trusted CI, immutable Evidence, independent human Review, Recovery and source-verified calibrated SLOs. An API 200 or green main CI alone must not set `READY`.

## Closure policy

| Gate | Current defensible state | Promotion evidence |
| --- | --- | --- |
| Protected-main regression | PASS for cited main only | Fresh exact-head + exact-main checks after changes |
| Historical M1 Feature/Debug | PROVEN for retained subjects only | No retrospective reinterpretation |
| Current Publisher/Worker fault combinations | INCOMPLETE / case-specific | Named negative tests and traceable artifacts |
| Branch/compatibility residue | NOT FULLY AUDITED | Reference inventory + exact safe retirement receipts |
| Embedded Skill effectiveness | DEFINED, NOT PROVEN | Independent repeated real task comparisons |
| Unattended provider | EXTERNAL_NOT_QUALIFIED | Real authenticated provider execution and credential controls |
| Production READY | NOT QUALIFIED | Entire #105 live terminal acceptance |

**Fail-closed rule:** Any missing, ambiguous, expired, or contradictory evidence preserves the gate as unqualified. This checklist grants no Runtime, Git, device, release, credential, verification, or closure authority.
