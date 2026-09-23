# Open Source Reference Review — Engineering Platform

Date: 2026-09-23
Status: **Archived research / implementation input**
Architecture baseline: `docs/architecture/AI_NATIVE_ENGINEERING_PLATFORM_V1_2_FINAL.md`

## 1. Purpose

This document archives the GitHub review of mature open-source implementations relevant to the AI Native Engineering Platform.

The result is intentionally **not** "pick one project and fork it".

No reviewed project covers the complete engineering authority chain:

```text
Requirement
-> Formal Work / Task / Run
-> Interactive AI Runtime
-> Artifact / Evidence
-> Verification / Review
-> Target / Device / HIL
-> Human Authority
-> Release / Closure
```

The useful strategy is therefore layered reference and selective reuse.

---

## 2. Executive reference map

| Priority | Project | Best fit in engineering-platform | Strategy |
|---|---|---|---|
| S | OpenHands/OpenHands + OpenHands/software-agent-sdk | Runtime / Session / Workspace / Events | Deep design reference; selective reuse |
| S | coder/coder | Worker / Workspace / Identity / AI Gateway | Deep design reference; optional future substrate |
| S | SWE-agent/SWE-ReX | Session Supervisor / Remote Execution | Deep design reference |
| S | temporalio/temporal | Durable Workflow | Direct infrastructure adoption |
| S | in-toto/in-toto | Artifact / Evidence / Issuer / supply-chain graph | Align domain semantics / standards compatibility |
| S | labgrid-project/labgrid | Device / HIL resource control | Strong PoC integration candidate |
| A | sigstore/cosign | Trust root / Signing / Attestation | Reuse implementation mechanisms |
| A | dagger/dagger | Reproducible Build / typed artifacts / OTel | Optional controlled-build backend |
| A | rauc/rauc | Embedded OTA / bundle / compatibility / rollback | Linux OTA backend/reference candidate |
| A | backstage/backstage | Catalog / Template concepts | Conceptual reference only |
| B | e2b-dev/E2B | Sandbox lifecycle/API | Sandbox API reference |
| B | mendersoftware/mender | Fleet OTA / phased deployments | Future production-fleet reference |
| C | daytonaio/daytona | Sandbox/snapshot historical design | Historical reference only; public repo unmaintained |

---

## 3. OpenHands / Software Agent SDK

Repositories:
- https://github.com/OpenHands/OpenHands
- https://github.com/OpenHands/software-agent-sdk

### Relevant design

OpenHands separates:
- Agent Canvas / developer control center;
- Agent Server / canonical execution API;
- agents, tools, conversations, workspaces and events;
- REST/WebSocket access;
- scheduling/automation in a separate component.

The Software Agent SDK explicitly supports:
- local or ephemeral workspaces;
- remote Agent Server;
- Python/TypeScript/REST APIs;
- conversations;
- tools;
- workspaces;
- events;
- WebSocket access.

### What to absorb

Use it to validate our separation:

```text
Runtime Adapter
!= Session Supervisor
!= Session Gateway
!= Workspace
!= Workflow
```

Study in M0/M1:
- conversation/session identity;
- REST vs WebSocket boundary;
- remote execution API;
- workspace abstraction;
- event stream;
- backend/provider selection.

### What not to absorb

Do not make OpenHands:
- Requirement authority;
- Verification authority;
- Release authority;
- Target/HIL authority.

It is an Agent Runtime reference, not the Engineering Control Plane.

---

## 4. Coder

Repository:
- https://github.com/coder/coder

### Relevant design

Coder provides:
- self-hosted workspaces;
- workspace lifecycle/templates;
- secure remote access;
- AI coding agents;
- AI Gateway;
- identity on actions;
- centralized model governance;
- cost/audit controls.

A particularly relevant boundary is keeping model/API credentials out of ordinary workspaces while centralizing model access.

### What to absorb

Study:
- worker/workspace registration;
- lifecycle;
- secure tunnels;
- template/provider boundary;
- resource inventory;
- identity propagation;
- AI Gateway governance;
- cost/audit model.

### What not to absorb

Do not require Coder as M1 infrastructure.

Embedded development often depends on:
- existing Ubuntu hosts;
- vendor SDKs;
- USB/UART;
- local debug probes;
- HIL equipment.

Use Coder as a future Workspace Substrate option, not as core domain authority.

---

## 5. SWE-ReX

Repository:
- https://github.com/SWE-agent/SWE-ReX

### Relevant design

SWE-ReX explicitly separates agent logic from execution infrastructure and supports:
- local and remote shell environments;
- Docker/cloud/remotes;
- long-running interactive shell sessions;
- interactive CLIs such as gdb/ipython;
- multiple parallel shell sessions.

### What to absorb

Strong reference for:
- execution backend interface;
- shell/session lifecycle;
- command completion/result handling;
- interactive process handling;
- remote backend abstraction.

### Required engineering-platform extensions

Our Session Supervisor still owns:
- Run identity;
- Attempt fencing;
- structured Steering;
- Checkpoint;
- Human Takeover;
- external-operation reconciliation;
- provenance.

---

## 6. Temporal

Repository:
- https://github.com/temporalio/temporal

### Decision

**Adopt directly as the reference durable workflow engine.**

Keep architectural boundary:

```text
PostgreSQL = business/domain authority
Temporal   = durable orchestration
```

Use Temporal for:
- long waits;
- retries;
- cancellation;
- human intervention;
- CI waits;
- Device/HIL waits;
- activity failure;
- worker recovery.

Do not make Temporal Workflow state the engineering system of record.

Implementation study should focus on Temporal SDK/samples, not Temporal server internals:
- Signals;
- Updates;
- Activities;
- cancellation;
- heartbeats;
- Continue-As-New;
- workflow versioning.

---

## 7. in-toto

Repository:
- https://github.com/in-toto/in-toto

### Why it matters

in-toto models a supply chain as:
- owner-defined layout;
- authorized functionaries;
- steps;
- materials;
- products;
- signed link metadata;
- final verification.

This maps closely to our domain:

| in-toto | engineering-platform |
|---|---|
| Layout | Verification Plan / supply-chain policy |
| Step | Build / Test / Sign / Package transform |
| Functionary | Evidence Issuer / trusted executor |
| Material | Input Artifact |
| Product | Output Artifact |
| Link metadata | Evidence / Transform Receipt |
| Owner key | Trust Root / Human Authority |
| Verify | Verification |

### Decision

Do not reinvent incompatible supply-chain evidence semantics.

M0 should design our custom engineering domain so that Artifact/Evidence/Transform records can map to in-toto-style statements or attestations.

The platform domain remains richer because it also contains:
- Requirement;
- Run/Attempt;
- Target;
- Device/HIL;
- Risk;
- Human Authority;
- Closure.

---

## 8. Sigstore / Cosign

Repository:
- https://github.com/sigstore/cosign

### Relevant design

Cosign supports:
- digest-based signing;
- keyless signing;
- Fulcio/Rekor;
- hardware/KMS signing;
- BYO PKI;
- offline verification.

### Decision

Do not implement cryptography from scratch.

Use Sigstore/cosign, enterprise PKI or KMS/HSM implementations behind the platform's abstract:
- Trust Root;
- Evidence Issuer;
- Artifact Signature;
- Transform Receipt;
- Release Manifest signature.

Keep the domain provider-neutral.

Important retained invariant:

```text
mutable locator/tag != content identity
```

Sign/verify exact digests.

---

## 9. Dagger

Repository:
- https://github.com/dagger/dagger

### Relevant design

Dagger provides:
- programmable delivery;
- containerized execution;
- typed artifacts;
- content-addressed operations;
- repeatable local/CI behavior;
- OpenTelemetry tracing.

### Decision

Add Dagger as an **optional Controlled Build Backend**, not a mandatory build system.

Recommended fit:
- REPRODUCIBLE/CONTROLLED build profiles;
- modern Linux/container-friendly projects;
- deterministic CI/local parity.

Do not force legacy embedded BSP/toolchains into Dagger when they require:
- vendor SDK assumptions;
- host-bound tools;
- USB/license dongles;
- non-containerizable tooling.

Keep `RECORDED_LEGACY` as a first-class reproducibility class.

---

## 10. labgrid

Repository:
- https://github.com/labgrid-project/labgrid

### Relevant design

labgrid already provides:
- embedded board abstraction;
- remote client/exporter/coordinator architecture;
- resource allocation;
- serial/SSH/bootloader access;
- power/reset;
- binary upload/bootstrapping;
- USB/SD mux;
- measurement/audio/video resources;
- pytest integration.

### Decision

**Run an M2 PoC with labgrid as the default Device/HIL backend.**

Preferred architecture:

```text
Engineering Control Plane
  -> Device Registry / Lease / Fencing
  -> Device Action Gateway
  -> labgrid adapter
  -> labgrid coordinator/exporter/resources
  -> UART / power / USB / fixture / DUT
```

engineering-platform keeps authority for:
- Target Revision;
- lease/fencing;
- Procedure Revision;
- Evidence Issuer;
- Subject Manifest;
- Verification.

labgrid handles low-level hardware control.

This avoids rebuilding serial servers, power control, USB mux abstraction and remote board routing.

---

## 11. RAUC

Repository:
- https://github.com/rauc/rauc

### Relevant design

RAUC includes:
- signed update bundles;
- compatibility checks;
- fail-safe/atomic update;
- bootloader integration;
- A/B/recovery layouts;
- data migration;
- eMMC/UBI/raw NAND/raw NOR support;
- PKCS#11/HSM support.

### Decision

Treat RAUC as:
- a strong embedded Release/OTA semantics reference;
- a candidate Linux OTA backend where product constraints fit.

Do not let RAUC own engineering release authority.

Preferred relation:

```text
Engineering Platform Release Manifest
  -> authorized final Artifact/Bundle
  -> RAUC bundle/update backend
  -> target device
```

---

## 12. Mender

Repository:
- https://github.com/mendersoftware/mender

### Relevant design

Mender adds:
- fleet management;
- phased rollout;
- grouping;
- A/B rollback;
- monitoring/configuration;
- audit/reporting.

### Decision

Keep as a future production fleet deployment reference.

For M2 engineering/device validation, RAUC/labgrid are closer to the immediate scope.

---

## 13. Backstage

Repository:
- https://github.com/backstage/backstage

### Relevant design

Backstage is strongest in:
- Software Catalog;
- Software Templates;
- TechDocs;
- plugin architecture.

### Decision

Do not introduce Backstage as a second user-facing R&D portal next to WorkBuddy.

Absorb concepts only:
- Entity/Kind/Owner/Relation ideas for future catalog/read models;
- template schema/parameters/steps for future Skill/Task templates.

WorkBuddy remains the Experience Plane front door.

---

## 14. E2B

Repository:
- https://github.com/e2b-dev/E2B

### Relevant design

Useful as a reference for:
- sandbox lifecycle;
- process/filesystem APIs;
- code execution;
- snapshot;
- isolated agent execution.

### Decision

Reference its API/isolation model.

Do not require cloud sandbox infrastructure for embedded M1/M2.

---

## 15. Daytona

Repository:
- https://github.com/daytonaio/daytona

The public repository states that core development moved to a private codebase in June 2026 and that the public repository is no longer maintained.

### Decision

Historical design reference only.

Do not introduce a new hard dependency on the public repository.

---

## 16. Reference architecture map

```text
                         engineering-platform
                                  |
         +------------------------+-------------------------+
         |                        |                         |
     Experience               Runtime                  Execution
         |                        |                         |
   Backstage ideas          OpenHands SDK               Coder
                           SWE-ReX                    E2B reference
         |                        |                         |
         +------------------------+-------------------------+
                                  |
                              Workflow
                                  |
                               Temporal
                                  |
              +-------------------+-------------------+
              |                   |                   |
            Build              Evidence            Device/HIL
              |                   |                   |
           Dagger              in-toto              labgrid
                               Sigstore
              |                   |                   |
              +-------------------+-------------------+
                                  |
                         Embedded Release
                                  |
                           RAUC / Mender
```

---

## 17. Final reuse policy

### Direct infrastructure adoption
- Temporal.

### Strong integration PoC
- labgrid.

### Standard/protocol alignment
- in-toto;
- SLSA-style provenance;
- Sigstore/cosign.

### Deep design reference
- OpenHands Software Agent SDK;
- SWE-ReX;
- Coder.

### Optional backend
- Dagger;
- RAUC;
- future Mender.

### Concept-only reference
- Backstage;
- E2B.

### Historical-only reference
- Daytona.

---

## 18. Architectural conclusion

The review does **not** justify replacing the v1.2 five-plane architecture.

It does justify reducing custom implementation:

1. do not build a workflow engine: use Temporal;
2. do not build low-level embedded lab control from scratch: integrate labgrid where practical;
3. do not invent incompatible provenance/attestation formats: align with in-toto/SLSA/Sigstore;
4. do not design Agent Runtime/Session APIs without studying OpenHands SDK and SWE-ReX;
5. do not design Worker/workspace governance without studying Coder;
6. do not force every build into one abstraction: support Dagger as an optional controlled backend;
7. do not invent embedded Linux OTA semantics unnecessarily: use RAUC as a backend/reference where suitable.

The platform should own **engineering authority and contracts**, while mature open-source components own specialized execution mechanisms.
