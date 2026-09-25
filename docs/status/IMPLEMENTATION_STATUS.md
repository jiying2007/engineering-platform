# Implementation Status

Reviewed base: `e66cb4bee39ab9b876da6e2e420ebb3085b02ba0` (#47).
Stage: **Authenticated Core + actual preparation + bounded offline execution +
real Codex 0.155 qualification + retained Git/CI/artifact provenance + exact requirement-bound trusted CI import + mandatory independent Review + database-generated recovery proof/restore drill + Worker/Git engineering evidence bridge; WIF-ready live-turn lane, no retained authenticated model-turn proof**.

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
| Core persistence | Immutable Task/RunInput identities, Run/Session epochs, PostgreSQL business/audit/outbox, durable Worker receipts and persisted ReviewReport authority | Broader memory-store deep-copy/conformance |
| Worker admission/preparation | Real relay, mTLS identities/profiles and leases; approved source/context preparation | Approval snapshots require restart; no online enrollment/tenant ACL |
| Workspace | Independent exact-base Git objects, sanitized trusted Git, ownership-safe slots and byte checks | Trusted source metadata/host parents; quota/retention |
| Offline execution | Exact one-shot authorization; live Run/recovery/lease checks; pinned image+guard; real constrained container; bounded output and retained result | Offline/read-only only; trusted Worker/daemon/kernel; no VM/rootless qualification |
| Codex protocol | Bounded JSONL client and typed lifecycle; current stable wire values; deny-only approvals | Interactive Action-Gateway approval lane not yet assembled |
| Codex qualification | Exact `codex-cli 0.155.0`; native binary hash; stable+experimental generated schema digests; real fresh-process initialize/thread-start; retained CI artifact | Production/runtime pin lifecycle remains operator-owned |
| Codex WIF live lane | Fail-closed WIF env/token-file boundary; authenticated prewarm exchange; upstream assertion must be deleted before thread/start; GitHub OIDC manual workflow; exact read-only one-turn observer and deterministic receipt | Workspace admin must enable/configure Codex WIF and a real manual run must succeed before any live-model proof claim |
| Action/recovery | Durable action ledger; UNKNOWN reconciliation states; dedicated reconciler/completer identities; database-generated epoch proof; production RecoveryCompletionGate; transaction revalidation; mandatory PG17 dump/restore authority drill | Real privileged provider/effect observer/reconciler remains provider-specific |
| Engineering delivery | Frozen requirement-bound Evidence/Verification/Delivery; trusted-CI importer; dedicated Worker execution importer; independent exact Git changed-tree importer; authenticated ReviewReport; Closure requires exact PASS Verification + PASS Review | Real engineering model-turn evidence and retained Feature/Debug pilots |

## Retained evidence

- #33 admission: main `71a4877e2eb4973196cd612852fba19277919197`, PR
  `36031414023`, fresh main `36031860646` passed.
- #34 preparation: main `2ff3de8d0520a84573a59d72cd460bf0492534af`, PR
  `36075559599`, fresh main `36075797271` passed.
- #35 offline execution: main `df5d69d2d2f822d90571c44526b313eb9a872090`;
  PR `36084019664` and fresh main `36084270695` passed both normal Go/PG
  regression and mandatory real-container jobs.
- #36 real Codex qualification: main `810d01c7e1c89efe22aa7c3c99bcbd2398b35380`; PR run `36088616108` and fresh-main run `36093348940` passed `go`, real container and real Codex qualification jobs. PR qualification artifact `10844861935` retained exact native/schema digests.
- #37 trusted CI provenance: main `c817876c8c10fddebe21fc38d394fee944f66c47`; PR run `36095060443` and fresh-main run `36095388934` passed `go`, real container, real Codex and `trusted-ci-artifact-evidence`. Main retained binary artifact `10847815571`, Codex qualification `10847795311`, trusted evidence `10847531493`; main receipt digest `sha256:9db9637b40c0ce09c71348a9d17d7681fba1c8cdcb48e95b26b2b101cb6c6c01`.
- #38 WIF-ready lane merged as main `0d786e73f952412dda5853dc8be675451db6d549`; its first fresh-main run `36099892960` exposed a Docker cold-start timeout while Go and real Codex qualification passed.
- #40 bounded Docker cold-start hotfix merged as main `d3bb6d591ee4b8b70e137d62d85fa3384109325e`; exact-head run `36101425449` and fresh-main run `36101773459` passed Go/PostgreSQL, 3x real-container, real Codex and trusted CI evidence.
- #42 WIF assertion-removal hardening merged into main `f8e9e60a517fed752d573dd7b27ed38a49175e33`; fresh-main run `36102881734` passed all four standard gates. Upstream assertion exchange is completed before thread/start and the assertion file is removed before any model-reachable turn.
- #43 exact Evidence requirement binding merged as main `e895fc6dddb509a51897b5fe32b21216312ebb9d`; exact-head run `36108017341` and fresh-main run `36108399745` passed Go/PostgreSQL, real container, real Codex and trusted CI evidence. Evidence now requires exact frozen requirement ID and exact-delivery artifact binding.
- #44 trusted CI import merged as main `34ff7d82316d1d095040ce6f62522fe5397dee86`; exact-head run `36112719171` and fresh-main run `36113188028` passed all four standard gates. The dedicated importer revalidates live GitHub run/jobs/artifacts and downloaded ZIP/binary/Codex bytes before registering one exact requirement-bound PASS Evidence item.
- #46 independent Review merged as main `747114948bfa35cf07cafcebdf1e32b7e0341047`; exact-head run `36129118788` and fresh-main run `36129501979` passed Go/PostgreSQL, real container, real Codex and trusted CI evidence. Closure now requires an exact PASS ReviewReport from a certificate-separated reviewer.
- #47 recovery/restore authority merged as main `e66cb4bee39ab9b876da6e2e420ebb3085b02ba0`; exact-head run `36135297468` and fresh-main run `36135713586` passed all five gates: Go/PostgreSQL/race, real container, PostgreSQL 17 authority dump/restore, real Codex qualification and trusted CI evidence. Recovery completion now requires database-generated proof plus separate reconciler/completer identities.
- This WIF assertion-removal follow-up is implementation hardening only. A real authenticated-model claim still requires a successful manual `Codex WIF live qualification` receipt after managed-workspace WIF enablement/configuration.

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

1. administrator enables/configures Codex WIF and repository non-secret variables,
   then retain one successful manual WIF read-only model-turn receipt;
2. consume that qualified WIF/binary boundary inside the existing Worker/Core
   execution reservation instead of workflow-only qualification;
3. route every command/file/network approval through Action Gateway authority;
4. Worker execution and Git changed-tree evidence procedures are assembled; add real Core-bound model-turn Evidence only after a retained WIF engineering turn exists;
5. recovery proof/restore drill is assembled; wire a real provider-specific Action observer/reconciler only together with the first privileged provider;
6. retain one real Feature pilot and one real Debug pilot through Verification + Review + Closure before assessing M1.

No new extension domains. **M1 and production readiness remain unclaimed.**

Contracts:
- `docs/implementation/ENGINEERING_EVIDENCE_BRIDGE_V1.md`
- `docs/implementation/RECOVERY_RESTORE_V1.md`
- `docs/implementation/INDEPENDENT_REVIEW_V1.md`
- `docs/implementation/TRUSTED_CI_IMPORT_V1.md`
- `docs/implementation/CODEX_WIF_LIVE_V1.md`
- `docs/implementation/TRUSTED_CI_EVIDENCE_V1.md`
- `docs/implementation/CODEX_QUALIFICATION_V1.md`
- `docs/implementation/OFFLINE_EXECUTION_V1.md`
- `docs/implementation/WORKER_PREPARATION_V1.md`
- `docs/implementation/WORKER_ADMISSION_V1.md`
- `docs/implementation/PREPARED_RUNTIME_PROTOCOL_V1.md`
- `docs/implementation/AUTHENTICATED_CONTROL_API_V1.md`
- `docs/implementation/OUTBOX_DISPATCH_AUTHORITY_V2.md`
