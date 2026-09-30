# Implementation Status

Reviewed base: `6094e108932be79ae3a02a0d6e195f46d4110b05` (#108).
Stage: **Trusted self-hosted M1 phase 1 is proven by two real retained Closure
chains and remains the default internal R&D execution lane. Since #108, Codex
execution authority uses Profile v3: `provider_id / credential_mode /
execution_mode / provider_config_digest` are frozen into one provider identity
inside the Core-authorized Profile digest and are carried through Worker
execution, live/engineering receipts, publication state, Recovery, Verification
and Review/Closure state. The Worker no longer owns a second top-level
`credential_mode` authority. Current code admits only
`openai-codex + chatgpt-session + trusted-self-hosted` and
`openai-codex + workload-identity + unattended`; unknown providers and
cross-lane combinations fail closed. Managed-workspace WIF remains externally
unqualified in #103, and company relay/model gateway remains unqualified in
#106. Production/unattended readiness remains unclaimed.**

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
       (explicit credential/provider lane:
        trusted self-hosted saved login OR managed-workspace WIF
        OR a separately qualified future provider)
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
| Codex qualification | Version-independent compatibility contract: actual semver + native hash + generated schema digests + fixed-profile/app-server qualification; deterministic qualification digest plus exact Provider v3 identity frozen into the Profile digest | CI keeps a sentinel baseline, while each real Worker binary/provider combination must qualify and requalify before execution |
| Core-bound Codex engineering | One reserved engineering turn with a frozen prompt identity and retained result chain. Profile v3 freezes provider, credential, execution and provider-config digest as one capability identity. Worker config carries only credential material locators; it cannot override the Core-authorized provider identity. Saved-session mode uses a fresh isolated HOME and deletes the bootstrap auth copy before thread/model work; WIF mode removes the upstream assertion before thread/model work. | Trusted self-hosted mode treats the Linux account/host as trusted and is the default internal R&D lane. Unattended production must separately qualify one explicit machine/provider credential lane; WIF is one candidate and #106 tracks optional relay qualification. |
| Git/PR publication | Existing Action Gateway ledger; exact Codex result-digest binding; operator target policy; bundle re-hash/verify; frozen-base ancestry; deterministic non-force branch; create/update exact PR; structured operation receipt; observation-only UNKNOWN reconciliation | Requires separately provisioned publisher credential and shared read-only retained-artifact view |
| Recovery | UNKNOWN reconciliation states, separated reconciler/completer identities, database-generated epoch proof, PG17 restore drill | No new publication-specific Recovery subsystem is required |
| Engineering delivery | Frozen requirement-bound Delivery/Evidence/Verification; exact PR-head CI, Git and Codex importers; separate verifier/reviewer/closure identities; GitHub-hosted resumable engineering -> verification -> independent-review workflows | Trusted self-hosted M1 phase-1 proof is complete with one real Feature and one real Debug Closure chain; production rollout/SLO evidence remains outside this pilot proof |
| Pilot operations | Renderable Work/Task/Run/post-model templates; frozen requirement/reproduction ContextRefs; local mTLS/PostgreSQL stack; `eng pilot-preflight`; trusted self-hosted saved-login and managed WIF authentication variants; resumable engineering artifacts consumed by the same Verification/Review/Closure chain | Trusted self-hosted phase-1 pilots are complete and this is the default internal path. #103 tracks optional WIF qualification; #106 tracks optional company-relay qualification; #105 remains blocked until at least one unattended lane is independently qualified. |

## M1 phase-1 closure evidence

The two required real retained Closure chains are complete. The canonical proof
record is `docs/status/M1_RETAINED_PHASE1_CLOSURE_2026-09-29.md`.

- Feature: result `6009ea95785237ad6ff9f5c9cba911b4891dfa58`,
  Verification `36516020897`, Review `36517048600`, ClosureReceipt
  `m1-feature-routing-closure`, post-Closure promotion #100.
- Debug: reproduction `36518555837`, result
  `91d7d9fa068b7667bab5c211f13cd9e0151aeb92`, Verification
  `36536077026`, Review `36536314395`, ClosureReceipt
  `m1-debug-firmware-identity-closure`, post-Closure promotion #101.
- Closure/promotion code checkpoint: `ee0249fe25b860a83beba9fe06b7acdc80cd75c6`;
  fresh-main CI `36538546170` passed.
- Canonical status checkpoint after #102: protected main
  `644127255f615abb753fab5c450ea23caad76b11`; fresh-main CI
  `36540163771` passed all five canonical gates.
- Credential-policy checkpoint after #107: protected main
  `04169db0b69862e90e1d8ff90c093a4dc65fa9c4`; fresh-main CI
  `36565274419` passed all five canonical gates.
- Provider-identity authority checkpoint after #108: protected main
  `6094e108932be79ae3a02a0d6e195f46d4110b05`; fresh-main CI
  `36574110172` passed all five canonical gates.

## Credential/provider posture

The canonical credential/provider policy is
`docs/implementation/CODEX_CREDENTIAL_LANES_V1.md`.

Profile v3 makes provider selection part of execution authority rather than a
late runtime switch. Every admitted execution freezes:

- `provider_id`;
- `credential_mode`;
- `execution_mode`;
- `provider_config_digest`.

Current admitted combinations:

- **Default internal lane:** `openai-codex / chatgpt-session /
  trusted-self-hosted`. This lane is already proven by the retained
  Feature/Debug M1 Closure chains.
- **Optional unattended lane:** `openai-codex / workload-identity /
  unattended`. Repository mechanics are admitted, while real managed-workspace
  WIF qualification remains tracked by #103.
- **Optional future relay lane:** company relay/model gateway. No relay identity
  is currently admitted. Repository-side prequalification now validates and
  deterministically digests the proposed Codex custom-provider contract without
  using an account, credential or live model turn. #106 must still establish
  live provider qualification, model/upstream semantics, credential lifecycle,
  privacy/retention, retry/UNKNOWN behavior and retained provenance before any
  Provider v3 admission.

Authentication/provider failure is fail-closed. Runtime must not silently move
to another provider, credential mode or execution mode.

## Relay repository prequalification

Issue #106 now has a repository-only prequalification stage documented by
`docs/implementation/RELAY_PROVIDER_PREQUALIFICATION_V2.md`. Two CLI surfaces
are available:

- `eng relay-prequalification-pack` builds the canonical Contract + Assessment
  from non-secret provider parameters and three bounded policy/mapping files;
- `eng relay-prequalification --contract CONTRACT.json` strictly revalidates an
  already-materialized v2 contract;
- `eng relay-render-codex-config --contract CONTRACT.json --out CONFIG.toml`
  renders the exact future user-level Codex provider TOML without any network or
  credential operation;
- `eng relay-live-manifest --contract CONTRACT.json` freezes the later
  read-only/no-tool live qualification plan and receipt schema before any
  account or credential is supplied;
- `eng relay-runtime-handoff` binds that live manifest to one exact
  compatibility-qualified Codex executable/version/binary digest without
  starting a model turn;
- `eng relay-qualification-kit` materializes the normalized Contract, exact
  Codex TOML, live manifest, runtime handoff, compatibility receipt, cross-bound
  kit manifest and SHA256SUMS into one new owner-private directory;
- `eng relay-verify-qualification-kit --dir DIR` independently revalidates the
  existing kit's exact directory/file modes, canonical JSON/TOML bytes,
  cross-binding and SHA256SUMS without rebuilding it or contacting any provider.

This stage now uses relay prequalification schema v2. The v1 live-tree document
has been retired with no compatibility shim because no relay was yet
live-qualified or admitted. v2 freezes one exact Codex custom-provider
configuration, its credential locator (never credential material), requested
model and policy digests, plus the renderer contract version and exact rendered
Codex TOML digest. The final provider_config_digest binds the contract digest,
renderer version and Codex config digest. HTTPS remains the default. An explicit private-HTTP exception is
available only for literal private/loopback IP endpoints; public/DNS/link-local
cleartext endpoints remain fail-closed, and the exception bit is included in the
provider configuration digest. The operator pack computes policy digests from
exact file bytes, rejects symlinked/oversized inputs, and performs no network or
credential operation. Command-token mode additionally binds an explicit
token-helper timeout and requires refresh_interval_ms=0 for the initial posture.
Initial qualification disables request/stream retries, WebSockets and standalone
web search to keep the first provider proof bounded.

A successful repository prequalification explicitly records
`account_verified=false`, `live_model_turn_executed=false` and
`provider_admitted=false`. It therefore cannot satisfy #105 and cannot make
`company-relay` valid in `internal/provideridentity`.

The future live gate is now pre-frozen by a deterministic manifest digest:
exactly one model turn, read-only, approval never, no tools/tool-network/web
search, zero request/stream retries, bounded output/time, exact prompt/output
digests, and mandatory gateway operation ID plus effective provider/model
identity. A separate deterministic runtime handoff re-binds the retained Codex
compatibility qualification to the exact native binary bytes/version/model using
only local hashing and `codex --version`; this prevents a later credentialed
probe from silently swapping the runtime. The same repository-side facts can now be emitted as one deterministic
qualification kit with a cross-bound manifest and raw-byte SHA256SUMS, so a
future credentialed probe must consume the already-frozen artifacts instead of
reconstructing them ad hoc. The kit now has an independent offline verifier that
rejects extra files, symlinks, permission drift, non-canonical JSON, rendered
Codex config drift, broken cross-binding or checksum drift. Synthetic receipt
tests prove only schema behavior and do not qualify the relay.
Account/credential verification and the actual live model turn remain deferred
to the next #106 gate.

## Productionization baseline

Issue #105 is now the terminal productionization epic. The canonical operating
contract is `docs/implementation/PRODUCTION_OPERATIONS_V1.md`.

Production-v1 targets Ubuntu + systemd + PostgreSQL rather than Kubernetes.
The first production baseline adds:

- hardened systemd units for Control Plane, Worker admission and Worker
  preparation;
- distinct non-root Unix service identities for those roles;
- owner-private EnvironmentFile contracts;
- `AUTO_MIGRATE=0` for production service startup;
- `eng production-preflight --config FILE` for host-side static validation.

Production preflight fails closed if Control Plane carries the legacy
`GITHUB_PUBLISHER_CONFIG_FILE`. Production now uses an independent
`publisher-service` under its own Unix identity. Control Plane holds only the
non-secret publication plan plus mTLS remote-client configuration; the
Publisher service alone reads the GitHub token and independently rechecks target
policy and retained bundle identity before GitHub mutation.

The publisher service split preserves the existing Action Gateway operation and
UNKNOWN/reconciliation authority; mTLS transport failure is not an instruction
to blindly replay publication.

With this internal P0 closed, production preflight can reach `INTERNAL=READY`
and `OVERALL=PROVIDER_PENDING`. The remaining provider blocker is:

- `unattended_provider_live_qualification`.

Provider/account validation remains a later live gate.

## Production operational readiness

Production-v1 now has an authenticated operational-status authority rather than
using process liveness as readiness.

`GET /api/v1/operations/status` requires the existing mTLS identity and
`core:read` capability. PostgreSQL returns one bounded snapshot of Recovery,
Run, Worker lease/inbox, outbox, external Action and Core-bound Codex UNKNOWN
facts. `eng production-status [--require-ready]` exposes that status to
operators without provider credentials.

The evaluation is deterministic and does not guess SLO thresholds:

- Recovery/Reconciliation -> `RECOVERY_REQUIRED`;
- expired Worker lease, dead-letter outbox, UNKNOWN/RECONCILING/MANUAL Action or
  UNKNOWN Codex execution -> `DEGRADED`;
- benign pending work remains observable but does not fail readiness merely
  because a guessed queue/latency threshold was exceeded.

This closes the repository-side facts/alerting substrate. Measured latency/SLO
targets and the terminal maintenance qualification remain later #105 slices.

## Production terminal acceptance

The final #105 acceptance contract is now frozen in
`docs/implementation/PRODUCTION_TERMINAL_ACCEPTANCE_V1.md`.

Repository-side pre-live assets now include:

- deterministic `eng production-terminal-plan` + plan digest;
- a one-time RELEASE fixture under
  `examples/production/terminal-maintenance`;
- exact requirement digest and marker before/after bytes;
- Work/Task/Run templates validated through Core;
- explicit independent human Review and Closure gates;
- `eng production-slo-report` for measured source-bound
  count/min/p50/p95/max observations;
- a manual protected-main `production-terminal-pre-live.yml` dry-run that uses
  no provider secret and retains the frozen fixture/plan/SHA256SUMS artifact.

The SLO contract requires all non-provider measurements before calling internal
calibration complete and separately requires the real provider-authentication
measurement before status can become COMPLETE. No guessed latency thresholds are
encoded.

This still does not qualify an unattended provider and does not claim production
readiness. The next external state-changing gate remains one real provider live
qualification, after the pre-live dry run is retained.

## Managed-workspace WIF unattended gate

Managed-workspace WIF remains a separate **optional unattended** qualification
gate tracked by issue #103; it is not a blocker for normal internal Ubuntu
development and is not an M1 phase-1 blocker.

Real live qualification probe `36556354798` was dispatched on protected main
`644127255f615abb753fab5c450ea23caad76b11`. It failed in the first
prerequisite step because both non-secret repository Actions variables were
absent:

- `OPENAI_WIF_AUDIENCE`;
- `OPENAI_CODEX_FEDERATION_RULE_ID`.

The run stopped before checkout, GitHub OIDC assertion minting, OpenAI
federation exchange or any model turn. No managed-WIF model call was consumed.

The remaining external action is administrator provisioning through
`examples/pilots/wif/configure-admin-api.sh`, using a WIF-enabled managed
ChatGPT workspace, an Admin API key authorized for workload identity, an
existing workspace principal and a dedicated GitHub OIDC audience. After that
helper writes the two repository variables, rerun `codex-wif-live.yml` and
require one retained successful live receipt before calling the unattended
authentication lane qualified.

Do not reopen Feature/Debug M1 pilots merely to qualify WIF, and do not weaken
repository/ref/workflow/audience/immutable-ID constraints to make the exchange
succeed.

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

## Superseded branch disposition

The following non-retained branches are explicitly classified as
**superseded history**. They are not pending product work, grant no authority,
and must not be merged or cherry-picked back into main.

Older implementation prototypes:

- `feat/github-ci-core-evidence-import` — replaced by #44 trusted CI import,
  then hardened by #48 and #51.
- `feat/independent-review-authority` — replaced by the stricter #46
  independent Review authority.
- `docs/retire-superseded-historical-branches` — documentation transport for
  #110; its content is already on main.

Relay/provider development branches:

- `feat/relay-provider-prequalification` — superseded by #111.
- `feat/relay-prequalification-operator-pack` — superseded by #112.
- `feat/relay-private-http-policy` — superseded by #113.
- `feat/relay-codex-config-renderer` — exploratory renderer branch; superseded
  by the v2 renderer identity merged in #114.
- `feat/relay-codex-config-renderer-v2` — no unique commits remain relative to
  its main checkpoint and it is superseded by #114.
- `feat/relay-prequalification-v2-renderer` — merged/superseded by #114.
- `feat/relay-live-handoff-manifest` — merged/superseded by #115.

Current canonical relay progression on main is therefore:

```text
#111 repository prequalification
  -> #112 non-secret operator pack
  -> #113 explicit private-HTTP transport policy
  -> #114 prequalification v2 + exact Codex renderer identity
  -> #115 frozen future live qualification manifest
```

These superseded refs may be deleted as repository hygiene. Their continued
existence is not an implementation blocker and must not be interpreted as
parallel supported implementations.

Do **not** apply this disposition to the two retained M1 result branches:

- `engineering-platform/3ac04fc7097e8375e5f7c1f8` — retained Feature result;
- `engineering-platform/994964383ed085e51545105d` — retained Debug result.

Those two branches are immutable evidence references for the already-closed M1
Feature/Debug chains and remain intentionally preserved.

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
five canonical CI checks are required. The default internal trusted self-hosted
lane already has real retained Closure evidence. Source merge still does not
provision an unattended machine credential/provider or a production service.

M1 phase 1 no longer requires WIF, and the retained proof sequence has now
completed for both required subjects:

1. Feature #54 ran from frozen base
   `75b6e24a409999129a0e89e6aa56e13733109034`. Engineering run
   `36505950006` reached FINISHED and publication CONFIRMED; the workflow then
   hit a mechanical post-publication self-copy failure. Read-only recovery run
   `36515098424` validated the retained subject with no model replay.
2. Feature exact-head CI `36513182863`, Verification `36516020897` and
   independent Review `36517048600` passed. ClosureReceipt
   `m1-feature-routing-closure` closed the WorkItem.
3. Debug #55 first retained authoritative reproduction run `36518555837` on
   frozen base `9d6309c6eff62d3489959ed574dda31536578568`, then engineering
   run `36518908635` completed directly with FINISHED + publication CONFIRMED.
4. Debug exact-head CI `36526250474`, Verification `36536077026` and
   independent Review `36536314395` passed. ClosureReceipt
   `m1-debug-firmware-identity-closure` closed the WorkItem.
5. Post-Closure promotion PRs #100 and #101 integrated the already-closed
   retained results onto protected main without mutating the immutable retained
   result branches. The post-Closure code checkpoint was
   `ee0249fe25b860a83beba9fe06b7acdc80cd75c6`; fresh-main CI
   `36538546170` passed. The current authentication-policy checkpoint is
   `1b50c17ccf3dbef3b0ac708da574cdfc88871944` (#104), with fresh-main CI
   `36558089001` PASS.
6. The original publication transport PRs #91 and #98 are closed as superseded
   evidence transports; their retained result SHAs remain immutable.

Detailed proof is retained in
`docs/status/M1_RETAINED_PHASE1_CLOSURE_2026-09-29.md`.

`examples/pilots/self-hosted/qualify-login.sh` remains an optional diagnostic
read-only model probe. It is not a prerequisite for phase-1 retained engineering,
so the saved ChatGPT login does not need to be consumed/refreshed twice before
the non-replayable retained turn.

The existing managed-workspace WIF/Admin API/GitHub-hosted workflow remains
supported as an unattended workload-identity lane. It is optional for current
internal development, is no longer an M1 phase-1 blocker, and must not be
represented as qualified until a real WIF turn exists.

Trusted self-hosted mode has explicit known limits: the host/account itself is
trusted, and the repository is public. The retained engineering runner therefore
uses a one-time random label, no default runner labels and ephemeral one-job
registration; it must never remain persistently eligible for public-repository
workflow jobs. Unrelated credentials elsewhere on the host account are outside
the Codex child-process isolation boundary. The model process receives neither the
operator HOME nor publisher/API/WIF credential environment, and the isolated
saved-login bootstrap is deleted before thread/start. Production/unattended
claims require a separately qualified unattended credential/isolation design;
managed-workspace WIF is one candidate, and a reviewed company relay/provider
may be another if it independently satisfies the authority contract.

At this checkpoint there are **two real retained Closure chains**: one Feature
and one Debug. This is sufficient to claim the trusted self-hosted **M1 phase-1
retained vertical slice** as proven for the scoped pilot authority chain.

Repository fixtures/fake app-server tests remain protocol/filesystem evidence
only and do not qualify any unattended credential/provider lane. For WIF, a real
administrator-provisioned provider/rule and live WIF model turn are required
before #103 may close. A future company relay/provider must independently meet
the #106 qualification contract and then receive a new explicitly admitted
Provider v3 identity; it cannot reuse either current `openai-codex` combination
or their qualification receipts before it may satisfy the credential prerequisite
for #105.

**Trusted self-hosted M1 phase 1 is proven. Production/unattended readiness
remains unclaimed.**

Contracts:
- `docs/implementation/CODEX_CREDENTIAL_LANES_V1.md`
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
