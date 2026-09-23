# Reference-Aligned Implementation Profile v4

Date: 2026-09-23
Status: **Canonical implementation companion to architecture v1.2**
Supersedes: docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V3.md

Research basis:
- docs/research/OPEN_SOURCE_REFERENCE_REVIEW_2026-09-23.md
- docs/research/OPEN_SOURCE_REFERENCE_REVIEW_ROUND2_2026-09-23.md
- docs/research/OPEN_SOURCE_REFERENCE_REVIEW_ROUND3_2026-09-23.md
- docs/research/OPEN_SOURCE_REFERENCE_SYNTHESIS_OPTIMIZATION_2026-09-23.md

## 1. Goal

Profile v4 preserves the minimal dependency strategy of v3.

It adds interoperability and last-mile trust rules discovered in Round 3 without creating new mandatory infrastructure.

Core rule remains:

> **Standardize authority and data early; integrate external infrastructure only when the milestone needs it.**

---

## 2. M1 concrete stack remains unchanged

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
       -> Codex RuntimeProvider
       -> Native Build
  -> Git / CI
  -> thin Attestation Controller
  -> Evidence / Verification
  -> Closure
~~~

Cross-cutting:
- OpenTelemetry
- Toxiproxy in failure tests

No new mandatory service is introduced by v4.

---

## 3. Session Grant model

Inspired by Teleport/Boundary JIT access patterns.

Every interactive attach uses a short-lived Session Grant.

Minimum fields:

~~~text
session_grant_id
actor_id
run_id
execution_epoch
mode
audience
issued_at
expires_at
nonce
policy_decision_id
~~~

Modes:

~~~text
OBSERVE
INTERACT
TAKEOVER_SUPPORT
~~~

Rules:
- Run A grant cannot attach to Run B.
- Old epoch invalidates write-capable grants.
- takeover/abort invalidates prior INTERACT grants.
- OBSERVE does not imply Steering.
- raw terminal access never grants business authority.

---

## 4. Session audit has two channels

### Authoritative structured channel

Contains:
- Steering;
- Pause/Resume;
- Takeover;
- permission/approval;
- checkpoint;
- state transitions.

This is retained as formal domain/audit data.

### Raw session channel

Contains:
- PTY input/output;
- runtime logs;
- diagnostic stream.

This is:
- operational/debug data;
- separately retained;
- redacted where required;
- not parsed later as the source of business truth.

This mirrors mature privileged-session systems while preserving formal engineering semantics.

---

## 5. Workload identity fallback

The provider-neutral WorkloadIdentity contract from v3 remains.

Preferred implementation order:

~~~text
1. existing enterprise PKI/workload identity
2. step-ca for lightweight short-lived mTLS if needed
3. SPIFFE/SPIRE when scale/multi-site identity warrants it
~~~

This keeps M1 operationally light while avoiding custom long-lived static certificates.

---

## 6. Artifact Profile v4

Artifact identity remains provider-neutral and digest-based.

Add distribution metadata:

~~~text
artifact_id
content_digest
media_type
artifact_type
size
subject_refs
distribution_locators[]
~~~

A locator may be:
- S3/MinIO object;
- OCI digest reference;
- external release-system object;
- archival package path.

Rule:

> locator is never identity.

Every authoritative consumption re-verifies digest.

---

## 7. OCI distribution profile

M1 continues using S3/MinIO for simplicity.

For suitable release-grade objects, later support OCI Distribution:
- firmware bundle;
- SBOM;
- provenance;
- signed attestation;
- test package;
- release package.

Use:
- OCI Distribution semantics;
- ORAS tooling where useful;
- Harbor as a future enterprise registry option.

Do not force:
- raw traces;
- PTY logs;
- every Evidence attachment

into OCI.

Harbor is considered when we need:
- registry RBAC;
- replication;
- audit;
- vulnerability scanning;
- retention/GC;
- multi-site registry operations.

---

## 8. Signature backend remains provider-neutral

AttestationSigner can use:
- Sigstore/cosign;
- Notation/Notary for OCI-native signatures;
- KMS/HSM;
- enterprise PKI.

Domain records never depend on one signature envelope.

Signing/verification always bind exact content digest.

---

## 9. Release Approval and Release Admission are separate

### Human Approval

Answers:

> has an authorized human accepted this exact Release Manifest and risk context?

### Release Admission

Answers immediately before irreversible promotion:

> is this exact Release Manifest still currently eligible to promote?

Admission reloads:
- exact Release Manifest;
- final Artifact bytes/digests;
- current issuer/trust status;
- required attestations;
- current Evidence applicability;
- waiver/approval expiry;
- quarantine/security findings;
- target/environment policy.

It emits an immutable Admission Decision bound to the Release Manifest digest.

This is an internal domain service, not a new plugin framework.

---

## 10. Release Admission guard

Before production dispatch:

~~~text
Release Manifest frozen
  -> resolve mutable locators
  -> verify exact content digests
  -> verify required signatures/attestations
  -> evaluate current Verification applicability
  -> evaluate current risk/quarantine/policy
  -> validate approval still applicable
  -> issue Admission Decision
  -> dispatch promotion
~~~

This catches changes that happen after the original human approval.

---

## 11. Promotion is desired-state reconciliation

Borrow the core GitOps idea without requiring Argo CD/Flux.

Desired:

~~~text
release_manifest_digest
target
environment
channel
component bundle
~~~

Actual:

~~~text
observed external deployment state
actual artifact/bundle identity
provider receipt/status
~~~

Projection:

~~~text
PENDING
CONVERGING
CONFIRMED
DRIFTED
UNKNOWN
FAILED
~~~

Dispatch success is not release confirmation.

---

## 12. Integration Event Projection

Internal Domain Event remains authoritative.

Add optional:

~~~text
IntegrationEventProjector
~~~

which maps selected domain events to CloudEvents-compatible envelopes.

Examples:
- WorkReady
- RunWaitingHuman
- VerificationCompleted
- ReleaseConfirmed

External payloads should contain:
- stable IDs;
- event type;
- source;
- time;
- correlation reference;
- minimal safe business data.

They should not expose secrets or raw internal authority state.

CloudEvents is transport interoperability, not audit authority.

---

## 13. Long-term result archival

Borrow Tekton Results' separation between live execution resources and durable history.

After a Run is durably closed:
- Run Receipt is finalized;
- required structured events are persisted;
- logs/artifacts are archived by retention class;
- ephemeral process/session resources can be garbage-collected.

Do not retain active session machinery only for audit purposes.

---

## 14. Policy testing

OPA is still the M1 default.

Policy CI must include:
- opa test;
- representative authorization fixtures;
- assurance decision fixtures;
- parent-deny/child-widen attempts;
- production environment cases;
- expiry/delegation cases.

Conftest may additionally validate representative structured config/manifests.

Policy changes are treated as platform-control changes and require stronger review.

---

## 15. Distribution Trust Profiles

Do not mandate TUF.

Reserve:

~~~text
DistributionTrustProfile
~~~

### BASIC
- exact digest;
- authenticated transport;
- signature if required.

### SIGNED_METADATA
- signed versioned metadata;
- expiry/freshness rules.

### HIGH_ASSURANCE
- TUF/Uptane-inspired role/key separation;
- threshold signatures where appropriate;
- rollback/freeze attack resistance.

Use high-assurance profile only when product/security risk justifies it.

Human Release Approval remains separate.

---

## 16. Scanner normalization

SupplyChainIntel does not hard-code one scanner.

Normalize outputs to:
- SBOM Artifact;
- Vulnerability Finding/Evidence;
- Secret Finding;
- Misconfiguration Finding;
- License Finding.

Candidate toolsets:
- Syft + OSV;
- Trivy;
- Grype where appropriate.

GUAC remains an optional derived impact graph.

---

## 17. Trusted control-component capability review

For Go platform-control components, Capslock-style capability analysis may be used as an additional review signal.

Applicable components:
- Control API;
- Worker;
- Session Gateway;
- Platform Action Gateway.

Unexpected new network/filesystem/process capability can trigger enhanced review.

This is a review signal, not a standalone Verification authority.

---

## 18. Updated external boundary set

Keep explicit replaceable boundaries:

- RuntimeProvider
- ExecutionTransport
- WorkspaceProvider
- AuthorizationEvaluator
- CredentialProvider
- ArtifactStore
- AttestationSigner
- DeviceLabProvider
- ReleaseProvider

Add no new plugin interface for:
- Release Admission;
- Integration Event Projection;
- Attestation Controller;
- SupplyChainIntel normalization;
- Session audit.

These remain internal services/modules until multiple real implementations demand abstraction.

---

## 19. Milestone dependency profile

### M0/M1 required
- PostgreSQL
- Temporal
- OPA
- S3/MinIO-compatible ArtifactStore
- OpenTelemetry
- Toxiproxy
- Git/CI
- Codex
- enterprise PKI or lightweight short-lived mTLS mechanism

### M1 optional
- step-ca if PKI is otherwise missing
- dev/test attestation signer

### M2 likely
- labgrid
- pytest/OpenHTF-style procedure execution
- OCI/ORAS for selected release artifacts
- production signing backend

### M2/M3 security/release
- Syft/OSV or Trivy
- RAUC/project-specific ReleaseProvider
- Notation or cosign according to artifact ecosystem

### Scale/high-assurance
- SPIRE
- OpenBao
- Harbor
- GUAC
- LAVA
- TUF/Uptane profile
- Coder
- remote execution backend

---

## 20. Final rule

Profile v4 adds standards and guards, not infrastructure sprawl.

> **M1 remains small. External interoperability is achieved through stable IDs, digests, event projections and admission/reconciliation semantics before introducing large external platforms.**

Architecture v1.2 remains canonical.
Profile v4 is the current implementation companion.
