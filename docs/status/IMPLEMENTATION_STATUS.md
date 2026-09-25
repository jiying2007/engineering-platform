# Implementation Status

Reviewed base: `df5d69d2d2f822d90571c44526b313eb9a872090` (#35).
Stage: **Authenticated Core + actual preparation + bounded offline execution +
real Codex 0.155 app-server qualification gate; no authenticated model pilot**.

## Canonical scope

Embedded Core architecture, capability model, M0/M1 plan and ADR-001/002/003 remain
canonical. Independent of digital-worker; no service/domain catalogue expansion.

## Current assembled path

Work/Task/frozen Run -> atomic outbox/inbox relay -> mTLS preparation Worker ->
approved Context bytes and independent Git snapshot -> exact Core execution
authorization -> inspected bounded offline container -> retained execution receipt.

This change adds the real Codex compatibility gate beside that path: exact native
0.155.0 binary -> raw-byte pin -> generated stable/experimental schema checks ->
fresh stdio app-server -> real initialize + ephemeral thread/start. It does NOT
yet route a Worker Run into a model turn.

All receipt kinds retain separate meanings: INPUT_VALIDATED,
WORKER_ATTESTED_PREPARATION and WORKER_ATTESTED_OFFLINE_EXECUTION. A process exit
or Codex protocol qualification does not complete an engineering Run or create
Evidence/Verification/Delivery/Closure automatically.

## Capabilities and remaining limits

| Area | Implemented | Remaining boundary |
| --- | --- | --- |
| Core persistence | Immutable Task/RunInput identities, Run/Session epochs, PostgreSQL business/audit/outbox and durable Worker receipts | Memory-store deep-copy/conformance; independent Review |
| Worker admission/preparation | Real relay, mTLS identities/profiles and leases; approved source/context preparation | Approval snapshots require restart; no online enrollment/tenant ACL |
| Workspace | Independent exact-base Git objects, sanitized trusted Git, ownership-safe slots and byte checks | Trusted source metadata/host parents; quota/retention |
| Offline execution | Exact one-shot authorization; live Run/recovery/lease checks; pinned image+guard; real constrained container; bounded output and retained result | Offline/read-only only; trusted Worker/daemon/kernel; no VM/rootless qualification |
| Codex protocol | Bounded JSONL client and typed lifecycle; current stable wire values; deny-only approvals | Real model turn and interactive approval lane not yet assembled |
| Codex qualification (this change) | Exact `codex-cli 0.155.0`; native binary hash; stable+experimental generated schema digests; real fresh-process initialize/thread-start; retained CI artifact | Provider credentials/network policy and real model-output qualification |
| Action/recovery | Durable action ledger, authenticated grants, independent recovery-completion gate | Real privileged provider/effect boundary and completion verifier |
| Engineering delivery | Frozen verification plus Delivery/Evidence/Closure contracts | Actual Git/CI/Artifact facts, independent Review and Feature/Debug pilots |

## Retained evidence

- #33 admission: main `71a4877e2eb4973196cd612852fba19277919197`, PR
  `36031414023`, fresh main `36031860646` passed.
- #34 preparation: main `2ff3de8d0520a84573a59d72cd460bf0492534af`, PR
  `36075559599`, fresh main `36075797271` passed.
- #35 offline execution: main `df5d69d2d2f822d90571c44526b313eb9a872090`;
  PR `36084019664` and fresh main `36084270695` passed both normal Go/PG
  regression and mandatory real-container jobs.
- This Codex change requires exact PR-head normal CI plus
  `codex-app-server-0.155.0-qualification`, then fresh-main copies of both.
  No helper-process test or earlier green checkpoint substitutes for them.

The real Codex job installs exactly `@openai/codex@0.155.0`, locates its native
platform executable, verifies the reported version, hashes the executable,
generates both stable and experimental app-server JSON schemas, validates the
stable protocol fields used by this repository, and runs a fresh no-credential
stdio app-server through initialize and ephemeral thread/start. It uploads the
machine-readable qualification receipt and npm integrity value.

The adapter no longer depends on stale helper-only values: thread/start uses
`sandbox=read-only`, `approvalPolicy=never`, `ephemeral=true`; provider launch
uses `app-server --stdio`. Managed daemon/proxy and per-thread config overrides
are intentionally excluded from this path.

## Deployment and next acceptance gates

No source merge deploys a service, changes production data, provisions credentials
or installs a production Codex binary. Operators must install and review the
qualified native binary and bind the exact raw-byte digest before launch.

Next implementation sequence:

1. consume the qualified binary identity in the existing Worker/Core execution
   reservation and perform a real authenticated Codex turn under explicit egress;
2. route every interactive command/file/network approval through current
   Action Gateway authority, never through provider self-approval;
3. retain exact model/protocol/output receipt plus Git diff, CI and Artifact-byte
   facts, then independent Review;
4. close UNKNOWN/reconciliation and restore drills;
5. retain one real Feature pilot and one real Debug pilot before assessing M1.

No new extension domains. **M1 and production readiness remain unclaimed.**

Contracts:
- `docs/implementation/CODEX_QUALIFICATION_V1.md`
- `docs/implementation/OFFLINE_EXECUTION_V1.md`
- `docs/implementation/WORKER_PREPARATION_V1.md`
- `docs/implementation/WORKER_ADMISSION_V1.md`
- `docs/implementation/PREPARED_RUNTIME_PROTOCOL_V1.md`
- `docs/implementation/AUTHENTICATED_CONTROL_API_V1.md`
- `docs/implementation/OUTBOX_DISPATCH_AUTHORITY_V2.md`
