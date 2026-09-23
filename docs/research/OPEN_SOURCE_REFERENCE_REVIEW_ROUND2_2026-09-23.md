# Open Source Reference Review — Round 2

Date: 2026-09-23
Status: **Archived research / implementation input**
Parent:
- docs/architecture/AI_NATIVE_ENGINEERING_PLATFORM_V1_2_FINAL.md
- docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V1.md

## 1. Scope

This round broadened the review beyond agent/runtime/workflow/device basics into:

1. authorization/policy;
2. workload identity;
3. secrets/credential lifecycle;
4. CI attestation controllers;
5. build/CAS/remote execution;
6. software supply-chain graph/SBOM/vulnerability;
7. hardware-test execution models;
8. MCU/OTA security;
9. workspace specifications;
10. failure injection;
11. coding-agent UX/session practices.

The goal remains selective reuse, not replacing engineering-platform's domain authority.

---

## 2. Policy and authorization

### Open Policy Agent
Repository: https://github.com/open-policy-agent/opa

OPA is a general-purpose context-aware policy engine. It separates policy rules from application code and supports service integration through Go SDK/REST and other deployment modes.

Relevant to engineering-platform:
- actor/action/resource/environment decisions;
- capability enforcement;
- policy bundle versioning;
- policy-as-code;
- external enforcement rather than Runtime self-governance.

### Cedar
Repository: https://github.com/cedar-policy/cedar

Cedar is purpose-built for authorization and emphasizes:
- fine-grained authorization;
- RBAC/ABAC;
- principal/action/resource entities;
- schema validation;
- analyzability and automated reasoning.

Relevant to engineering-platform:
- Human Authority;
- capability/resource authorization;
- schema-validated authorization model;
- policy analysis/testing.

### OpenFGA
Repository: https://github.com/openfga/openfga

OpenFGA is a Zanzibar-inspired relationship-based authorization engine.

Relevant when relationships become large/complex:
- user/team/project/resource ownership;
- delegated access;
- nested groups/resources.

### Decision

Do not hard-code a custom policy language.

Add two logical boundaries:

~~~text
AuthorizationEngine
  -> permit / deny for actor-action-resource-context

AssurancePolicyEngine
  -> engineering requirements:
     approval needed
     verification class
     reviewer independence
     issuer trust
     release gates
~~~

One implementation may serve both, but the domain distinction matters.

M0 spike:
- compare OPA and Cedar for AuthorizationEngine;
- keep PostgreSQL relationship data initially;
- defer OpenFGA unless relation scale/complexity justifies it.

OPA is operationally attractive for a Go control plane.
Cedar is especially valuable as an authorization-semantics/analyzability reference.

---

## 3. Workload identity — SPIFFE/SPIRE

Repository: https://github.com/spiffe/spire

SPIRE provides workload attestation and SPIFFE identities/SVIDs. Workloads can use those identities for mTLS, JWT authentication, secret-store/database/cloud authentication, and automatic identity rotation.

### Major optimization

engineering-platform should not create a bespoke long-lived machine certificate lifecycle for every service/worker.

Add:

~~~text
WorkloadIdentityBackend
~~~

Candidate implementation:
- SPIFFE/SPIRE.

Potential identities:
- Control API;
- Session Gateway;
- Ubuntu Worker;
- Platform Action Gateway;
- trusted CI/verifier;
- lab exporter/Device Agent host;
- signing/provenance service.

### Boundary

SPIRE proves workload identity.

It does not decide:
- engineering capability;
- Human Authority;
- Verification acceptance;
- Release policy.

Identity feeds AuthorizationEngine.

DUT MCUs themselves are not required to run SPIFFE.

---

## 4. Secret lifecycle — OpenBao

Repository: https://github.com/openbao/openbao

OpenBao provides:
- secure secret storage;
- dynamic credentials;
- leases;
- renewal;
- revocation;
- encryption services;
- audit-oriented secret access.

### Decision

Add:

~~~text
SecretBackend
~~~

OpenBao is the default open-source candidate behind Credential Broker.

Preferred pattern:

~~~text
Runtime
 -> Platform Action Gateway
 -> Credential Broker
 -> OpenBao dynamic/leased credential
 -> external service
~~~

Runtime should not receive reusable credentials when an action proxy can avoid it.

Use OpenBao lease/revocation semantics to implement:
- Run-bound credentials;
- Attempt/takeover revocation;
- short TTL;
- emergency revocation.

---

## 5. Independent attestation controller — Tekton Chains practice

Repository: https://github.com/tektoncd/chains

Tekton Chains observes completed TaskRuns/PipelineRuns, snapshots them, converts snapshots into standard payloads, signs them, and stores attestations.

### Important pattern to absorb

Attestation creation should be **post-execution and independently controlled**.

Recommended engineering-platform pattern:

~~~text
Build/Test Executor
  -> completion record
  -> Attestation Controller
       -> reload exact subject/results
       -> canonical snapshot
       -> standard attestation projection
       -> sign with trusted issuer
       -> store Evidence/Attestation
~~~

This prevents user-controlled build/test steps from self-declaring trusted provenance.

Do not require Tekton/Kubernetes.
Absorb the controller pattern.

---

## 6. BuildKit + Remote Execution API

Repositories:
- https://github.com/moby/buildkit
- https://github.com/bazelbuild/remote-apis
- https://github.com/buildbuddy-io/buildbuddy

### Useful practices

Remote Execution API formalizes:
- content-addressable input storage;
- execution request;
- result cache;
- remote worker execution;
- streaming stdout/stderr;
- digest-oriented assets.

BuildBuddy demonstrates rich build invocation records:
- initiator;
- flags/options/environment;
- target/test result;
- artifacts;
- timing;
- raw build events/logs.

### Optimization

Strengthen Build Receipt schema:

~~~text
Build Receipt
  source/input digests
  action/build definition digest
  build environment
  explicit flags/config
  relevant environment variables
  executor identity
  stdout/stderr/log refs
  timing/resource usage
  output Artifact digests
  cache-hit/provenance info
~~~

Add optional future:
- RemoteBuildBackend compatible with CAS/action-digest concepts.

Do not require Bazel or remote execution in M1.

BuildKit/Dagger remain useful controlled-build backends; legacy embedded builds remain first-class.

---

## 7. Supply-chain graph, SBOM and vulnerability impact

### GUAC
Repository: https://github.com/guacsec/guac

GUAC ingests and relates:
- CycloneDX;
- SPDX;
- in-toto;
- SLSA;
- OSV;
- Scorecard;
- VEX and related metadata.

It is designed as an aggregation/synthesis graph for audit, policy and risk analysis.

### Syft
Repository: https://github.com/anchore/syft

Syft generates SBOMs for filesystems/images and supports:
- SPDX;
- CycloneDX;
- signed SBOM attestations aligned with in-toto.

### OSV-Scanner
Repository: https://github.com/google/osv-scanner

OSV-Scanner maps dependencies to OSV vulnerability data and supports:
- multiple language ecosystems;
- Linux OS packages;
- container images;
- C/C++ and vendored-code scanning.

### Decision

Add optional:

~~~text
SupplyChainIntelBackend
~~~

M2/M3 candidate stack:
- Syft -> SBOM Artifact;
- OSV-Scanner -> Vulnerability Evidence;
- GUAC -> derived/query graph for cross-release impact.

Critical boundary:

~~~text
GUAC graph
!= engineering business authority
~~~

It is a read/analysis projection.

Use cases:
- new CVE -> affected Artifact -> Release -> Target;
- revoked dependency -> affected builds/releases;
- SBOM coverage;
- release security review.

Do not add graph DB to core domain solely because GUAC uses graph semantics.

---

## 8. Device/HIL: split resource control from test execution

Previous review selected labgrid as Device/HIL backend candidate.

Round 2 adds test-framework references.

### OpenHTF
Repository: https://github.com/google/openhtf

Useful concepts:
- DUT;
- Test;
- test run vs test recipe;
- Station;
- Phase;
- Measurement + pass specification;
- Attachment;
- Plug.

These map well to engineering-platform:

| OpenHTF | engineering-platform |
|---|---|
| DUT | Device / Target instance |
| test recipe | Procedure Revision |
| test run | Verification execution / Evidence production |
| Phase | Procedure step |
| Measurement | Raw/evaluated metric |
| Attachment | Raw Evidence Artifact |
| Plug | Test equipment adapter |
| Station | Test station / Device Agent host |

### LAVA
Repository: https://github.com/linaro/lava

LAVA is a CI system for deploying OS images to physical/virtual hardware and running tests.

Useful as a reference for:
- large shared device farms;
- job scheduling;
- deployment/test jobs;
- physical hardware CI.

It is heavier than labgrid for our immediate lab-control scope.

### pytest-embedded
Repository: https://github.com/espressif/pytest-embedded

Useful pattern:
- modular embedded test services;
- serial;
- JTAG/OpenOCD/GDB;
- auto-flash;
- QEMU/emulator;
- target-specific plugins.

### Major optimization

Split:

~~~text
DeviceLabBackend
  -> resource reservation/routing/control
  -> labgrid candidate

TestProcedureBackend
  -> test recipe/phase/measurement execution
  -> pytest/OpenHTF-compatible patterns
~~~

Do not force resource management and test logic into one Device Agent abstraction.

Future large farm:
- evaluate LAVA backend.

---

## 9. MCU update and OTA security

### MCUboot
Repository: https://github.com/mcu-tools/mcuboot

MCUboot provides a portable secure bootloader/update foundation for MCUs and includes signed-image tooling and simulator support.

Use as:
- MCU ReleaseBackend reference;
- secure image/boot state reference;
- potential future standardization candidate where hardware/project permits.

Do not force existing MCU products to migrate in M1/M2.

### Uptane
Repository: https://github.com/uptane/uptane-standard

Uptane is a compromise-resilient software-update security standard designed for automotive systems.

Use as a high-assurance design reference for:
- update metadata separation;
- trust/key-role separation;
- compromise resilience;
- deployment best practices.

Do not implement full Uptane unless product risk justifies it.

### SWUpdate / hawkBit

Repositories:
- https://github.com/sbabic/swupdate
- https://github.com/eclipse-hawkbit/hawkbit

Keep as additional candidates for:
- embedded Linux updater;
- rollout/fleet update backend.

RAUC remains the cleaner immediate Linux OTA reference for M2.

### Optimized ReleaseBackend taxonomy

~~~text
ReleaseBackend
  LinuxTarget:
    RAUC / SWUpdate / project-specific

  MCUTarget:
    current project OTA
    future MCUboot-compatible backend

  FleetRollout:
    Mender / hawkBit / enterprise backend

UpdateSecurityProfile:
    normal
    high-assurance / Uptane-inspired
~~~

---

## 10. Development environment standard — Dev Containers

Repository: https://github.com/devcontainers/spec

Dev Containers defines portable structured development-environment metadata via devcontainer.json and supports reuse across local development and CI.

### Decision

Add optional workspace-environment reference:

~~~text
WorkspaceEnvironmentSpec
  kind:
    native-profile
    devcontainer
    immutable-image
~~~

For modern projects, devcontainer metadata can become part of environment definition/provenance.

Do not force legacy embedded BSP/SDK projects into containers.

---

## 11. Failure injection — Toxiproxy and Chaos Mesh

### Toxiproxy
Repository: https://github.com/Shopify/toxiproxy

Toxiproxy provides deterministic network fault injection:
- latency;
- disconnect/down;
- timeout;
- bandwidth limits;
- slow close;
- reset/packet loss.

### Chaos Mesh
Repository: https://github.com/chaos-mesh/chaos-mesh

Chaos Mesh provides broader Kubernetes fault injection:
- network;
- DNS;
- HTTP;
- I/O;
- time;
- CPU/stress;
- kernel;
- cloud and node faults.

### Decision

Use Toxiproxy directly in M0/M1 failure-injection tests for:
- PostgreSQL proxy path where safe/test-only;
- Temporal;
- object store;
- Git/CI mock;
- Session Gateway/Worker channels;
- external Release backend mocks.

Chaos Mesh is future optional if platform deployment becomes Kubernetes-heavy.

This turns failure model prose into repeatable tests.

---

## 12. Coding-agent practice review

### OpenCode
Repository: https://github.com/anomalyco/opencode

Useful practice:
- distinct plan vs build modes;
- plan defaults read-only and asks permission for shell;
- build has broader development access.

### Aider
Repository: https://github.com/Aider-AI/aider

Useful practices:
- repository map for large-codebase context;
- explicit Git integration;
- lint/test feedback loop after edits.

### Cline
Repository: https://github.com/cline/cline

Useful practices:
- human-in-the-loop tool approval;
- checkpoints;
- diff/revert;
- long-running process observation;
- CLI/headless/SDK surfaces.

### Continue / Roo Code

Continue's repository states it is no longer actively maintained/read-only.
Roo Code's repository is archived and states the product/extension was shut down.

Keep as historical reference only.

### Optimizations

Add Runtime capability profiles:

~~~text
PLAN
  read-only source
  analysis/search
  restricted shell

BUILD
  workspace write
  bounded local shell/tools

DEBUG
  workspace write
  debug/device capabilities by policy

REVIEW
  reviewed subject read-only
  test/query execution
  no subject mutation
~~~

These are platform capability profiles, not agent-defined authority.

Add optional Context Resolver artifact:

~~~text
Repository Map
  generated context artifact
  source tree digest
  generator/version
  non-authoritative
~~~

Local lint/test loop generates Engineering Checks, not Verification authority.

---

## 13. New reference classification

### Direct/strong adoption candidates
- Temporal;
- SPIFFE/SPIRE for workload identity;
- OpenBao for secret backend;
- Toxiproxy for deterministic network failure tests.

### Strong integration/standards candidates
- labgrid;
- in-toto/SLSA/Sigstore;
- Syft/OSV;
- optional GUAC read model.

### M0 spikes
- OPA vs Cedar;
- OpenHands/SWE-ReX/Cline session patterns;
- OpenHTF/pytest-embedded Procedure backend;
- BuildKit/Dagger/Remote API BuildBackend semantics.

### M2+ backend candidates
- RAUC/SWUpdate;
- MCUboot;
- Mender/hawkBit;
- LAVA.

### Concept/reference only
- OpenFGA until relationship complexity proves need;
- Backstage;
- E2B;
- Uptane high-assurance profile;
- Dev Containers for compatible projects;
- BuildBuddy invocation UX/read-model ideas.

### Historical only
- Daytona public repo;
- Continue;
- Roo Code.

---

## 14. Main architectural lessons

### A. Separate identity from authorization

~~~text
SPIFFE/SPIRE
  -> who is this workload?

OPA/Cedar
  -> may this actor/workload perform this action?

engineering-platform
  -> is this engineering action valid for this Run/Subject/Assurance state?
~~~

### B. Separate resource control from test semantics

~~~text
labgrid
  -> where/how to reach and control hardware

OpenHTF/pytest
  -> how to execute Procedure and collect measurements

engineering-platform
  -> why this Evidence is authoritative for this Subject
~~~

### C. Attestation should be independently produced

~~~text
Executor finishes
  -> Attestation Controller observes immutable result
  -> creates canonical statement
  -> trusted signer signs
  -> Verification consumes it
~~~

### D. Supply-chain security needs both point evidence and impact graph

~~~text
Artifact/Evidence/Manifest = authority-bearing point records
GUAC/SBOM/OSV graph       = derived impact/read model
~~~

### E. Build records should be invocation-complete

Build authority should preserve:
- inputs;
- environment;
- flags;
- dependencies;
- executor;
- logs;
- outputs;
- provenance.

### F. Agent "modes" are really capability profiles

Do not trust an agent's internal mode flag.
Control Plane grants the actual capabilities.

---

## 15. Conclusion

Round 2 reinforces the v1.2 architecture and expands the implementation reuse strategy.

The largest new opportunities to reduce bespoke infrastructure are:

1. **SPIFFE/SPIRE** — workload identity;
2. **OpenBao** — dynamic secret lifecycle;
3. **OPA/Cedar** — policy engine instead of custom policy DSL;
4. **Tekton Chains pattern** — independent post-execution attestation controller;
5. **labgrid + OpenHTF/pytest split** — hardware resource control separate from test procedure;
6. **Syft/OSV/GUAC** — SBOM/security impact analysis;
7. **Toxiproxy** — deterministic failure-injection tests;
8. **MCUboot/RAUC/Uptane practices** — update security without inventing every mechanism.

The implementation principle remains:

> **Own engineering authority; reuse mature identity, policy, execution, attestation, secret, device and update mechanisms behind replaceable interfaces.**
