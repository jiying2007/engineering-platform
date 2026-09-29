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

Current canonical documents:

- [Core Architecture v1](docs/architecture/EMBEDDED_AI_ENGINEERING_PLATFORM_CORE_V1.md)
- [Embedded Domain Capability Model v1](docs/architecture/EMBEDDED_DOMAIN_CAPABILITY_MODEL_V1.md)
- [Core M0 / M1 Vertical Slice Plan v1](docs/roadmap/CORE_M0_M1_VERTICAL_SLICE_PLAN_V1.md)
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
Closure chain.

The authoritative live status is
[Implementation Status](docs/status/IMPLEMENTATION_STATUS.md). Authentication
and provider selection are summarized in
[Codex credential lanes v1](docs/implementation/CODEX_CREDENTIAL_LANES_V1.md).

Current execution posture:

- **default internal development lane:** trusted Ubuntu host with a
  ChatGPT-authenticated Codex CLI, exact compatibility qualification, isolated
  Codex HOME and the existing Core/Worker/Action Gateway/Evidence chain;
- **optional unattended lane:** managed-workspace WIF for GitHub-hosted or other
  workload-identity execution; repository mechanics are ready, but the external
  workspace administrator provider/rule and first real live qualification are
  still pending in #103;
- **optional future company relay/model gateway:** a separate provider lane that
  requires its own compatibility, credential, privacy, retry/UNKNOWN and
  provenance review. It is not a WIF fallback.

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

## M1

M1 proves two real vertical slices:

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

## M1 infrastructure

Default:
- Go
- PostgreSQL
- Temporal
- OPA
- S3/MinIO-compatible Artifact store
- OpenTelemetry
- Git/CI
- Ubuntu Worker
- Codex

Test-only:
- Toxiproxy
- disposable integration dependencies where practical

Do not add a service because a future extension might need it.

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

Current stage:

> **retained-pilot-ready (external WIF-gated)**

Internal M1 execution mechanics are assembled, but M1 is not claimed until the
real retained Feature and Debug pilots complete. The next maturity milestone is
not another architecture profile. It is:

> **one real Feature pilot and one real Debug pilot completed through the new Core with exact Evidence and Verification.**

Further architecture research should be targeted only by gaps discovered during implementation or real product pilots.
