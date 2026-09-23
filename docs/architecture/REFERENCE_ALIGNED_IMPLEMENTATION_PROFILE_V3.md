# Reference-Aligned Implementation Profile v3

Date: 2026-09-23
Status: **Canonical implementation companion to architecture v1.2**
Supersedes: docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V2.md

Research basis:
- docs/research/OPEN_SOURCE_REFERENCE_REVIEW_2026-09-23.md
- docs/research/OPEN_SOURCE_REFERENCE_REVIEW_ROUND2_2026-09-23.md
- docs/research/OPEN_SOURCE_REFERENCE_SYNTHESIS_OPTIMIZATION_2026-09-23.md

## 1. Optimization goal

Profile v3 reduces framework surface area.

v2 correctly identified many mature open-source mechanisms, but too many plugin-style interfaces would make M0/M1 integration-heavy.

v3 follows:

> **Standardize authority/data early. Integrate infrastructure only when the milestone needs it.**

The v1.2 five-plane architecture and all domain invariants remain unchanged.

---

## 2. M1 concrete stack

Use a concrete stack instead of abstracting everything up front:

~~~text
WorkBuddy
  -> Go Control API
  -> PostgreSQL
  -> Temporal
  -> OPA
  -> S3/MinIO-compatible Artifact Store
  -> Session Gateway
  -> Ubuntu Worker
       -> git worktree
       -> local sandbox
       -> Codex Adapter
       -> Native Build
  -> Git / CI
  -> thin Attestation Controller
  -> Evidence / Verification
  -> Closure
~~~

Cross-cutting:
- OpenTelemetry
- Toxiproxy in integration/failure tests

Do not make SPIRE, OpenBao, Dagger, Coder, labgrid, OpenHTF, GUAC, RAUC or Mender M1 prerequisites.

---

## 3. Keep only nine replaceable external boundaries

Explicit interfaces exist only where replacement/containment is genuinely valuable.

### 3.1 RuntimeProvider

Owns provider-specific:
- launch/resume;
- native session ID;
- event/stdout parsing;
- provider permission-mode translation;
- provider metadata.

M1:
- CodexProvider

Later:
- ClaudeProvider
- other Runtime providers

### 3.2 ExecutionTransport

Owns:
- PTY/process execution;
- local/remote command transport;
- stream/reconnect mechanics.

M1:
- LocalUbuntuTransport

Later:
- remote/SWE-ReX-inspired transport

### 3.3 WorkspaceProvider

Owns:
- workspace create/open/destroy;
- source checkout/worktree;
- environment attachment.

M1:
- NativeGitWorktreeWorkspace

Later:
- CoderWorkspace
- DevContainerWorkspace

### 3.4 AuthorizationEvaluator

Owns deterministic authorization:

~~~text
principal + action + resource + context -> permit/deny
~~~

M1:
- OPA

Cedar remains a future alternative/reference.

### 3.5 CredentialProvider

Owns:
- obtain scoped/short-lived credential;
- renew where allowed;
- revoke;
- secret metadata/audit reference.

M1:
- enterprise secret backend or minimal internal adapter

Later:
- OpenBao if needed

### 3.6 ArtifactStore

Owns immutable bytes:
- upload/finalize;
- fetch;
- digest verification;
- retention/object-lock integration.

M1:
- S3/MinIO-compatible implementation

### 3.7 AttestationSigner

Owns cryptographic signing/verification only.

M1:
- test/dev signer or unsigned standard projection where policy permits

Later:
- Sigstore/cosign/KMS/PKI

### 3.8 DeviceLabProvider

Owns low-level lab resource/control.

M2:
- labgrid candidate

### 3.9 ReleaseProvider

Owns deployment/update mechanism.

M2/M3:
- project-specific first
- optional RAUC/MCUboot/Mender/hawkBit by target class

---

## 4. Internal domain services are not plugin frameworks

Keep these as ordinary internal packages/services:

- AssurancePolicyService
- VerificationService
- EvidenceApplicabilityService
- ReconciliationService
- RiskService
- ClosureService
- AttestationController
- ProcedureExecutionService
- ImpactAnalysisService

They may use external libraries, but do not define plugin marketplaces/interfaces unless a second real implementation appears.

This avoids speculative abstraction.

---

## 5. OPA is the M1 authorization default

Choose OPA for M1.

Reasons:
- mature Go integration;
- context-aware policy;
- policy bundles;
- broad operational adoption;
- suitable for actor/action/resource/environment checks;
- keeps authorization outside Runtime.

Keep two semantic outputs:

~~~text
AuthorizationDecision
AssuranceDecision
~~~

Both may initially be evaluated by OPA policy packages, but remain distinct domain records/contracts.

Cedar remains:
- a policy semantics reference;
- a future option where schema/analyzability benefits justify migration.

OpenFGA remains deferred.

---

## 6. Workload identity is a contract, not an M1 platform dependency

M0 freezes workload identity fields:

~~~text
workload_id
trust_domain
credential_id
attestation_method
issued_at
expires_at
~~~

M1 may use:
- enterprise workload identity;
- platform-issued short-lived mTLS credentials;
- explicit Worker enrollment/revocation.

Adopt SPIFFE/SPIRE when scale/operational triggers justify it:
- many Workers/Agents;
- multiple sites/networks;
- certificate rotation burden;
- cross-service workload identity proliferation.

This preserves migration without forcing SPIRE operations into the first vertical slice.

---

## 7. Credential Broker stays thin

Credential Broker owns:
- authorization check;
- Run/action scope;
- TTL;
- audit link;
- revocation trigger.

CredentialProvider owns secret issuance/storage mechanics.

M1 requirement:
- no reusable long-lived credential in Runtime workspace;
- privileged action proxy preferred;
- short-lived credential when injection is unavoidable.

OpenBao is preferred only when a self-hosted secret backend is actually required.

---

## 8. Session architecture

M1 runtime path:

~~~text
Control API
  -> Session Gateway
      -> Session Supervisor
          -> RuntimeProvider(Codex)
          -> ExecutionTransport(LocalUbuntu)
~~~

Session Supervisor owns:
- Run/Attempt;
- execution epoch;
- Steering;
- pause/resume;
- Human Takeover;
- Checkpoint;
- durable structured events;
- side-effect cursor.

RuntimeProvider and ExecutionTransport never own formal Run authority.

OpenHands/SWE-ReX/Cline remain design references and comparison fixtures.

---

## 9. Runtime capability profiles

Platform-owned profiles:

### PLAN
- source read;
- search/analysis;
- restricted shell;
- no workspace mutation.

### BUILD
- workspace write;
- local build/test;
- no privileged external action except through Action Gateway.

### DEBUG
- BUILD plus explicitly granted debug/device capabilities.

### REVIEW
- reviewed subject read-only;
- query/test allowed;
- no subject mutation.

Provider-native modes do not grant authority; they only map to platform policy.

---

## 10. RepositoryMap is optional context, not authority

Add optional RepositoryMap Artifact:

- source tree digest;
- generator/version;
- symbol/file summary;
- generated timestamp;
- content digest.

It is:
- regenerable;
- disposable;
- non-authoritative.

A stale map cannot be used for current Formal Run context.

---

## 11. Build strategy

M1 uses NativeLegacyBuildBackend behavior internally, not as a plugin framework.

Build Receipt is mandatory from day one:

- source/input digests;
- build definition/config;
- flags;
- relevant environment;
- toolchain/image/profile;
- resolved dependency info where available;
- executor identity;
- logs;
- timing;
- output Artifact digests;
- reproducibility class.

Reproducibility:
- REPRODUCIBLE
- CONTROLLED
- RECORDED_LEGACY
- UNKNOWN

Add Dagger/BuildKit only when a concrete project benefits.

Reserve remote-execution-compatible concepts but do not deploy a remote execution system in M1.

---

## 12. Thin Attestation Controller for M1

M1 implements a small independent controller/service:

~~~text
trusted execution completes
  -> controller reloads authoritative subject/result
  -> verifies exact input/output digests
  -> canonicalizes statement
  -> projects in-toto/SLSA-compatible form
  -> optional dev/test signing
  -> publishes Evidence/Attestation
~~~

The executor cannot self-promote its own output into trusted Evidence.

Production signer integration comes later.

---

## 13. Evidence standards strategy

Platform-native domain remains:
- Artifact;
- Evidence;
- Subject Manifest;
- Verification;
- Transform Receipt.

Standard projection aligns with:
- in-toto statement concepts;
- SLSA provenance;
- Sigstore-compatible verification/signing.

Do not encode Requirement/Run/Target lifecycle inside an external attestation standard.

The standard projection is an interoperability/export layer.

---

## 14. Device/HIL enters at M2

M2 introduces exactly two boundaries:

### DeviceLabProvider
Hardware/resource control.

Preferred spike:
- labgrid.

### ProcedureExecutionService
Platform-owned orchestration of versioned test procedures.

Implementation may use:
- pytest;
- pytest-embedded;
- OpenHTF-inspired phases/measurements/plugs.

Do not expose TestProcedureBackend as a generic plugin API until a real second executor requires it.

The platform keeps:
- Procedure Revision;
- measurement schema;
- Evidence/Verification authority.

---

## 15. Supply-chain intelligence is derived

M2/M3 optional sequence:

1. Syft -> SBOM Artifact
2. OSV -> Vulnerability Evidence
3. Optional GUAC -> cross-artifact/release derived graph

Never put GUAC in the write/authority path.

Security findings create:
- Risk;
- Incident;
- Quarantine;
- Reverification requirement.

They do not rewrite Release history.

---

## 16. Failure injection is mandatory, not optional

Use Toxiproxy in M0/M1 tests.

Inject failures into:
- Session Gateway/Worker stream;
- object store;
- Temporal/API connections;
- Git/CI mocks;
- Release mocks.

Key invariants:
- UNKNOWN outcome reconciles before retry;
- stale epoch cannot mutate;
- duplicate command cannot duplicate side effect;
- reconnect does not create second active owner.

Chaos Mesh is deferred to a future Kubernetes deployment.

---

## 17. Release strategy

M1:
- no generic OTA framework required;
- Release records/manifests may exist without a production deployment backend.

M2/M3:
- choose one ReleaseProvider for the actual pilot/product.

Reference:
- Linux: RAUC first;
- MCU: current OTA / MCUboot practices;
- fleet: Mender/hawkBit later;
- high assurance: Uptane practices.

Do not implement multiple ReleaseProviders preemptively.

---

## 18. Dependency admission

A dependency becomes mandatory only if:

- current milestone needs it;
- active maintenance;
- acceptable license;
- acceptable security process;
- clear adapter boundary;
- degraded/failure mode known;
- upgrade test exists;
- migration/exit path exists;
- no hidden engineering authority.

---

## 19. M0/M1 dependency matrix

| Component | M0/M1 position |
|---|---|
| PostgreSQL | Required |
| Temporal | Required |
| OPA | Required |
| S3/MinIO-compatible store | Required |
| OpenTelemetry | Required |
| Toxiproxy | Required for tests |
| Codex | Required RuntimeProvider |
| Git/CI integration | Required |
| SPIRE | Deferred; identity contract only |
| OpenBao | Deferred unless no enterprise secret backend |
| Sigstore/cosign | Optional PoC, production later |
| OpenHands/SWE-ReX/Cline | Design references |
| Coder | Future WorkspaceProvider |
| Dagger/BuildKit | Project-triggered |
| labgrid | M2 spike |
| OpenHTF/pytest-embedded | M2 procedure reference |
| Syft/OSV | M2/M3 |
| GUAC | Optional derived graph |
| RAUC/MCUboot | M2/M3 target-specific |
| Mender/hawkBit | Future fleet |
| LAVA | Future large device farm |

---

## 20. Final implementation rule

Profile v3 freezes:

> **M1 must prove the trusted engineering loop with the fewest operational dependencies possible.**

And:

> **A mature open-source project is a reason to avoid reinvention, not a reason to add an integration before the milestone needs it.**

The architecture remains v1.2.
Profile v3 is the current implementation companion.
