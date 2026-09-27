# Implementation Status

Reviewed base: `6f29464ab0cad9261fe6d3827c283339b51cd72b` (#74).
Stage: **Retained M1 execution mechanics remain one authority chain, but Codex
authentication is no longer WIF-only. M1 phase 1 supports trusted self-hosted
ChatGPT-authenticated Codex on an explicitly trusted Linux host, with the saved
login copied only into a fresh isolated Codex HOME for auth prewarm and deleted
before model-reachable work. Managed-workspace WIF remains the preferred
GitHub-hosted/unattended authentication lane. GitHub main protection is now
actually active. No real retained Feature/Debug Closure chain has completed, so
M1 and production readiness remain unclaimed.**

## Canonical scope

Embedded Core architecture, capability model, M0/M1 plan and ADR-001/002/003
remain canonical. This repository is independent of digital-worker. No new
extension domain, Runtime lane, Evidence family or Recovery mechanism is being
added for the publication slice.

## Status authority

This file is the single live implementation-status document. Superseded
architecture-profile revisions, M0 adoption-plan revisions and transient
`CI_VALIDATION_*` checkpoint markers are retained in Git history instead of the
working tree. GitHub Actions runs/artifacts remain the authority for CI evidence;
this document summarizes them and does not replace retained receipts.

## Current assembled path

```
Requirement / Work / frozen Task
  -> frozen Run + approved Context
  -> mTLS Worker admission/preparation
  -> independent exact-base workspace
  -> Core-bound Codex engineering execution
       (trusted self-hosted saved login OR managed-workspace WIF)
  -> retained result commit + verified Git bundle
  -> Action Gateway publication authority
  -> independent GitHub publisher
  -> target-policy check + non-force branch + PR
  -> exact PR-head trusted CI
  -> Delivery + Codex/Git/CI Evidence
  -> Verification
  -> independent human Review
  -> conditional Closure
```

#49 moved Codex beyond compatibility qualification: the existing Core
reservation now authorizes one bounded engineering turn and retains
`WORKER_ATTESTED_CODEX_EXECUTION` plus exact changed-tree and Git-bundle facts.
Repository fake app-server tests prove protocol/filesystem behavior only. They
remain explicitly non-authoritative for a live managed-workspace model claim.

#50 adds the Git/PR publication adapter under the existing Action Gateway. The
Worker and model process still receive no GitHub write credential. Publication
is derived from the FINISHED Codex receipt, frozen Task repository/base and
operator-owned target policy; caller-supplied repository/ref/commit/bundle
locators are not accepted.

#64-#66 assemble the real retained-pilot operator path without creating a second
authority model. The protected-main engineer workflow performs one real WIF
qualification and one Core-bound engineering turn, snapshots the non-replayable
FINISHED model state, and only then exposes a separately scoped GitHub
publication token. The verification workflow resumes the exact frozen Core and
imports Codex/Git/CI Evidence before Core computes Verification. The final
workflow requires an explicit PASS/FAIL decision from a GitHub actor different
from the engineering dispatcher; only PASS allows the separate closure identity
to create ClosureReceipt.

## Capabilities and remaining limits

| Area | Implemented | Remaining boundary |
| --- | --- | --- |
| Core persistence | Immutable Task/RunInput identities, Run/Session epochs, PostgreSQL business/audit/outbox, durable Worker/Codex receipts and persisted Review authority | Broader memory-store deep-copy/conformance is non-M1 infrastructure work |
| Worker admission/preparation | Real relay, mTLS identities/profiles and leases; approved source/context preparation | Approval snapshots remain operator-managed |
| Workspace | Independent exact-base Git objects, sanitized trusted Git, ownership-safe slots, byte checks and result finalization | Trusted host/source boundary; operational quota/retention |
| Offline execution | Exact one-shot authority, live Run/recovery/lease checks, constrained container and retained receipt | Offline/read-only lane remains separate from engineering model execution |
| Codex qualification | Exact `codex-cli 0.155.0`, native hash, generated schema digests, real app-server protocol qualification | Production binary installation/pin remains operator-owned |
| Core-bound Codex engineering | One reserved engineering turn with a frozen prompt identity and the same retained result chain. Credential mode is explicit and mutually exclusive: trusted self-hosted saved ChatGPT login or managed-workspace WIF. Saved-login mode uses a fresh isolated HOME and deletes the bootstrap auth copy before thread/model work; WIF mode removes the upstream assertion before thread/model work. | Trusted self-hosted mode treats the Linux account/host as trusted and is the M1 phase-1 lane; WIF remains the preferred unattended/GitHub-hosted upgrade. |
| Git/PR publication | Existing Action Gateway ledger; exact Codex result-digest binding; operator target policy; bundle re-hash/verify; frozen-base ancestry; deterministic non-force branch; create/update exact PR; structured operation receipt; observation-only UNKNOWN reconciliation | Requires separately provisioned publisher credential and shared read-only retained-artifact view |
| Recovery | UNKNOWN reconciliation states, separated reconciler/completer identities, database-generated epoch proof, PG17 restore drill | No new publication-specific Recovery subsystem is required |
| Engineering delivery | Frozen requirement-bound Delivery/Evidence/Verification; exact PR-head CI, Git and Codex importers; separate verifier/reviewer/closure identities; GitHub-hosted resumable engineering -> verification -> independent-review workflows | One retained real Feature pilot and one retained real Debug pilot are still required |
| Pilot operations | Renderable Work/Task/Run/post-model templates; frozen requirement/reproduction ContextRefs; local mTLS/PostgreSQL stack; `eng pilot-preflight`; trusted self-hosted saved-login and managed WIF authentication variants; resumable engineering artifacts consumed by the same Verification/Review/Closure chain | M1 phase 1 requires one dedicated repository-scoped self-hosted runner plus real Feature/Debug retained Closure chains; managed-workspace WIF remains a future unattended-path prerequisite, not a phase-1 blocker |

## Retained evidence

- #33 admission: main `71a4877e2eb4973196cd612852fba19277919197`, PR
  `36031414023`, fresh-main `36031860646` passed.
- #34 preparation: main `2ff3de8d0520a84573a59d72cd460bf0492534af`, PR
  `36075559599`, fresh-main `36075797271` passed.
- #35 offline execution: main `df5d69d2d2f822d90571c44526b313eb9a872090`;
  PR `36084019664`, fresh-main `36084270695` passed.
- #36 real Codex qualification: main `810d01c7e1c89efe22aa7c3c99bcbd2398b35380`;
  PR `36088616108`, fresh-main `36093348940` passed.
- #37 trusted CI provenance: main `c817876c8c10fddebe21fc38d394fee944f66c47`;
  PR `36095060443`, fresh-main `36095388934` passed.
- #40 bounded container cold-start hardening: main
  `d3bb6d591ee4b8b70e137d62d85fa3384109325e`; exact-head `36101425449`,
  fresh-main `36101773459` passed.
- #42 WIF assertion-removal hardening: main
  `f8e9e60a517fed752d573dd7b27ed38a49175e33`; fresh-main `36102881734`
  passed.
- #43 exact Evidence requirement binding: main
  `e895fc6dddb509a51897b5fe32b21216312ebb9d`; exact-head `36108017341`,
  fresh-main `36108399745` passed.
- #44 trusted CI import: main `34ff7d82316d1d095040ce6f62522fe5397dee86`;
  exact-head `36112719171`, fresh-main `36113188028` passed.
- #46 independent Review: main `747114948bfa35cf07cafcebdf1e32b7e0341047`;
  exact-head `36129118788`, fresh-main `36129501979` passed.
- #47 recovery/restore authority: main
  `e66cb4bee39ab9b876da6e2e420ebb3085b02ba0`; exact-head `36135297468`,
  fresh-main `36135713586` passed all five gates.
- #48 engineering evidence bridge: main
  `f5af0ee88bf283213424839b6a28db961a4478db`; exact-head `36200540315`,
  fresh-main `36200839712` passed all five gates.
- #49 Core-bound Codex engineering execution: main
  `713aa1f25f0e6ea513674d53343b9fc1c9787843`; exact-head `36213531198`,
  fresh-main `36213786919` passed all five gates. This establishes the
  execution/result-bundle chain, not a retained real managed-workspace model
  proof because WIF administrator configuration is still external.
- #50 independent Git/PR publication adapter: final exact-head run `36215624193`
  and fresh-main run `36215857974` passed all five gates. Main became
  `9c3e15fffcf75b20a56576e623f29b8976381879`.
- #51 exact PR-head trusted CI: main
  `316bba9fab02f9cdbbf0278cf89a21a25eea10fd`; exact-head run `36219337024`
  and fresh-main run `36219687449` passed all five gates. Pull-request CI now
  checks out and retains artifacts for the exact PR head/result commit. The
  trusted importer accepts that mode only for a same-repository PR on the exact
  frozen base and only when `.github/workflows/ci.yml` has the identical Git
  blob at base and result. Fork, merge-SHA, base, workflow and live-head drift
  fail closed.
- #52 retained-pilot readiness: exact-head `36226872470`, fresh-main
  `36227173710` passed all five gates; exact Codex profile generation and
  Feature/Debug runbook are retained.
- #56 pre-WIF pilot pack: fresh-main `36235946636` passed; Work/Task/Run,
  least-privilege access policy and Worker preparation inputs dry-run to RUNNING.
- #58 post-model pack: exact-head `36237460187`, fresh-main `36237727348`
  passed; publication/Delivery/Verification/Closure request shapes dry-run
  through Core while PASS Review remains intentionally untamplated.
- #59 local/deployment preflight: exact-head `36240815798`, fresh-main
  `36241078607` passed; `eng pilot-preflight` separates internal readiness
  from external WIF/publisher blockers.
- #60 local pilot stack plus #61 WIF handoff: local mTLS identities/PKI,
  PostgreSQL helper, admin-to-live qualification helper and receipt/config
  rendering are tested; fresh-main through `36243410207` passed.
- #62 publisher artifact-view binding: main
  `34f3be138f8fe709099cb6caa3150305adb97671`, fresh-main `36245881737`
  passed. Publisher must consume the Worker's exact retained
  `preparation-root/artifacts` view.
- #63 frozen real requirement context: main
  `8741a6e818a767f818348ab9082c13315f8faf23`, fresh-main `36246559361`
  passed. Feature #54 and Debug #55 requirement/reproduction bytes are approved
  content-addressed ContextRefs available to no-network Codex.
- #64 protected-main retained engineering: main
  `4c2254bd2ff3b1c72fd7819743457bcced8ff109`, exact-head `36247408099`,
  fresh-main `36248358841` passed. Model/WIF phase is credential-separated
  from the later Action Gateway publisher.
- #65 retained verification: main
  `a710bcf0662e4281ec887314b0f28950db8465b7`, exact-head `36248775774`
  passed all five gates; post-merge `36249081304` passed. It restores frozen
  Core, verifies exact PR-head CI artifacts/live facts and creates
  requirement-bound Codex/Git/CI Evidence plus Verification without Review.
- #66 independent Review/conditional Closure: main
  `9c385ac415e063c9c16e1b8b8e882a9c79bdcb9c`; exact-head `36249306005`
  passed all five gates. Review result remains explicit human PASS/FAIL input,
  the reviewer GitHub actor must differ from the engineering dispatcher, and
  only PASS permits the separate closure identity to close Work.
- #67 aligned the canonical implementation status with the complete retained
  execution workflow set.
- #68 managed-workspace WIF Admin API provisioning helper: PR exact-head
  `36252504615` and fresh-main `36252808263` passed all five gates. At that
  implementation checkpoint main was `973bf27daad5b546fe4fdd4d6c62d2d6cce17443`. The helper creates/reuses
  exact GitHub OIDC provider/rule policy and can hand off the two non-secret
  repository variables without exposing the Admin API key to Runtime/Worker.
- #69 canonical live-tree cleanup: exact-head `36259229901`, fresh-main
  `36259541528` passed all five gates. Superseded architecture/adoption
  snapshots and transient CI markers now remain in Git history instead of the
  live authority/search surface.
- #70 real WIF provider/token contract hardening: exact-head `36278545554`,
  fresh-main `36278796891` passed all five gates. Provider creation now matches
  the Admin API schema, alternate provider trust sources fail closed, and M1
  engineering no longer assumes a 15-minute credential window.
- #71 GitHub OIDC immutable identity/upstream-lifetime fencing: exact-head
  `36283137777`, fresh-main `36283385395` passed all five gates. Both live
  qualification and retained engineering validate immutable repository IDs and
  exact workflow identity without depending on the legacy subject format;
  Runtime deadlines derive from actual JWT expiry.
- #72 OpenAI server-side immutable federation claims: exact-head `36288357165`,
  fresh-main `36288625934` passed all five gates. The OpenAI rule itself
  requires exact repository name, immutable repository/owner IDs,
  `workflow_dispatch`, exact `refs/heads/main`, dedicated audience and the
  approved workflow-ref allow-list. GitHub branch-protection state is not an
  OIDC token claim and must be enforced separately by repository protection plus
  the workflow runtime `GITHUB_REF_PROTECTED` fact.

## Publication authority

The publication contract is `docs/implementation/GITHUB_PR_PUBLICATION_V1.md`.
A publication request must be authorized by all of:

1. frozen Task `allowed_actions` contains `github.publish-pr`;
2. the requesting authenticated principal has the exact
   `github.publish-pr / CONTROLLED_MUTATION / github.publish-pr` grant;
3. operator publisher policy permits the exact Task repository/base ref; and
4. action `parameters_digest` equals the retained FINISHED Codex
   `result_digest`.

The publisher re-hashes the retained bundle, verifies the frozen target base,
validates the bundle/result commit and ancestry in an isolated Git repository,
uses a deterministic non-force branch, and retains a structured PR operation
receipt. UNKNOWN reconciliation observes GitHub state only and never replays a
push or PR mutation.

## Deployment and next acceptance gates

GitHub main protection is now a live external fact: main is protected and the
five canonical CI checks are required. Source merge still does not provision a
publisher credential, a managed-workspace WIF rule, or a production service.

M1 phase 1 no longer requires WIF. The next retained proof sequence is:

1. Register/enable a repository-scoped trusted self-hosted Linux runner for
   `jiying2007/engineering-platform` with label
   `engineering-platform-codex`. Require exact `codex-cli 0.155.0`,
   `codex login status` = `Logged in using ChatGPT`, working Docker/Go, and
   an authenticated host `gh` publisher credential.
2. Dispatch `retained-pilot-self-hosted-engineer.yml` for Feature #54 from
   protected main. The workflow must freeze the exact base, execute one
   Core-bound saved-login Codex turn, retain FINISHED state/result bundle, stop
   the model process, and only then materialize the host GitHub publisher token
   for the existing Action Gateway.
3. Require publication CONFIRMED and exact PR-head CI PASS.
4. Dispatch `retained-pilot-verify.yml` with the self-hosted engineering run
   ID; require exact Codex/Git/CI Evidence plus PASS Verification.
5. A GitHub actor different from the engineering dispatcher performs the
   existing independent Review. PASS alone permits Closure.
6. Disable/remove the dedicated runner after the pilot window.
7. Repeat the complete chain for Debug #55 from the then-current protected main
   with new Work/Task/Run/Evidence/Review/Closure identities.
8. Only after both real ClosureReceipts exist should M1 be reassessed.

`examples/pilots/self-hosted/qualify-login.sh` remains an optional diagnostic
read-only model probe. It is not a prerequisite for phase-1 retained engineering,
so the saved ChatGPT login does not need to be consumed/refreshed twice before
the non-replayable retained turn.

The existing managed-workspace WIF/Admin API/GitHub-hosted workflow remains
supported as the unattended execution lane. It is no longer an M1 phase-1 hard
blocker and must not be represented as qualified until a real WIF turn exists.

Trusted self-hosted mode has an explicit known limit: the host/account itself is
trusted, so unrelated credentials elsewhere on that account are outside the
Codex child-process isolation boundary. The model process receives neither the
operator HOME nor publisher/API/WIF credential environment, and the isolated
saved-login bootstrap is deleted before thread/start. Production/unattended
claims should prefer WIF or a separately reviewed stronger host-isolation design.

At this checkpoint there are still **no real retained Feature or Debug Closure
chains**. Repository fixtures/fake app-server tests remain protocol/filesystem
evidence only.

**M1 and production readiness remain unclaimed.**

Contracts:
- `docs/implementation/RETAINED_PILOT_RUNBOOK_V1.md`
- `docs/implementation/GITHUB_PR_PUBLICATION_V1.md`
- `docs/implementation/CORE_BOUND_CODEX_EXECUTION_V1.md`
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
