# engineering-platform

AI-native embedded-software engineering platform for the R&D center.

## What this project is

engineering-platform turns a real embedded engineering work item into a controlled Human + AI execution and assurance loop:

~~~text
WorkBuddy / CLI / IDE
        ↓
WorkItem
        ↓
Material Readiness
        ↓
Embedded Capability / Skill
        ↓
Task Contract + Verification Plan
        ↓
Ubuntu Worker + Interactive Runtime
        ↓
Engineer + Codex/Claude
        ↓
Git / Build / CI / Device
        ↓
Artifact + Delivery Receipt
        ↓
Evidence
        ↓
Verification
        ↓
Review when required
        ↓
Closure
~~~

The first product scope is embedded software engineering:
- Embedded Linux / BSP
- MCU / RTOS / bare metal
- drivers and components
- boot / storage / OTA
- debugging and reliability
- performance
- multi-component embedded systems
- later Device/HIL

## Clean-slate architecture

This repository is an independent new platform.

It does **not** preserve compatibility with historical digital-worker schemas, repository boundaries, runtime profiles, or cross-repository ownership.

Useful engineering principles may be re-derived, but the implementation is clean-slate.

Current architecture, policy and status references (the M0/M1 plan is **historical**, not a live operating checklist):

- [Core Architecture v1](docs/architecture/EMBEDDED_AI_ENGINEERING_PLATFORM_CORE_V1.md)
- [Embedded Domain Capability Model v1](docs/architecture/EMBEDDED_DOMAIN_CAPABILITY_MODEL_V1.md)
- [Historical Core M0 / M1 Vertical Slice Plan v1](docs/roadmap/CORE_M0_M1_VERTICAL_SLICE_PLAN_V1.md)
- [Implementation Status](docs/status/IMPLEMENTATION_STATUS.md)
- [Codex Credential Lanes v1](docs/implementation/CODEX_CREDENTIAL_LANES_V1.md)
- [Extension Catalog v1](docs/extensions/EXTENSION_CATALOG_V1.md)
- [ADR-001 — Clean-Slate Embedded Platform Scope](docs/adr/ADR-001-clean-slate-embedded-platform-scope.md)

Superseded implementation profiles, adoption-plan revisions and transient CI
checkpoint markers are retained by Git history rather than kept beside current
authority-bearing documents. Explicit architecture reviews and research remain
design evidence, not the current M0/M1 implementation checklist.

## Core product principles

1. **Embedded-specific from M1.** Expert/Capability/Skill is core product behavior, not a late marketplace feature.
2. **Material readiness fails closed.** Missing source/log/device/acceptance material is explicit; the Runtime does not invent it.
3. **Interactive execution.** Observe, steer, add context, pause, resume, checkpoint and Human Takeover are first-class.
4. **Runtime provider neutral.** Codex first, Claude later; domain semantics do not depend on provider internals.
5. **Privileged actions stay outside the Runtime sandbox.** Git push, CI, flash, signing and release go through policy-controlled Action Gateway.
6. **External side effects are reconciled.** Timeout/UNKNOWN is never blindly retried.
7. **Artifact and Evidence are exact.** Path/tag/branch is not identity.
8. **Runtime claim is not Verification.**
9. **Engineering != Verification != Review.**
10. **Real engineering Evidence drives maturity.** File/schema/CI existence alone does not prove a Skill or workflow is mature.

## Embedded domain

Initial capabilities:

~~~text
embedded.architecture
embedded.linux-bsp
embedded.mcu-rtos
embedded.driver-component
embedded.debug-reliability
embedded.verification
~~~

Initial Skills:

~~~text
material-readiness
architecture-impact-analysis
interface-contract-review
linux-bsp-debug
mcu-rtos-debug
driver-integration-review
log-triage
verification-plan-builder
~~~

Automatic Planner comes later. Explicit Skill routing is enough for M1.

## Current implementation status

The platform is past M1 bootstrap. **Trusted self-hosted M1 phase 1 is proven**
by one real retained Feature Closure chain and one separate retained Debug
Closure chain. The current repository-side RC posture is
**`INTERNAL_CLOSED_CANDIDATE_EXTERNAL_QUALIFICATION_PENDING`**: the approved
small-team implementation/fault/governance slices are closed, while unattended
provider, production-environment, WorkBuddy/device and independent human
acceptance remain external gates. This is not production qualification.

The authoritative live status is
[Implementation Status](docs/status/IMPLEMENTATION_STATUS.md). Authentication
and provider selection are summarized in
[Codex credential lanes v1](docs/implementation/CODEX_CREDENTIAL_LANES_V1.md).

Current execution posture uses **Profile v3**, which freezes
`provider_id / credential_mode / execution_mode / provider_config_digest` into
the Core-authorized profile digest. Worker execution, retained receipts,
Recovery, Verification and Review all carry that same provider identity.

- **default internal development lane:** trusted Ubuntu host with a
  ChatGPT-authenticated Codex CLI, exact compatibility qualification, isolated
  Codex HOME and the existing Core/Worker/Action Gateway/Evidence chain;
- **optional unattended lane:** managed-workspace WIF for GitHub-hosted or other
  workload-identity execution; repository mechanics are ready, but the external
  workspace administrator provider/rule and first real live qualification are
  still pending in #103;
- **optional future company relay/model gateway:** a separate provider lane that
  requires its own compatibility, credential, privacy, retry/UNKNOWN and
  provenance review plus an explicitly admitted Provider v3 identity. It is not
  a WIF fallback and is currently rejected by provider admission.

Feature #54 and Debug #55 are already CLOSED and their retained results were
promoted to protected main after Verification/Review/Closure. Do not rerun them
merely to qualify another credential lane.

Production/unattended readiness remains a separate gate (#105). Do not infer it
from files, schemas, mocks or green CI alone; retained operational Evidence is
the maturity boundary.

## M0

M0 freezes only contracts required by the first real engineering loop:

- WorkItem / TaskContract
- MaterialManifest / TargetContext
- RunInputManifest
- Run / RunAttempt / execution_epoch
- Session / Steering / Checkpoint
- ActionRequest / ActionReceipt / ExternalOperation
- ArtifactRef / DeliveryReceipt / EvidenceRef
- VerificationPlan / VerificationReport / ReviewReport
- ClosureReceipt
- Debug HypothesisRegistry
- canonical digest / audit / recovery_epoch

PLM, MES, RMA, DPP, GS1, manufacturing, PSIRT, certification, privacy, fleet and other lifecycle research are **not** M0 blockers.

## M1 design journeys

The following are the canonical product journeys. The retained phase-1 proof
covers bounded engineering-to-Closure execution, not every interactive control
in these journeys:

### Feature pilot

~~~text
WorkBuddy/CLI
 -> TaskContract
 -> Material Ready
 -> Skill route
 -> Codex interactive Run
 -> Steering
 -> Git change
 -> Build/CI
 -> DeliveryReceipt
 -> Evidence
 -> Verification
 -> Closure
~~~

### Debug pilot

~~~text
Issue/log
 -> MaterialManifest
 -> HypothesisRegistry
 -> log-triage
 -> embedded debug Skill
 -> experiment/evidence
 -> fix or explicit BLOCK
 -> regression Verification
~~~

Only after these work reliably does the project move to M2 Device/HIL.

## Current implementation stack

Implemented: Go, PostgreSQL, direct mTLS, Ubuntu Worker, Codex app-server,
Git/GitHub Actions, an independent Publisher and exact retained evidence.
Temporal, OPA, S3/MinIO and OpenTelemetry remain design candidates, not deployed
services or prerequisites of the current small-team baseline. Add a service only
when a measured implementation gap justifies it.

## Extensions and research

The research archive remains valuable.

Use:
- [Extension Catalog v1](docs/extensions/EXTENSION_CATALOG_V1.md) to decide when a researched capability should activate.
- [Open Source Research Index](docs/research/OPEN_SOURCE_REFERENCE_INDEX.md) for detailed standards/tool/reference research.

Examples of later extensions:
- Device/HIL / labgrid
- simulation/emulation
- interface compatibility
- system safety
- diagnostics
- ML/audio/KWS
- OTA/fleet
- manufacturing/PLM
- PSIRT/security
- certification/compliance
- RMA
- DPP/GS1/EPCIS

These are not inherited into Core automatically.

## Repository maturity

**Trusted self-hosted M1 phase 1 is proven and the repository-side internal RC
is a closed candidate; production/unattended qualification is not granted.**
The Feature and Debug retained chains are historical proof, not pending WIF
prerequisites and not a substitute for production acceptance. The only tracked
open gates are #103 (WIF), #106 (optional relay/provider) and #105 (production
acceptance).

The current delivery contract includes all six executables, including
`publisher-service`. After extracting an authenticated CI artifact and restoring
its executable modes, verify it without rebuilding missing components:

```sh
/path/to/bin/eng distribution-verify --dir /path/to/bin
```

Production preflight v2 binds the expected source commit, real host identities,
configuration/TLS references and binary roles. `--config-only` yields only
`CONFIG_VALIDATED`; default preflight can yield `HOST_VALIDATED`, never live
operational qualification. The systemd Control Plane launches with
`--production`, which prohibits the in-process publisher regardless of a
conflicting EnvironmentFile.

SLO report v2 distinguishes `UNVERIFIED_SUMMARY` from `SOURCE_BYTES_VERIFIED`.
Neither grants production qualification. WorkBuddy integration and the complete
interactive Worker control path must not be inferred from adapter methods alone.
See [Implementation Status](docs/status/IMPLEMENTATION_STATUS.md) for the current
remaining gates.

## Live engineering controls

`eng run-control inspect|status|steer|interrupt` uses the existing direct-mTLS
Control API to bind operator input to the actual authorized thread/turn.
Steering acceptance, cancellation ACK, interrupted event and app-server process
exit are separate observations. Missing or UNKNOWN receipts block successful
delivery and are never replayed. See
[Live Codex controls v1](docs/implementation/LIVE_CODEX_CONTROLS_V1.md).
The `run-control` surface intentionally covers live Codex steer/interrupt only.
Core persists pause/resume separately, and exact-epoch Human Takeover is exposed
by `eng human-takeover`; takeover increments the execution epoch and transfers
control to HUMAN without replaying the model turn.

### Stopped source recovery

Interrupted/failed engineering turns with a sealed quiescent scope can retain
private source checkpoints. The authenticated execution status exposes the exact
descriptor; `eng source-checkpoint verify|restore` can recover bytes to a new
private directory without account access or a model replay. This is not automatic
resume or takeover. See [the source-checkpoint contract](docs/implementation/STOPPED_SOURCE_CHECKPOINT_V1.md).

### Explicit source continuation

After a proven requested interruption, `eng run-continue authorize|status`
connects the retained private source to one new Run under the existing Work,
frozen Task and original Git base. It requires the authenticated Work owner,
RunStart + RunControl, exact checkpoint/version/epoch bindings and a new host
preparation approval. Old execution is stopped without successful Delivery;
it is not replayed. The new turn rechecks the complete inherited diff.
See [source continuation contract](docs/implementation/SOURCE_CONTINUATION_V1.md).
Source continuation does not reconstruct model memory and is not itself Human
Takeover or production qualification; Human Takeover is the separate exact-epoch
control-owner transfer path described above.
