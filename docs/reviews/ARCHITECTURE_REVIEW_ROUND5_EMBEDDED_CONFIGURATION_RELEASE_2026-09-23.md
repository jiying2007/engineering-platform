# Architecture Review — Round 5: Embedded Product Configuration and Release Integrity

Date: 2026-09-23
Reviewed baseline: `docs/architecture/AI_NATIVE_ENGINEERING_PLATFORM_V1_1.md`
Depends on: Round 2–4 reviews
Focus: multi-firmware embedded products, target variants, compatibility, deployment/rollback, build reproducibility, protocol evolution and operational admission control.

## Result

The generic Engineering Control Plane architecture remains correct, but the embedded-product release model is still incomplete.

For a real product, a "release" is often not one binary. It may contain:
- Linux kernel/rootfs/application
- main-board MCU firmware
- motor MCU firmware
- dock/charger MCU firmware
- bootloader
- FPGA/DSP/NPU/model assets
- configuration/calibration data
- update metadata and partition/layout constraints

The platform therefore needs an explicit **Target Configuration / Product Configuration** concept before M2 and should reserve it in M0 schemas.

Without this, the platform can prove that an Artifact was verified but still deploy the correct Artifact to the wrong hardware/bootloader/product variant.

---

## P0-1 — Add Target Specification / Product Variant as a formal authority

Define a stable Target identity and immutable Target Revision.

Example:

```yaml
target_id: PCR02
revision: 7

variant:
  sku: PCR02-CN
  hardware_revision: V3

components:
  linux:
    soc: SSC305
    board_revision: V3
  main_mcu:
    part: GD32L235
    board_revision: V3
  motor_mcu:
    part: MM32SPIN023C
  charger_mcu:
    part: HC32F072

boot_constraints:
  bootloader_min: ...
  partition_layout: ...
  secure_boot_profile: ...

interfaces:
  motor_protocol: v4
  dock_protocol: v2
```

Requirement, Task, Subject Manifest, Verification and Release Manifest may bind one or more exact Target Revision digests.

Do not encode target compatibility only in free-text labels or CI job names.

---

## P0-2 — Add Component Set / Release Bundle semantics

A product Release may contain multiple artifacts that must be treated as one validated set.

Define a Release Bundle inside the Release Manifest:

```yaml
bundle:
  target_revision_digest: ...
  components:
    - role: linux
      artifact_digest: ...
      version: ...
    - role: main_mcu
      artifact_digest: ...
      version: ...
    - role: motor_mcu
      artifact_digest: ...
      version: ...
    - role: charger_mcu
      artifact_digest: ...
      version: ...
```

The Release decision applies to the exact bundle digest, not to each component independently unless policy explicitly allows independent promotion.

This prevents "each firmware passed alone, but this combination was never validated".

---

## P0-3 — Define compatibility constraints as machine-readable policy

Compatibility must be explicit.

Examples:
- Linux app v2 requires main MCU protocol >= 4.
- motor MCU firmware 0.6.x requires main-board firmware >= 1.8.
- OTA package requires bootloader >= X.
- rootfs requires partition layout revision Y.
- model artifact requires NPU/runtime revision Z.

Define a compatibility expression model:

```text
component/version/artifact
  requires
component/protocol/layout/hardware range
```

Release Verification evaluates the complete compatibility set.

Do not rely on engineers remembering cross-component version rules.

---

## P0-4 — Distinguish software version, artifact identity and compatibility identity

These are different:

```text
version = human/product semantic label
artifact digest = exact bytes
compatibility identity = machine-readable interface/ABI/protocol contract
```

Two artifacts can both report version 1.2.3 but have different bytes.
Two versions may remain protocol-compatible.
A byte-identical artifact may be reused across multiple target variants.

Store all three dimensions explicitly.

---

## P0-5 — Protocol/ABI contracts need their own revisions

Cross-component embedded development depends on interfaces.

Define versioned interface contracts for:
- MCU communication protocol
- IPC/RPC schema
- OTA metadata/partition contract
- persistent storage schema
- calibration/configuration schema
- bootloader/application handoff
- device/HIL control protocol where relevant

Tasks declare which contracts they consume/provide.

Change-impact analysis can then detect:
- implementation-only change
- backward-compatible interface change
- breaking interface change

A breaking interface change should automatically widen affected Work/Verification scope.

---

## P0-6 — Add migration and rollback compatibility to Release Manifest

A Release is not valid merely because the final state works.

It must define supported transitions:

```text
from configuration A
  -> migration/update procedure
  -> configuration B
```

Record:
- supported source versions/configurations
- preconditions
- migration steps
- ordering between components
- interruption behavior
- rollback target
- rollback compatibility
- data/schema downgrade constraints

For OTA, "rollback available" must mean the old configuration can actually boot and interpret persisted state after the attempted upgrade.

---

## P0-7 — Multi-component update needs transaction semantics at product level

You cannot assume updates across Linux + several MCUs are atomic.

Define a product update state machine with explicit safe intermediate configurations.

Example:

```text
PRECHECK
 -> STAGE_ARTIFACTS
 -> UPDATE_MAIN_MCU
 -> VERIFY_MAIN_MCU
 -> UPDATE_MOTOR_MCU
 -> VERIFY_MOTOR_MCU
 -> UPDATE_LINUX
 -> REBOOT
 -> POSTCHECK
 -> COMMITTED
```

Failure policy identifies:
- safe retry point
- component-specific rollback
- whole-product rollback
- quarantine/manual recovery

The allowed ordering belongs to the Release/Deployment policy, not agent improvisation.

---

## P0-8 — Verification must bind the tested configuration, not just artifact

For embedded systems, Evidence should include enough target state to prove what was actually tested.

Add to Subject Manifest / Evidence context as applicable:
- Target Revision digest
- hardware revision / serial class
- bootloader version
- existing firmware configuration
- loaded Release Bundle digest
- calibration/fixture revision
- power/environment mode
- relevant persistent-data schema revision

This prevents a PASS on V3 hardware from silently proving V2 or another SKU.

---

## P0-9 — Baselines are first-class verification inputs

Many acceptance criteria are relative:
- "not lower than current baseline"
- regression threshold
- performance delta
- thermal/current delta

Do not store "baseline" as free text.

Define immutable Baseline Artifact/record:
- subject/target
- procedure revision
- raw evidence refs
- metric set
- statistical definition
- selection/approval decision
- baseline digest

Verification then binds the exact baseline digest.

Changing the baseline creates a new verification context rather than rewriting history.

---

## P0-10 — Non-deterministic verification needs statistical contract

For motor control, audio, KWS, robotics and long-run stability, one PASS run may not be sufficient.

Procedure Revision should define:
- sample size
- repetition count
- warm-up
- random seed where relevant
- environmental conditions
- confidence/acceptance rule
- allowed outlier handling
- stop rule

Store raw per-run Evidence plus aggregate Evaluation Result.

This prevents a single favorable run from being promoted as authoritative evidence.

---

## P0-11 — Formal configuration drift detection is required

After a device is flashed or a Worker environment is prepared, actual state can drift from expected manifest.

Before authoritative Verification, measure and compare:
- actual firmware component digests/versions
- board/target identity
- fixture state
- toolchain/test software digest
- calibration state

If actual state does not match Subject Manifest, Verification must not proceed as authoritative.

Device/HIL should be fail-closed on configuration mismatch.

---

## P0-12 — Build environment should be versioned as an Artifact/profile

Embedded SDKs/toolchains are often host-sensitive.

A build provenance record should resolve to an immutable build environment definition:
- container/image digest when possible
- compiler/toolchain exact version
- SDK/BSP revision
- build scripts
- configuration/defconfig
- generated-code tool versions
- environment variables that affect output

M1 may tolerate non-reproducible legacy toolchains, but the system must record the environment and surface reproducibility level rather than pretend all builds are hermetic.

Suggested classification:
- REPRODUCIBLE
- CONTROLLED
- RECORDED_LEGACY
- UNKNOWN

Release policy can require stronger classes for higher-risk components.

---

## P0-13 — Worker/runtime protocol needs version negotiation

The platform will evolve while Ubuntu Workers may not upgrade simultaneously.

Every Worker connection should advertise:
- worker software version
- control protocol version range
- Session protocol version range
- runtime adapter versions
- supported sandbox/capability features

Control Plane selects a compatible protocol or rejects/quarantines the Worker.

Do not assume lockstep deployment of all Workers.

The same applies to:
- `eng` CLI
- WorkBuddy Connector
- Device Agent
- Runtime Adapter contract

---

## P0-14 — Define upgrade compatibility for stored schemas and running workflows

Control Plane deployments will occur while Runs are active.

Freeze rules for:
- database migration compatibility
- Temporal Workflow versioning
- old Run/Attempt payload decoding
- old Checkpoint resume
- old Worker protocol compatibility
- rolling deployment safety

No platform upgrade should strand active Formal Runs without an explicit migration/termination Decision.

---

## P1-1 — Add admission control and quotas

Formal execution consumes scarce resources:
- model/API quota
- Ubuntu CPU/RAM
- CI minutes
- artifact storage
- shared Device/HIL
- human approval attention

Add admission control before scheduling.

Policy dimensions may include:
- project
- priority
- risk
- runtime/provider
- budget
- Worker pool
- device pool
- concurrency limit

This prevents one noisy Work from starving the engineering organization.

---

## P1-2 — Scheduling needs priority and fairness

Once multiple Works exist, "find any eligible Worker" is insufficient.

Define at least:
- priority class
- FIFO within class where practical
- project quota/fair share
- preemptibility
- max Run duration
- device reservation policy

High-risk Release Verification should not be blocked indefinitely by low-value exploratory workloads.

---

## P1-3 — Cost budget can be a policy input, not a completion criterion

Record and expose:
- model/API usage
- compute duration
- CI cost proxy
- device/HIL occupancy
- storage

Budget can constrain provider/runtime routing or require approval.

But never treat "cheap" as equivalent to "verified".

Cost policy and engineering acceptance remain separate.

---

## P1-4 — Define provider/data-residency eligibility

Runtime provider selection should consider data classification and project policy.

Example:
- RESTRICTED context may prohibit external provider A.
- certain repositories may require enterprise tenant/runtime only.
- source export may be disabled while local tools remain allowed.

Provider routing policy consumes both:
- context confidentiality classification
- provider eligibility profile

This stays outside the model's own decision-making authority.

---

## P1-5 — Add compatibility matrix read models

Engineering users need simple queries:
- Which Release Bundle is valid for PCR02 V3?
- Which motor firmware is compatible with main MCU firmware X?
- What is the minimum bootloader for Release Y?
- Which target variants are covered by Verification Z?

Build relational projections for these queries.

Do not introduce a graph database unless relational performance proves insufficient.

---

## P1-6 — Define quarantine semantics

Targets, devices, artifacts or releases may become suspect after later findings.

Support quarantine for:
- Device
- Fixture
- Worker
- Artifact
- Release
- Evidence Issuer

Quarantine does not erase historical records.

It prevents new authoritative use and triggers impact analysis for dependent verification/release objects.

---

## P1-7 — Post-release field evidence should be attachable without mutating the Release

Production/field telemetry, crash reports and defect findings may arrive after Release.

Represent them as new Evidence/Incident objects referencing the immutable Release Manifest.

They may lead to:
- risk Decision
- Release quarantine
- rollback
- new Requirement/Work

Never rewrite the historical release evidence to make the past appear different.

---

## Required M0 schema reservations

Even if M1 only implements one simple target, reserve these concepts now:

1. Target + Target Revision
2. component role/type
3. interface/protocol contract revision
4. Release Bundle / component set
5. compatibility constraints
6. Baseline identity
7. build-environment/reproducibility profile
8. protocol-version negotiation fields for Worker/CLI/Connector/Device Agent
9. environment/project namespace
10. quarantine state/applicability

The first implementation can leave advanced fields unused; the schema must not encode assumptions that make multi-component embedded products a breaking redesign later.

---

## Additional M2 go/no-go criteria

M2 embedded trusted loop should require:

- exact Target Revision is known before flash/test;
- actual device configuration is measured before authoritative verification;
- tested Release Bundle digest is recorded;
- incompatible component combinations are rejected before deployment;
- procedure/baseline revisions are immutable and bound to Evidence;
- repeated/statistical tests follow the declared procedure contract;
- device/fixture/issuer quarantine prevents new trusted Evidence;
- OTA/update failure can identify a safe recovery path;
- rollback compatibility is checked, not merely documented;
- target/hardware coverage is visible from Verification;
- release promotion cannot silently swap a component artifact.

---

## Assessment

The architecture is now strong at generic workflow, identity, evidence and runtime governance.

The main remaining architectural risk for the intended embedded use case is **configuration correctness**:

> proving an Artifact is good is not enough; the platform must prove that the exact set of Artifacts, interfaces, hardware revision, baseline and migration path is valid as one product configuration.

The next architecture baseline should therefore add Target/Configuration/Compatibility semantics without adding another top-level Plane.
