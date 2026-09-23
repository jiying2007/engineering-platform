# Reference-Aligned Implementation Profile v1

Date: 2026-09-23
Status: **Canonical implementation profile for v1.2 architecture**
Parent architecture: docs/architecture/AI_NATIVE_ENGINEERING_PLATFORM_V1_2_FINAL.md
Research basis: docs/research/OPEN_SOURCE_REFERENCE_REVIEW_2026-09-23.md

## 1. Purpose

This profile converts the open-source review into concrete implementation decisions.

It does not change the five-plane architecture or engineering authority model.

It changes the implementation strategy from:

> build most infrastructure ourselves

to:

> own domain authority; reuse mature execution mechanisms through narrow adapters.

---

## 2. Final implementation principle

### Engineering-platform owns

- Requirement / Work / Task contracts;
- Target / Compatibility;
- Run / Attempt identity;
- Steering / Human Takeover semantics;
- Capability / Human Authority;
- Artifact identity;
- Evidence applicability;
- Verification / Review / Risk;
- Release / Closure authority;
- cross-system reconciliation.

### External/open-source components may own

- durable workflow execution;
- low-level device/lab control;
- cryptographic signing/attestation;
- optional reproducible build execution;
- optional OTA installation;
- provider/runtime-specific shell/session mechanisms.

Rule:

~~~text
External component implementation state
!=
engineering-platform business authority
~~~

Every adapter translates external state into platform-owned typed records.

---

## 3. New adapter boundaries

Add explicit provider-neutral interfaces:

~~~text
WorkflowBackend
RuntimeBackend
ExecutionBackend
BuildBackend
AttestationBackend
DeviceLabBackend
ReleaseBackend
WorkspaceBackend
~~~

M1/M2 concrete implementations:

~~~text
WorkflowBackend
  -> Temporal

RuntimeBackend
  -> Codex
  -> future Claude

ExecutionBackend
  -> NativeUbuntu
  -> future SWE-ReX-inspired remote execution

BuildBackend
  -> Native/Legacy
  -> optional Dagger

AttestationBackend
  -> in-toto/SLSA-compatible statement model
  -> Sigstore/cosign implementation

DeviceLabBackend
  -> labgrid

ReleaseBackend
  -> Native/Project-specific
  -> optional RAUC
  -> future Mender/Fleet

WorkspaceBackend
  -> Native git worktree
  -> future Coder-backed workspace
~~~

Adapters may be optional; domain contracts are not.

---

## 4. Runtime/Session optimization from OpenHands + SWE-ReX

### Before

Risk:
- over-design a custom Runtime Gateway;
- mix provider logic with shell/session transport;
- duplicate already solved remote-shell/session patterns.

### Optimized boundary

~~~text
Control Plane
  -> Session Gateway
  -> Session Supervisor
      -> Runtime Adapter
      -> Execution Backend
~~~

#### Runtime Adapter owns
- provider launch/resume arguments;
- provider-native session IDs;
- provider metadata;
- permission-mode translation;
- provider stdout/event parsing.

#### Session Supervisor owns
- formal Run/Attempt;
- execution epoch;
- structured Steering;
- Human Takeover;
- Checkpoint;
- stream ownership;
- durable event emission;
- external side-effect cursor.

#### Execution Backend owns
- process/shell lifecycle;
- PTY;
- remote/local execution;
- command completion;
- transport.

### M0 action

Study and prototype against:
- OpenHands Agent Server REST/WebSocket contract;
- OpenHands workspace/conversation/events;
- SWE-ReX shell/session backend interface.

Do **not** copy their Agent authority semantics.

---

## 5. Worker/workspace optimization from Coder

Introduce a clearer Worker contract.

### Worker registration

~~~yaml
worker_id:
trust_class:
protocol_versions:
labels:
capacity:
toolchains:
attached_resources:
project_scopes:
environment_scopes:
software_version:
heartbeat:
~~~

### Workspace contract

~~~yaml
workspace_id:
run_id:
backend:
source_snapshot:
mounts:
resource_limits:
network_profile:
lifecycle_state:
~~~

M1 backend:

~~~text
NativeUbuntuWorkspace
  -> git worktree
  -> rootless sandbox where possible
~~~

Future backend:

~~~text
CoderWorkspaceBackend
~~~

No domain change is required to adopt Coder later.

### Credential optimization

Adopt Coder-like central model credential governance:

~~~text
Runtime process
  -> model/runtime gateway or scoped provider session
  -> no reusable provider secret stored in workspace
~~~

---

## 6. Workflow optimization: Temporal is mandatory reference backend

Remove any remaining temptation to build a homegrown durable scheduler.

### PostgreSQL owns
- aggregate state;
- versions;
- decisions;
- authoritative relationships;
- outbox.

### Temporal owns
- waits;
- retries;
- orchestration;
- timers;
- cancellation;
- activity recovery.

### Workflow contract

Every workflow/activity receives stable IDs, not mutable object blobs as authority:

~~~text
workflow input:
  organization_id
  project_id
  aggregate_id
  command/correlation ID
  expected revision/digest
~~~

Workflow always reloads authoritative business state from Control Plane/domain store where necessary.

### M0 study

Use Temporal Go samples for:
- Signals/Updates;
- Activity heartbeat;
- cancellation;
- Continue-As-New;
- versioning;
- long human waits.

---

## 7. Evidence optimization: in-toto/SLSA-compatible core

This is the biggest semantic reuse opportunity.

### Internal engineering record

Keep rich platform-native records:
- Artifact;
- Evidence;
- Transform Receipt;
- Subject Manifest;
- Verification.

### Add standard attestation projection

Allow relevant records to project into a standard attestation form:

~~~text
subject:
  exact artifact digest(s)

predicate/type:
  build
  test
  transform
  provenance
  custom engineering evidence

issuer:
  trusted identity

statement:
  canonical signed payload
~~~

Where suitable, align with:
- in-toto Statement concepts;
- SLSA provenance predicates;
- Sigstore verification/signing ecosystem.

### Do not

- replace Requirement/Run/Target with in-toto;
- force every Human Decision into supply-chain attestation;
- make external standards the mutable business database.

### Benefit

This gives:
- ecosystem compatibility;
- independent verification;
- less proprietary cryptography;
- easier archival/export.

---

## 8. Trust/signing optimization: Sigstore/cosign implementation profile

Create abstract platform interface:

~~~text
AttestationBackend
  Sign(subjectDigest, statement, signingProfile)
    -> SignedAttestation

  Verify(signedAttestation, trustPolicy)
    -> VerificationResult
~~~

Initial implementation options:
- enterprise KMS/HSM + cosign;
- enterprise PKI;
- Sigstore keyless where organizational policy permits.

### Hard rules

- signing exact digest;
- key material outside Runtime/ordinary Worker;
- trust root/version stored in issuer registry;
- verification performed before authority is granted;
- offline archival verification supported for release-grade evidence when required.

---

## 9. Device/HIL optimization: labgrid-first backend

Do not build low-level board-control infrastructure in engineering-platform.

### engineering-platform owns

~~~text
Device identity
Target Revision
Lease
Fencing token
Procedure Revision
Subject Manifest
Evidence Issuer
Verification
Quarantine
~~~

### labgrid owns

~~~text
Remote resource routing
Exporter/coordinator
Serial
SSH
Power/reset
USB
Bootloader interaction
Mux
Measurement resources
pytest hardware access
~~~

### Adapter

~~~text
Device Action Gateway
  -> LabgridAdapter
     -> Coordinator/Exporter
        -> Resource/Driver/Target
~~~

### Mapping proposal

| engineering-platform | labgrid concept |
|---|---|
| Device/Fixture resource | Resource |
| Device Agent/export host | Exporter |
| resource arbitration backend | Coordinator |
| test target | Target |
| low-level action | Driver |
| reserved execution context | Place/reservation-style resource allocation |

The platform lease/fencing token remains authoritative even if labgrid also has reservation semantics.

### M2 PoC

Use one representative lane:
- motor MCU board/HIL;
- serial;
- power/reset;
- flash path;
- one measurement/log source.

Success criterion:
engineering-platform can prove exact Target + Artifact + Procedure + labgrid resource identity + raw Evidence.

---

## 10. Build optimization: dual backend instead of one-size-fits-all

Define a provider-neutral BuildBackend.

### NativeLegacyBuildBackend

For:
- vendor BSP;
- old cross-compilers;
- host-bound SDKs;
- special licenses;
- hardware-dependent build tooling.

Must still emit:
- build environment profile;
- source digest;
- config/toolchain versions;
- resolved dependencies where possible;
- Artifact digest.

### DaggerBuildBackend

For modern/reproducible builds:
- container-compatible;
- typed inputs;
- content-addressed execution;
- local/CI parity;
- OTel.

### Policy

Assurance Profile may require minimum reproducibility class:

~~~text
A0/A1 -> RECORDED_LEGACY acceptable by policy
A2+   -> CONTROLLED preferred
A3/A4 -> stronger build/provenance requirement
~~~

Exact thresholds stay policy-configurable.

---

## 11. OTA optimization: RAUC backend profile

For Linux-based targets where product layout fits:

~~~text
Release Manifest
  -> ReleaseBackend
     -> RAUC Adapter
        -> signed RAUC bundle
        -> target
~~~

### engineering-platform owns

- Release authority;
- Release Bundle digest;
- Target Revision;
- compatibility decision;
- migration/rollback policy;
- Human Authority;
- Evidence/Verification.

### RAUC owns

- bundle format;
- target installation mechanics;
- slot/update mechanics;
- bootloader integration;
- bundle cryptographic checks.

### Important

RAUC compatibility checks complement but do not replace platform Target/Compatibility policy.

### Future

Mender may become a FleetReleaseBackend for phased production deployments.

---

## 12. Catalog/template optimization from Backstage

Do not add Backstage UI.

Borrow only data-model ideas.

Potential future catalog projection:

~~~text
Entity
  kind:
    Repository
    Target
    WorkerPool
    DevicePool
    Procedure
    Skill
    RuntimeProfile

  metadata:
    owner
    labels
    lifecycle

  relations:
    dependsOn
    partOf
    provides
    consumes
~~~

This is a read/catalog projection, not the engineering business authority.

For M4 Task/Skill templates, borrow:
- parameter schema;
- typed steps;
- ownership;
- versioning.

---

## 13. Sandbox optimization from E2B/Daytona

Do not adopt Daytona public repo as a dependency; it is unmaintained.

Use E2B/Daytona only to validate a provider-neutral Sandbox contract:

~~~text
create
exec
interactive session
filesystem
snapshot
network policy
resource limits
destroy
~~~

M1 concrete backend remains Native Ubuntu/rootless isolation.

Future cloud backends can implement the same contract without changing Run semantics.

---

## 14. Revised M0 priorities

Open-source review changes M0 implementation order.

### M0-A — Domain and standards

1. Canonical schema/digest profile.
2. Artifact/Evidence/Transform schemas.
3. in-toto/SLSA-compatible attestation projection.
4. Trust Root / issuer schema.
5. Sigstore/cosign implementation ADR.

### M0-B — Runtime/session

6. RuntimeBackend contract.
7. ExecutionBackend contract.
8. Session Gateway protocol.
9. Session Supervisor state/guard model.
10. OpenHands/SWE-ReX comparison spike.

### M0-C — Worker/workspace

11. Worker enrollment/capability schema.
12. WorkspaceBackend contract.
13. Native Ubuntu backend.
14. Coder design comparison ADR.

### M0-D — Workflow

15. Temporal workflow conventions.
16. PostgreSQL/outbox/Temporal transaction boundary.
17. retry/cancellation/versioning rules.

### M0-E — Embedded

18. DeviceLabBackend contract.
19. labgrid adapter spike.
20. ReleaseBackend contract.
21. RAUC compatibility spike.

---

## 15. Revised M1 scope

M1 keeps breadth narrow.

Concrete stack:

~~~text
WorkBuddy Connector
  -> Go Control API
  -> PostgreSQL
  -> Temporal
  -> Session Gateway
  -> Native Ubuntu Worker
  -> Native WorkspaceBackend
  -> Codex RuntimeBackend
  -> Native/Legacy BuildBackend
  -> Git/CI
  -> in-toto-compatible Evidence projection
  -> Sigstore/cosign optional signing PoC
  -> Verification
  -> Closure
~~~

Do not require:
- Coder;
- OpenHands runtime server;
- SWE-ReX;
- Dagger;
- labgrid;
- RAUC

inside M1 unless a spike proves clear reduction of implementation risk.

They are reference/integration tracks, not M1 blockers.

---

## 16. Revised M2 scope

M2 embedded loop should preferentially become:

~~~text
Control Plane
  -> DeviceLabBackend
     -> labgrid
  -> exact firmware Artifact
  -> flash / power / serial / measurement
  -> Raw Evidence
  -> in-toto-compatible attestation
  -> Verification

Release candidate
  -> optional RAUC ReleaseBackend
  -> final-byte/device verification
~~~

This is expected to remove significant low-level device-infrastructure code from engineering-platform.

---

## 17. Build-vs-buy/borrow rule

Before implementing a new infrastructure component, require an ADR section:

~~~text
Existing open-source candidates
Why reuse/integration is insufficient
Why custom implementation is required
Authority boundary preserved
Exit/replacement strategy
~~~

Apply this rule especially to:
- sandbox;
- workflow;
- device control;
- signing;
- provenance;
- OTA;
- remote workspace;
- agent session transport.

---

## 18. Dependency policy

External projects are implementation dependencies only when:
- actively maintained;
- license acceptable;
- security/update process acceptable;
- boundary is replaceable;
- platform authority remains internal;
- data migration/exit is understood.

Every external dependency gets:
- adapter boundary;
- version compatibility policy;
- failure/degradation behavior;
- observability;
- upgrade test.

---

## 19. Source-of-truth policy

Never let an integration become hidden authority.

Examples:

~~~text
Temporal Workflow status
!= Work status authority

labgrid reservation
!= engineering Device Lease authority

RAUC installation state
!= Release authority

Coder workspace state
!= Run authority

OpenHands conversation
!= Requirement/Run authority

Cosign signature
!= business approval
~~~

External state is reconciled into the domain.

---

## 20. Final optimized architecture

The optimized implementation keeps the v1.2 architecture but makes mechanism reuse explicit:

~~~text
WorkBuddy
  |
  v
Engineering Control Plane
  |-- PostgreSQL
  |-- Temporal
  |
  +--> Session Gateway
  |      -> Session Supervisor
  |          -> Codex/Claude Runtime Adapter
  |          -> Native/SWE-ReX-style Execution Backend
  |
  +--> WorkspaceBackend
  |      -> Native Ubuntu/worktree
  |      -> future Coder
  |
  +--> BuildBackend
  |      -> Native Legacy
  |      -> optional Dagger
  |
  +--> AttestationBackend
  |      -> in-toto/SLSA-compatible statements
  |      -> Sigstore/cosign/KMS
  |
  +--> DeviceLabBackend
  |      -> labgrid
  |
  +--> ReleaseBackend
         -> project-specific
         -> optional RAUC
         -> future Mender

All adapters return typed immutable records to:
Artifact -> Evidence -> Verification -> Review -> Human Authority -> Release
~~~

---

## 21. Expected reduction in custom code

The platform should **not** custom-build:
- durable workflow engine;
- generic serial/power/USB-mux lab framework;
- cryptographic signing stack;
- general provenance specification;
- every reproducible build primitive;
- embedded Linux updater;
- generic cloud workspace platform.

The platform should custom-build:
- engineering domain contracts;
- authority and policy semantics;
- Run/Attempt/Steering/Takeover governance;
- Target/Compatibility model;
- Evidence applicability;
- Verification and risk gates;
- cross-backend reconciliation;
- WorkBuddy/engineering UX;
- adapters.

This is the principal optimization derived from the open-source review.

---

## 22. Decision

This profile is the canonical implementation companion to v1.2.

The architecture remains v1.2; implementation should now proceed with:
- **own authority, reuse mechanisms**;
- standards-compatible Evidence;
- adapter-first external integrations;
- no unnecessary infrastructure reinvention.
