# Reference-Aligned Implementation Profile v2

Date: 2026-09-23
Status: **Canonical implementation companion to architecture v1.2**
Supersedes: docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V1.md
Research:
- docs/research/OPEN_SOURCE_REFERENCE_REVIEW_2026-09-23.md
- docs/research/OPEN_SOURCE_REFERENCE_REVIEW_ROUND2_2026-09-23.md

## 1. What changed from v1

The v1 profile already established:
- Temporal as workflow backend;
- OpenHands/SWE-ReX/Coder as runtime/workspace references;
- in-toto/SLSA/Sigstore alignment;
- labgrid-first Device/HIL strategy;
- optional Dagger/RAUC backends.

Round 2 adds six implementation boundaries that materially reduce bespoke infrastructure:

1. WorkloadIdentityBackend
2. AuthorizationEngine + AssurancePolicyEngine split
3. SecretBackend
4. AttestationController
5. TestProcedureBackend
6. SupplyChainIntelBackend

It also upgrades failure injection and Runtime capability profiles.

---

## 2. Updated backend/interface map

~~~text
Engineering Control Plane
  |
  +-- WorkloadIdentityBackend
  |     -> SPIFFE/SPIRE candidate
  |
  +-- AuthorizationEngine
  |     -> OPA/Cedar candidate
  |
  +-- AssurancePolicyEngine
  |     -> engineering gate policy
  |
  +-- SecretBackend
  |     -> OpenBao candidate
  |
  +-- WorkflowBackend
  |     -> Temporal
  |
  +-- Session Gateway / Session Supervisor
  |     -> RuntimeBackend
  |     -> ExecutionBackend
  |
  +-- WorkspaceBackend
  |     -> Native Ubuntu
  |     -> future Coder/devcontainer-compatible
  |
  +-- BuildBackend
  |     -> NativeLegacy
  |     -> optional Dagger/BuildKit
  |     -> future Remote Execution backend
  |
  +-- AttestationController
  |     -> AttestationBackend
  |          -> in-toto/SLSA statement
  |          -> Sigstore/cosign/KMS/PKI
  |
  +-- DeviceLabBackend
  |     -> labgrid
  |
  +-- TestProcedureBackend
  |     -> pytest/OpenHTF-compatible executor
  |
  +-- SupplyChainIntelBackend
  |     -> Syft / OSV
  |     -> optional GUAC projection
  |
  +-- ReleaseBackend
        -> project-specific
        -> RAUC/SWUpdate
        -> MCUboot-compatible
        -> future Mender/hawkBit
~~~

No backend is allowed to become hidden business authority.

---

## 3. Workload identity

### Problem

v1 relied on Worker identity/enrollment/mTLS but left certificate issuance/rotation mostly platform-specific.

### v2 optimization

Add:

~~~text
WorkloadIdentityBackend
~~~

Default candidate:
- SPIFFE/SPIRE.

Use for:
- Control Plane services;
- Session Gateway;
- Worker;
- Platform Action Gateway;
- Device/lab host agents;
- trusted CI/verifier service;
- attestation/signing service.

Expected result:
- short-lived workload identity;
- automatic rotation;
- explicit trust domain;
- mTLS/JWT identity;
- no bespoke static worker certificates.

### Boundary

Workload identity answers:
> what authenticated workload is calling?

AuthorizationEngine answers:
> is it allowed to perform this action?

engineering-platform domain answers:
> is this action valid in current Run/Subject/Assurance state?

---

## 4. Authorization and assurance policy split

Do not overload a single generic PolicyEngine with every concept.

### AuthorizationEngine

Input:

~~~text
principal
action
resource
context
~~~

Output:

~~~text
PERMIT / DENY
policy bundle/version
reason/explanation
~~~

Candidates:
- OPA;
- Cedar.

M0 comparison criteria:
- Go integration;
- schema/type safety;
- policy analysis;
- testability;
- bundle/version deployment;
- decision explanation;
- latency;
- deny/fail-closed semantics.

### AssurancePolicyEngine

Engineering-specific domain rules, such as:
- Verification classes required;
- reviewer independence;
- issuer trust level;
- approval quorum;
- rollback requirements;
- evidence freshness;
- release gate.

It may internally use the same policy technology but keeps a separate domain contract.

### OpenFGA

Do not introduce in M1.

Use PostgreSQL relationships first.
Add OpenFGA only when relationship-based authorization complexity actually warrants it.

---

## 5. Credential Broker optimization

Add:

~~~text
SecretBackend
~~~

Default open-source candidate:
- OpenBao.

Credential Broker becomes orchestration/policy layer rather than secret database.

Pattern:

~~~text
Platform Action
  -> authorize
  -> request scoped credential/lease
  -> execute
  -> revoke/expire
~~~

Preferred:
- dynamic credentials;
- short lease;
- automatic revocation;
- per-run/action scope.

Avoid exposing secret values to Runtime where action proxy is possible.

---

## 6. Independent Attestation Controller

This is a new explicit component pattern.

### Why

Executor-generated evidence can be self-serving or compromised.

### Pattern

~~~text
Build/Test/Transform execution
  -> immutable completion/result record
  -> Attestation Controller
       1. verifies exact inputs/outputs
       2. reloads authoritative subject
       3. creates canonical snapshot
       4. projects standard attestation
       5. asks trusted signer
       6. publishes signed Evidence/Attestation
~~~

Inspired by Tekton Chains, but not dependent on Tekton.

### Authority

Attestation Controller does not decide release.
It produces trusted claims for Verification.

---

## 7. BuildBackend v2

Support three classes.

### NativeLegacyBuildBackend

For embedded vendor SDK/toolchains.

### ControlledBuildBackend

Dagger/BuildKit-style:
- typed inputs;
- content addressing;
- isolated execution;
- controlled secret mounts;
- reproducible environment;
- OTel.

### RemoteBuildBackend

Future optional backend influenced by Bazel Remote Execution API:
- CAS;
- action digest;
- remote execution;
- result cache;
- log stream.

### Build Receipt v2

Must retain:
- source/input digests;
- build/action definition;
- config/flags;
- relevant environment;
- toolchain/image;
- dependencies;
- executor identity;
- logs;
- timing/resource use;
- output digests;
- cache/provenance metadata.

---

## 8. Device/HIL v2: two independent backends

### DeviceLabBackend

Owns resource infrastructure:
- reservation routing;
- serial/power/USB;
- device reachability;
- low-level control.

Default candidate:
- labgrid.

### TestProcedureBackend

Owns Procedure execution semantics:
- phases/steps;
- measurements;
- attachments;
- pass/fail evaluation input;
- test runner lifecycle.

Candidates/reference:
- pytest;
- OpenHTF;
- pytest-embedded.

This prevents Device Agent from becoming an unmaintainable mix of:
- resource scheduler;
- hardware driver framework;
- test framework;
- Evidence authority.

### Future

LAVA is a large-farm backend reference if device scale grows significantly.

---

## 9. Procedure/measurement model refinement

Adopt useful OpenHTF-like vocabulary without importing its authority model.

Platform concepts:

~~~text
Procedure Revision
  -> Phase/Step
  -> Measurement Definition
  -> Attachment Definition
  -> required equipment capability
~~~

Execution creates:
- Raw Measurement;
- Raw Attachment Artifact;
- Procedure Execution Receipt.

Evaluation then produces Evidence.

This reinforces:
> measurement != evaluation != Verification.

---

## 10. Supply-chain intelligence

Add optional:

~~~text
SupplyChainIntelBackend
~~~

### Inputs
- SBOM;
- provenance;
- Artifact graph;
- vulnerability feeds;
- VEX;
- dependency metadata.

### Initial tools
- Syft for SBOM;
- OSV-Scanner for vulnerability Evidence.

### Optional derived graph
- GUAC.

### Use cases
- CVE impact across Releases/Targets;
- dependency revocation impact;
- security re-verification triggers;
- supply-chain completeness.

### Boundary

SupplyChainIntel is a derived read/risk model.

It cannot mutate Release authority directly.
A finding creates:
- Incident/Risk;
- Quarantine/Impact Decision;
- new Verification requirement;
according to policy.

---

## 11. Workspace environment specification

Add optional:

~~~text
WorkspaceEnvironmentSpec
~~~

Kinds:
- native_profile;
- devcontainer;
- immutable_image.

Dev Containers can standardize modern projects and improve local/CI parity.

Legacy BSP projects remain valid native profiles.

This allows gradual modernization without forcing containerization.

---

## 12. Runtime capability profiles

Borrow the useful pattern from OpenCode/Cline but enforce it outside the agent.

Define platform-owned profiles:

### PLAN
- source read;
- search/analysis;
- no workspace mutation;
- restricted shell.

### BUILD
- workspace write;
- bounded local shell/build/test;
- no privileged external actions without gateway policy.

### DEBUG
- BUILD plus explicitly granted debug/device actions.

### REVIEW
- reviewed subject read-only;
- test/query allowed;
- no subject mutation.

Runtime UI mode is merely a hint.
Actual permissions come from platform capability grant.

---

## 13. Repository Map context artifact

Borrow Aider's large-repository context idea.

Add optional generated:

~~~text
RepositoryMap Artifact
~~~

Properties:
- bound to source tree digest;
- generator/version;
- symbol/file relation summary;
- non-authoritative context;
- disposable/regenerable.

Use it to reduce model context cost without turning summarization into authority.

---

## 14. Engineering checks vs Verification

Local loops such as:
- lint;
- compiler diagnostics;
- fast unit tests;
- Runtime self-checks;

are:

~~~text
Engineering Checks
~~~

They improve iteration quality.

They are not automatically:

~~~text
Verification Evidence
~~~

unless executed through an approved Procedure/Issuer/Subject path.

This keeps the productivity patterns from Aider/Cline/OpenCode without weakening assurance.

---

## 15. Failure injection backend

Make failure injection a real M0/M1 facility.

### NetworkFaultBackend

Initial implementation:
- Toxiproxy.

Use for deterministic tests of:
- Worker reconnect;
- Session Gateway;
- object store;
- Temporal;
- Git/CI mock;
- Release backend mock;
- unknown-outcome reconciliation.

### Future
- Chaos Mesh if platform infrastructure becomes Kubernetes-heavy.

Failure injection cases become executable test fixtures, not manual exercises.

---

## 16. ReleaseBackend v2 taxonomy

### Linux target
Candidates:
- RAUC;
- SWUpdate;
- project-specific.

### MCU target
Candidates:
- current project-specific OTA;
- MCUboot-compatible future backend.

### Fleet rollout
Candidates:
- Mender;
- hawkBit;
- enterprise rollout system.

### High-assurance security profile
Use Uptane practices as a design benchmark for:
- role/key separation;
- compromise resilience;
- update metadata policy.

Do not mandate full Uptane unless product risk requires it.

---

## 17. OCI/Artifact interoperability

Use OCI-compatible artifact distribution where useful.

Reference:
- ORAS.

Potential stored/distributed objects:
- firmware bundles;
- SBOM;
- provenance;
- signed attestations;
- test packages;
- archived verification package.

Do not require every artifact to be OCI.
Keep Artifact Registry provider-neutral.

---

## 18. Revised M0 implementation order

### Track A — Identity / Policy / Secret
1. WorkloadIdentityBackend ADR + SPIRE spike.
2. AuthorizationEngine ADR: OPA vs Cedar.
3. AssurancePolicyEngine schema.
4. SecretBackend ADR + OpenBao spike.

### Track B — Runtime / Session
5. Runtime capability profiles.
6. RuntimeBackend / ExecutionBackend.
7. Session Gateway.
8. OpenHands/SWE-ReX/Cline spike.
9. RepositoryMap context artifact.

### Track C — Supply Chain
10. Artifact/Evidence schemas.
11. AttestationController.
12. in-toto/SLSA projection.
13. Sigstore backend.
14. SBOM/Vulnerability evidence schema.
15. Syft/OSV PoC.

### Track D — Build
16. Build Receipt v2.
17. NativeLegacy backend.
18. ControlledBuildBackend spike: Dagger/BuildKit.
19. future RemoteBuildBackend contract.

### Track E — Embedded
20. DeviceLabBackend + labgrid.
21. TestProcedureBackend + pytest/OpenHTF.
22. Procedure/Measurement/Attachment schemas.
23. ReleaseBackend taxonomy.
24. RAUC/MCUboot feasibility.

### Track F — Reliability
25. Toxiproxy failure-injection harness.
26. recovery/reconciliation property tests.

---

## 19. Dependency strategy v2

### Strong candidate dependencies
- Temporal;
- SPIFFE/SPIRE;
- OpenBao;
- Toxiproxy.

### Strong integration candidates
- labgrid;
- in-toto/SLSA/Sigstore;
- Syft/OSV.

### Spike before choosing implementation
- OPA vs Cedar;
- pytest/OpenHTF;
- Dagger vs BuildKit-style controlled backend.

### Future/optional
- GUAC;
- Coder;
- Remote Execution API backend;
- LAVA;
- RAUC/SWUpdate;
- MCUboot;
- Mender/hawkBit.

No dependency is accepted without:
- adapter boundary;
- version/upgrade contract;
- failure behavior;
- replacement path;
- license/security review.

---

## 20. Final design rule

The optimized platform now follows four layers of trust:

~~~text
Identity
  SPIRE / enterprise identity

Authorization
  OPA/Cedar-style deterministic policy

Engineering Authority
  engineering-platform domain state/policy

Evidence Authenticity
  trusted Attestation Controller + Sigstore/in-toto-compatible records
~~~

None of these layers should silently substitute for another.

The implementation principle remains:

> **Own engineering authority; reuse mature mechanisms behind explicit, replaceable, observable interfaces.**
