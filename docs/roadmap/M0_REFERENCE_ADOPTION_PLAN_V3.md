# M0 Reference Adoption Plan v3

Date: 2026-09-23
Status: **Current execution plan**
Supersedes:
- docs/roadmap/M0_REFERENCE_ADOPTION_PLAN.md
- docs/roadmap/M0_REFERENCE_ADOPTION_PLAN_V2.md

Parent:
- docs/architecture/AI_NATIVE_ENGINEERING_PLATFORM_V1_2_FINAL.md
- docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V3.md

## 1. Principle

M0 is no longer a broad integration program.

M0 exists to freeze the few contracts M1 needs and to prove that the selected infrastructure choices do not violate platform authority.

Do not block M1 on infrastructure that belongs to M2/M3.

---

## 2. M0 critical path

Execute in this order:

### Phase 1 — Domain/data invariants
1. Common schema envelope.
2. Canonical serialization/digest profile.
3. Requirement/Task/Run/Artifact/Evidence/Verification schemas.
4. Subject/RunInput/RunReceipt/Closure manifests.
5. Command/Event/Steering envelopes.
6. State/guard tables.
7. Reconciliation/UNKNOWN semantics.

### Phase 2 — Core infrastructure contracts
8. PostgreSQL authority/outbox ADR.
9. Temporal orchestration ADR + spike.
10. OPA authorization ADR + policy tests.
11. ArtifactStore contract.
12. RuntimeProvider / ExecutionTransport / WorkspaceProvider contracts.
13. Session Gateway protocol.
14. CredentialProvider contract.
15. Workload identity contract.

### Phase 3 — Trust/evidence
16. thin Attestation Controller.
17. in-toto/SLSA-compatible projection.
18. Evidence Issuer/Trust Root schema.
19. dev/test AttestationSigner.
20. Verification service rules.

### Phase 4 — Reliability
21. Toxiproxy harness.
22. stale epoch/duplicate command tests.
23. external UNKNOWN/reconciliation tests.
24. worker/session reconnect tests.
25. Temporal replay/versioning tests.

Only then start the M1 business vertical slice.

---

## 3. Required Spike — Temporal

Decision: ADOPT.

Prove:
- human wait;
- Worker reconnect;
- cancellation;
- heartbeat;
- retry;
- workflow versioning;
- PostgreSQL outbox boundary;
- duplicate Activity does not duplicate side effects.

Deliver:
- ADR;
- Go sample;
- failure tests.

---

## 4. Required Spike — OPA

Decision: M1 default AuthorizationEvaluator.

Implement representative policies:
- user steer/takeover;
- Worker Git push request;
- Device dev flash request;
- production release authorization;
- environment restriction;
- parent deny / child attempt to widen.

Deliver:
- ADR;
- policy package structure;
- test fixtures;
- policy bundle digest/version rules.

Cedar comparison remains documented but is not an M1 blocker.

---

## 5. Required Spike — Session path

Implement one complete path:

~~~text
eng CLI
 -> Control API
 -> short-lived attach token
 -> Session Gateway
 -> Session Supervisor
 -> LocalUbuntuTransport
 -> CodexProvider
~~~

Prove:
- attach/reconnect;
- Steering ordering;
- execution epoch;
- Pause/Resume;
- Human Takeover;
- Runtime write revoked after takeover;
- process crash -> new Attempt.

Study OpenHands/SWE-ReX/Cline while implementing, but do not introduce a hard dependency without measurable value.

---

## 6. Required Spike — Artifact/Evidence/Attestation

Create one real chain:

~~~text
source commit
 -> native build
 -> Artifact
 -> CI/test result
 -> Attestation Controller
 -> standard-compatible statement
 -> Evidence
 -> Verification
~~~

Prove:
- exact digest binding;
- executor cannot self-issue trusted Evidence;
- subject mismatch fails;
- old Evidence becomes inapplicable after subject changes;
- historical statement can still be verified.

Production-grade Sigstore/KMS signing is not required for M1.

---

## 7. Required Spike — Credential isolation

Implement CredentialProvider contract with whichever enterprise secret backend is already available.

Prove:
- no reusable credential in workspace;
- action-scoped/short-lived credential;
- abort/takeover revokes or expires access;
- broker outage fails privileged action closed;
- secret never enters manifest/event/artifact.

OpenBao is evaluated only if a new secret backend is needed.

---

## 8. Required Spike — Failure injection

Use Toxiproxy or equivalent deterministic harness.

Inject:
- Session Gateway disconnect;
- Worker stream disconnect;
- object-store timeout;
- Git/CI response loss after simulated success;
- Temporal connectivity interruption.

Prove:
- UNKNOWN result is reconciled before retry;
- no duplicate privileged side effects;
- reconnect does not create dual ownership.

---

## 9. Deferred M2 Spike — Device/HIL

Do not block M1.

At M2:

### labgrid spike
- one motor MCU/HIL lane;
- serial;
- power/reset;
- flash;
- measurement/log.

### Procedure execution spike
Compare:
- pytest;
- pytest-embedded;
- OpenHTF concepts.

Freeze only after real HIL experience:
- Procedure Revision;
- Phase/Step;
- Measurement;
- Attachment;
- Evaluation result.

---

## 10. Deferred M2/M3 Spike — Supply-chain intelligence

Sequence:
1. Syft SBOM;
2. OSV vulnerability Evidence;
3. optional GUAC impact graph.

Trigger:
- Artifact/Evidence/Release path already works;
- security impact querying becomes valuable.

---

## 11. Deferred M2/M3 Spike — Release backend

Select only the backend needed by the product.

Potential:
- RAUC for Linux;
- current project updater / MCUboot practices for MCU;
- Mender/hawkBit for fleet.

No multi-backend framework before the first real deployment integration.

---

## 12. Deferred scale Spike — SPIRE

Trigger:
- worker/service count grows;
- multi-site;
- certificate lifecycle becomes painful;
- enterprise workload identity absent.

Until then:
- provider-neutral workload identity schema;
- short-lived mTLS identity;
- explicit enrollment/revocation.

---

## 13. Optional modernization spikes

Only project-triggered:

### Dagger/BuildKit
When a repository is container-friendly and build reproducibility justifies it.

### Dev Containers
When a project can share one modern environment definition across local/CI.

### Coder
When remote/cloud workspace provisioning becomes a real need.

### Remote Execution API
When build scale/caching makes distributed execution worthwhile.

---

## 14. M0 Exit Criteria

M0 is complete only when:

- canonical digest fixtures pass independently;
- domain schemas and refs are frozen;
- Temporal boundary is proven;
- OPA policy boundary is proven;
- Session path is functional end-to-end;
- Artifact/Evidence/Verification chain is functional;
- credential isolation is proven;
- Toxiproxy failure tests pass;
- invariant/property tests exist for stale epoch, duplicate command, stale Evidence and approval binding;
- every M1 hard dependency has failure/upgrade/exit semantics.

M0 is **not** required to finish:
- labgrid;
- SPIRE;
- OpenBao;
- Dagger;
- Syft/OSV/GUAC;
- RAUC/MCUboot/Mender;
- Coder.

---

## 15. M1 target

The first production-worthy vertical slice is:

~~~text
WorkBuddy
 -> Requirement READY
 -> Work / Task Revision
 -> Run Input Manifest
 -> Codex interactive Run
 -> Git / CI
 -> Artifact
 -> Attestation Controller
 -> Evidence
 -> Verification
 -> Closure Manifest
 -> WorkBuddy
~~~

with:
- PostgreSQL;
- Temporal;
- OPA;
- object store;
- native Ubuntu execution;
- OpenTelemetry;
- deterministic failure tests.

This is the smallest credible formal engineering loop.
