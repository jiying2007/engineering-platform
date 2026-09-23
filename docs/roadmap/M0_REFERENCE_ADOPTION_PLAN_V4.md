# M0 Reference Adoption Plan v4

Date: 2026-09-23
Status: **Current execution plan**
Supersedes: docs/roadmap/M0_REFERENCE_ADOPTION_PLAN_V3.md

Parents:
- docs/architecture/AI_NATIVE_ENGINEERING_PLATFORM_V1_2_FINAL.md
- docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V4.md

## 1. Principle

M0 remains a contract-and-proof phase, not an ecosystem integration phase.

Round 3 adds only five required contract refinements:
1. Session Grant;
2. Artifact distribution metadata;
3. Release Admission;
4. Integration Event projection;
5. workload-PKI fallback.

No new heavyweight M1 service is required.

---

## 2. Phase 1 — Domain/data invariants

Freeze:

1. Common schema envelope.
2. Canonical serialization/digest profile.
3. Requirement/Task/Run/Artifact/Evidence/Verification schemas.
4. Subject/RunInput/RunReceipt/Closure manifests.
5. Command/Event/Steering envelopes.
6. Session Grant schema.
7. Artifact distribution locator/media-type fields.
8. State/guard tables.
9. UNKNOWN/reconciliation semantics.
10. Release Admission Decision schema.
11. DistributionTrustProfile enum/contract.
12. Integration Event projection schema.

Exit:
- canonical digest fixtures are stable;
- typed refs distinguish ID/digest/locator;
- no schema treats mutable tag/path as identity.

---

## 3. Phase 2 — Core infrastructure

### PostgreSQL
- authoritative projections;
- optimistic concurrency;
- outbox;
- audit event append.

### Temporal
Prove:
- human waits;
- Worker waits/reconnect;
- cancellation;
- retries;
- heartbeat;
- Continue-As-New;
- workflow versioning;
- replay.

### OPA
Implement and test:
- steer/takeover;
- worker action;
- environment scope;
- production authority;
- parent DENY;
- delegation/expiry.

Use:
- opa test;
- optional Conftest fixtures for structured manifests/config.

### ArtifactStore
M1:
- S3/MinIO-compatible.
- upload/finalize;
- digest verify on write/read;
- locator metadata.

---

## 4. Phase 3 — Runtime/session

Implement:

~~~text
eng CLI
 -> Control API
 -> Session Grant
 -> Session Gateway
 -> Session Supervisor
 -> LocalUbuntuTransport
 -> CodexProvider
~~~

Prove:
- OBSERVE / INTERACT grant separation;
- Run/epoch/audience binding;
- expiration;
- reconnect;
- Steering ordering;
- Pause/Resume;
- Human Takeover;
- old grant invalid after epoch/takeover;
- raw transcript separated from authoritative events.

Use Teleport/Boundary as design references only.

---

## 5. Phase 4 — Workload identity / credentials

### WorkloadIdentity contract

Freeze:
- workload_id;
- trust_domain;
- credential_id;
- attestation_method;
- issued_at;
- expires_at.

Implementation order:
1. enterprise PKI/identity if available;
2. step-ca spike if a lightweight internal CA is needed;
3. SPIRE later at scale.

### CredentialProvider

Prove:
- no reusable credential in workspace;
- short-lived/action-bound credential;
- revoke/expiry;
- no secrets in manifests/events/artifacts;
- outage fails privileged action closed.

OpenBao remains optional unless a new secret store is required.

---

## 6. Phase 5 — Artifact/Evidence/Attestation

Create one real chain:

~~~text
source commit
 -> native build
 -> Artifact
 -> CI/test
 -> Attestation Controller
 -> in-toto/SLSA-compatible projection
 -> Evidence
 -> Verification
~~~

Prove:
- executor cannot self-authorize trusted Evidence;
- exact digest binding;
- subject change makes old Evidence inapplicable;
- historical statement remains verifiable.

AttestationSigner:
- dev/test implementation sufficient for M1;
- production signer later.

---

## 7. Phase 6 — Release Admission contract

M1 may not perform real production promotion, but the domain contract must be frozen.

Admission algorithm:

1. load frozen Release Manifest;
2. resolve any mutable locators;
3. verify exact final digests;
4. verify required signature/attestation references;
5. re-evaluate current Evidence applicability;
6. re-evaluate waiver/approval validity;
7. check quarantine/current policy;
8. issue Admission Decision.

Invariant:

~~~text
Human Approval
!=
Current Release Admission
~~~

A Release cannot be promoted after trust conditions change merely because an older approval exists.

---

## 8. Phase 7 — Promotion reconciliation model

Freeze desired-vs-actual schema even if M1 has only a mock ReleaseProvider.

Desired:
- Release Manifest digest;
- environment;
- target/channel.

Actual:
- provider-observed artifact/bundle;
- external status;
- receipt.

States:
- PENDING;
- CONVERGING;
- CONFIRMED;
- DRIFTED;
- UNKNOWN;
- FAILED.

Use Argo CD/Flux only as conceptual reconciliation references.

---

## 9. Phase 8 — Integration events

Keep internal Domain Events authoritative.

Implement a small projector for selected external notifications using a CloudEvents-compatible envelope.

Candidate events:
- WorkReady;
- RunWaitingHuman;
- VerificationCompleted;
- ReleaseConfirmed.

Prove:
- integration event loss/retry does not change domain truth;
- no secret/internal privileged payload leakage;
- event includes stable IDs/correlation.

---

## 10. Phase 9 — Failure injection

Use Toxiproxy or equivalent deterministic network fault harness.

Inject:
- Session Gateway disconnect;
- Worker reconnect;
- object store timeout;
- Git/CI response loss;
- Temporal/API interruption;
- mock Release timeout after success.

Prove:
- stale owner rejected;
- UNKNOWN reconciled;
- duplicate dispatch prevented;
- session reconnect preserves Run authority.

---

## 11. Deferred M2 — Artifact distribution

When needed:

### OCI/ORAS spike
Validate:
- firmware/release package;
- SBOM;
- attestation;
- referrer/subject relationships;
- digest-preserving pull/push.

### Harbor
Evaluate only when requiring:
- RBAC/project registry;
- replication;
- audit;
- scanning;
- retention/GC.

Do not replace Artifact business identity with registry coordinates.

---

## 12. Deferred M2/M3 — Signature ecosystem

AttestationSigner candidates:
- cosign/Sigstore;
- Notation/Notary;
- KMS/HSM/enterprise PKI.

Choose based on actual artifact ecosystem.

No requirement to support multiple signing stacks simultaneously.

---

## 13. Deferred high-assurance distribution

Use TUF/Uptane design when risk requires:
- repository compromise resilience;
- role/key separation;
- metadata expiry;
- rollback/freeze protection;
- threshold signing.

Not required for normal M1 engineering artifacts.

---

## 14. Deferred Device/HIL

M2:
- labgrid resource/control spike;
- pytest/pytest-embedded/OpenHTF procedure execution comparison.

Freeze Procedure/Measurement/Attachment only from a real motor/HIL pilot.

---

## 15. Deferred supply-chain intelligence

After Artifact/Evidence/Release path works:
- Syft or Trivy -> SBOM;
- OSV/Grype/Trivy -> vulnerability findings;
- optional GUAC -> derived impact graph.

Never put scanner/graph state in authority path.

---

## 16. M0 exit

M0 exits only when:

- core schemas/digests frozen;
- Session Grant model proven;
- PostgreSQL/Temporal boundary proven;
- OPA policy tests pass;
- ArtifactStore exact-digest path works;
- interactive Codex session works;
- workload identity/credential contract works;
- Attestation Controller chain works;
- Release Admission rules have executable fixtures;
- promotion reconciliation schema is frozen;
- CloudEvents integration projection is demonstrated;
- Toxiproxy failure matrix passes;
- invariant/property tests cover stale epoch, duplicate command, stale Evidence, approval/admission binding.

M0 does not require:
- SPIRE;
- OpenBao;
- Harbor;
- RAUC;
- labgrid;
- GUAC;
- Coder;
- Dagger;
- TUF.

---

## 17. M1 target remains intentionally small

~~~text
WorkBuddy
 -> Requirement READY
 -> Work / Task Revision
 -> Run Input Manifest
 -> Session Grant
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
- S3/MinIO-compatible store;
- native Ubuntu execution;
- OpenTelemetry;
- deterministic fault injection.

This remains the smallest credible trusted engineering loop.
