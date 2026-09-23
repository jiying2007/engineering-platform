# Open Source Reference Synthesis — Optimization Decisions

Date: 2026-09-23
Status: **Archived synthesis / design decision input**
Sources:
- Open Source Reference Review Round 1
- Open Source Reference Review Round 2
- Architecture v1.2
- Implementation Profile v2

## 1. Why another synthesis

The broad open-source search was useful, but blindly converting every good project into a first-class backend interface would create a new failure mode:

> engineering-platform becomes an integration framework before it becomes a working engineering control plane.

The optimized strategy is therefore:

1. keep the v1.2 domain architecture unchanged;
2. reduce M0/M1 dependency breadth;
3. choose defaults where evidence is strong;
4. keep replaceable interfaces only at genuinely unstable/external boundaries;
5. defer optional ecosystems until a real need appears.

---

## 2. Final classification

### Core dependencies for M1

These solve hard problems that should not be rebuilt:

- PostgreSQL — mutable business authority
- Temporal — durable orchestration
- OPA — default authorization engine
- Object storage — Artifact/Evidence bytes
- OpenTelemetry — observability
- Toxiproxy — deterministic failure-injection harness

### Core internal components

These are the unique value of engineering-platform and must remain platform-owned:

- Requirement / Work / Task / Target
- Run / Attempt / Steering / Takeover
- Policy decision records / Human Authority
- Artifact / Subject / Release / Closure manifests
- Evidence applicability
- Verification / Review / Risk / Closure
- cross-system reconciliation
- WorkBuddy integration

### M1 optional / deferred dependencies

Do not block M1 on these:

- SPIFFE/SPIRE
- OpenBao
- Sigstore/cosign production signing
- Dagger/BuildKit
- Coder
- OpenHands/SWE-ReX reuse
- Syft/OSV/GUAC
- labgrid
- OpenHTF
- RAUC/MCUboot/Mender

Their interfaces may be reserved, but implementation is introduced only when its milestone needs it.

---

## 3. Policy decision: choose OPA as M1 default

Cedar remains a strong semantics/analyzability reference.

For M1, OPA is preferred because:
- direct Go integration is mature;
- general context-aware decisions fit actor/action/resource/environment;
- policy bundles are operationally straightforward;
- one engine can initially serve authorization and assurance-policy evaluation through separate policy packages/contracts.

Do not merge the domain contracts:

~~~text
AuthorizationDecision
!= AssuranceDecision
~~~

Even if both are evaluated by OPA.

Cedar remains a future option if stronger schema-driven authorization analysis becomes valuable.

OpenFGA remains deferred until relationship complexity justifies a dedicated ReBAC store.

---

## 4. Workload identity decision: interface now, SPIRE later unless needed

SPIFFE/SPIRE is an excellent production-grade solution, but it can be operationally heavy for an M1 topology with one Control Plane and one/few Ubuntu Workers.

Optimized approach:

### M0/M1

Freeze a provider-neutral WorkloadIdentity contract:

~~~text
workload_id
trust_domain
credential_id
issued_at
expires_at
attestation_method
~~~

Use:
- short-lived platform-issued mTLS credentials or enterprise workload identity already available;
- explicit Worker enrollment/revocation.

### M2/scale trigger for SPIRE

Adopt SPIRE when one or more become true:
- many Workers/Device Agents;
- multiple networks/sites;
- automatic cert rotation becomes operational burden;
- service-to-service identity spans many components;
- secret/cloud integration benefits from SPIFFE identity.

This avoids making SPIRE a prerequisite while preserving a clean migration path.

---

## 5. Secret decision: do not require OpenBao in M1

OpenBao is a strong SecretBackend candidate.

But M1 should first prove:
- Credential Broker API;
- no reusable credential in Runtime workspace;
- action-scoped credential;
- TTL/revocation;
- audit.

If the company already has an enterprise secret manager, use it.

Adopt OpenBao only if a new self-hosted secret backend is actually needed.

The platform owns the broker/policy semantics, not the secret database.

---

## 6. Runtime/session decision: borrow patterns, keep implementation small

Do not embed OpenHands or SWE-ReX as a mandatory runtime dependency in M1.

M1 needs only:

~~~text
Session Gateway
Session Supervisor
Codex Adapter
Native Local ExecutionBackend
~~~

Study and copy proven boundaries from:
- OpenHands Agent Server;
- SWE-ReX;
- Cline.

Only add a remote ExecutionBackend when a real remote execution need exists.

This keeps the interactive Codex path close to the user's existing Ubuntu workflow.

---

## 7. Attestation decision: standards first, minimum implementation

Do not deploy a large supply-chain platform in M1.

M1 should implement a thin Attestation Controller that:
1. receives/observes a completed trusted execution;
2. reloads authoritative subject and output digests;
3. canonicalizes a statement;
4. stores an in-toto/SLSA-compatible unsigned or test-signed attestation;
5. binds it to Evidence.

Production-grade signing via Sigstore/KMS follows when release/signing authority is introduced.

This gets the data model right without pulling cryptographic operations into the critical path too early.

---

## 8. Device/HIL decision: milestone-gated

M1 has no labgrid/OpenHTF dependency.

M2 introduces two explicit boundaries:

~~~text
DeviceLabBackend
TestProcedureBackend
~~~

Then run a real comparison:

- labgrid for hardware resource/control;
- pytest/pytest-embedded/OpenHTF patterns for procedure execution.

Prefer composition:

~~~text
labgrid -> hardware access
pytest/OpenHTF-style runner -> test procedure
engineering-platform -> Evidence/Verification authority
~~~

Do not build a generic hardware framework before the motor/HIL pilot proves what is required.

---

## 9. Build decision: NativeLegacy first

M1 default:

~~~text
NativeLegacyBuildBackend
~~~

because embedded projects often contain non-hermetic vendor SDKs.

Still freeze Build Receipt and reproducibility classes from day one.

Only add Dagger/BuildKit when:
- a project is container-friendly;
- reproducibility/local-CI parity is valuable;
- spike shows lower operational complexity.

Remote Execution API stays a future compatibility model, not an M1 feature.

---

## 10. Supply-chain intelligence decision: after Artifact/Evidence closure

SBOM and vulnerability analysis are valuable but not part of the minimal formal engineering loop.

Sequence:

1. M1: Artifact/Evidence/Verification works.
2. M2/M3: add Syft SBOM Artifact.
3. add OSV Vulnerability Evidence.
4. introduce GUAC only if cross-release impact querying is painful enough to justify it.

Never introduce a graph database before the core relational lineage queries prove inadequate.

---

## 11. Release/update decision

M1 does not implement generic OTA backends.

M2/M3 reserves:

~~~text
ReleaseBackend
~~~

but only implements the backend needed by the pilot/product.

Reference priority:
- Linux embedded: RAUC first
- MCU secure update: MCUboot practices
- fleet rollout: Mender/hawkBit later
- high-assurance update: Uptane practices

Avoid "support every updater" as a platform goal.

---

## 12. Interface reduction

Profile v2 listed many Backend interfaces.

Profile v3 should keep only boundaries that protect domain ownership or external replaceability.

### Keep as explicit interfaces

- RuntimeProvider
- ExecutionTransport
- WorkspaceProvider
- AuthorizationEvaluator
- CredentialProvider
- ArtifactStore
- AttestationSigner
- DeviceLabProvider
- ReleaseProvider

### Keep as internal domain services, not plugin interfaces

- AssurancePolicyEngine
- VerificationEngine
- EvidenceApplicability
- ReconciliationEngine
- RiskEngine
- ClosureEngine
- AttestationController
- TestProcedure orchestration

Reason:

> not every internal module needs to become a pluggable framework.

This materially reduces interface/versioning burden.

---

## 13. Optimized milestone dependency map

### M0/M1 hard dependencies
- PostgreSQL
- Temporal
- OPA
- S3/MinIO-compatible object store
- OpenTelemetry
- Toxiproxy for tests
- Git/CI provider
- Codex CLI/runtime

### M2 likely
- labgrid
- pytest/OpenHTF-style procedure runner

### M2/M3 likely
- Sigstore/cosign/KMS
- Syft/OSV
- RAUC or product-specific ReleaseProvider

### Scale-triggered
- SPIRE
- OpenBao
- Coder
- GUAC
- LAVA
- Remote Execution API backend
- Mender/hawkBit

---

## 14. Architecture principle after optimization

The open-source search leads to a simpler rule:

> **Standardize data and authority early; integrate infrastructure only when the milestone actually needs it.**

This prevents both extremes:

- building everything ourselves;
- turning the platform into a dependency-heavy integration mesh before the formal engineering loop works.

---

## 15. Final decision

Architecture v1.2 remains unchanged.

Implementation Profile should be updated to v3 with:
- OPA as M1 default;
- SPIRE/OpenBao deferred behind stable contracts;
- thin M1 Attestation Controller;
- Native Ubuntu/NativeLegacy first;
- labgrid/test framework introduced at M2;
- SBOM/security graph and generic OTA kept milestone-gated;
- fewer generic plugin interfaces.

The project is now ready to stop broad architectural expansion and convert these decisions into ADR/schema/code spikes.
