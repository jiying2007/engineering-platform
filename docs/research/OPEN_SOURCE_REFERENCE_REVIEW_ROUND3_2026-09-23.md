# Open Source Reference Review — Round 3

Date: 2026-09-23
Status: **Archived research / implementation input**
Parent:
- docs/architecture/AI_NATIVE_ENGINEERING_PLATFORM_V1_2_FINAL.md
- docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V3.md

## 1. Scope

Round 3 focused on standardizing boundaries that were still largely platform-specific:

- privileged interactive session access and audit;
- artifact registry/distribution;
- OCI signing and release admission;
- update metadata trust;
- external event interoperability;
- long-term execution/result storage;
- lightweight PKI;
- GitOps-style reconciliation;
- supply-chain metadata models.

The goal is not to add more mandatory systems. The goal is to identify standards and patterns that make existing boundaries safer and easier to replace.

---

## 2. Session access and audit — Teleport / Boundary

Repositories:
- https://github.com/gravitational/teleport
- https://github.com/hashicorp/boundary

### Relevant practices

Teleport provides:
- identity-aware access;
- short-lived certificate-based access;
- JIT elevation;
- secure tunnels through NAT/firewalls;
- session sharing;
- session recording/audit;
- RBAC/ABAC.

Boundary separates:
- Controller: API/session coordination;
- Worker: session handling;
- user CLI/Desktop client;
- JIT access;
- per-session credentials;
- dynamic credential integration.

### Absorb

These validate and refine our existing Session architecture:

~~~text
eng CLI
 -> Control API: authorize/mint short-lived session grant
 -> Session Gateway
 -> outbound Worker channel
 -> Session Supervisor
~~~

Session grant should include:
- actor;
- Run;
- execution epoch;
- mode: OBSERVE / INTERACT / TAKEOVER_SUPPORT;
- audience;
- expiry;
- nonce/session ID.

Session audit should have two layers:
- structured authoritative engineering commands/events;
- optional raw/terminal recording with independent retention/redaction.

### Do not adopt

Do not make Teleport/Boundary a mandatory M1 dependency.

Reasons:
- our formal Run semantics are richer than infrastructure access;
- normal Runtime traffic is not generic SSH access;
- licensing/operations/deployment requirements may not fit.

They remain strong Session Gateway/JIT/audit design references.

---

## 3. Artifact registry and OCI distribution — OCI / ORAS / Harbor

Repositories:
- https://github.com/opencontainers/image-spec
- https://github.com/opencontainers/distribution-spec
- https://github.com/oras-project/oras
- https://github.com/goharbor/harbor

### Relevant practices

OCI Distribution is generic enough to distribute content beyond container images.

ORAS provides practical tooling for OCI artifact distribution.

Harbor adds:
- RBAC/projects;
- replication;
- vulnerability scanning;
- audit;
- retention/garbage collection;
- OCI distribution conformance.

### Optimization

Keep platform Artifact identity provider-neutral, but enrich Artifact metadata with:

~~~text
media_type
artifact_type
content_digest
size
subject/referrer relation
distribution_locators[]
~~~

Allow a release-grade Artifact to have one or more locators:
- S3/MinIO object;
- OCI registry digest reference;
- external release backend locator.

Identity remains digest.

### Milestone strategy

M1:
- S3/MinIO-compatible ArtifactStore remains simplest.

M2/M3:
- add OCI Distribution/ORAS support when we need:
  - portable artifact exchange;
  - attestations/SBOM as referrers;
  - registry-native retention/distribution.

Scale/enterprise:
- Harbor becomes a candidate Artifact Registry implementation when RBAC, replication, scanning and audit justify it.

Do not force all raw logs/traces into OCI.

---

## 4. OCI signatures — Notation / Notary

Repository:
- https://github.com/notaryproject/notation

Notation represents signatures as standard items in the OCI registry ecosystem and supports OCI artifact signing/verification.

### Decision

Keep AttestationSigner provider-neutral.

Candidates:
- Sigstore/cosign;
- Notation/Notary;
- enterprise KMS/PKI.

Guideline:
- cosign/in-toto is a strong default for general attestation;
- Notation is an important OCI-specific alternative;
- do not make engineering domain records depend on one signature envelope.

---

## 5. Release Admission — Sigstore Policy Controller pattern

Repository:
- https://github.com/sigstore/policy-controller

### Important practice

Policy Controller verifies signatures/attestations at admission time and resolves mutable image tags so the admitted image cannot silently change afterward.

### Engineering-platform optimization

Introduce an internal domain step:

~~~text
Release Admission
~~~

Before irreversible promotion:

1. load exact Release Manifest;
2. resolve every mutable locator/reference;
3. verify final content digests;
4. verify required signatures/attestations;
5. check Evidence/Verification applicability;
6. evaluate current risk/quarantine/policy;
7. issue Admission Decision bound to exact Release Manifest digest;
8. dispatch promotion.

This is not a new pluggable service requirement.
It is part of Release/Assurance domain logic.

Key rule:

> approval may be historical, but admission evaluates current trust immediately before promotion.

This catches:
- issuer revocation after approval;
- vulnerability/quarantine;
- mutable tag drift;
- missing final artifact;
- expired waiver.

---

## 6. Secure update metadata — TUF

Repositories:
- https://github.com/theupdateframework/python-tuf
- https://github.com/theupdateframework/specification

TUF protects update/content delivery even under repository or signing-key compromise, using structured metadata, roles, key separation and expiry.

Uptane extends similar principles for automotive OTA.

### Optimization

Define DistributionTrustProfile:

~~~text
BASIC
  exact digest + HTTPS + signature

SIGNED_METADATA
  role/version/expiry-aware signed metadata

HIGH_ASSURANCE
  TUF/Uptane-inspired separation/threshold/rollback protections
~~~

Do not implement TUF in M1.

Use TUF/Uptane as the reference when:
- release artifacts are distributed through untrusted mirrors/CDNs;
- key compromise resilience matters;
- update metadata rollback/freeze attacks matter.

This is separate from engineering Human Approval.

---

## 7. External event interoperability — CloudEvents

Repository:
- https://github.com/cloudevents/spec

CloudEvents provides a standard envelope for interoperable events across systems.

### Decision

Do not replace the internal authoritative Domain Event/Audit envelope.

Instead add a projection:

~~~text
Internal Domain Event
 -> Integration Event Projection
 -> CloudEvents v1.x envelope
 -> webhook/message bus/connector
~~~

Examples:
- Work status changed;
- Run waiting for human;
- Verification completed;
- Release confirmed.

Benefits:
- less proprietary connector/event integration;
- standard SDKs/protocol bindings;
- clean separation between authoritative audit data and integration transport.

CloudEvents payload includes references/IDs, not secret-heavy internal state.

---

## 8. Run/result archival — Tekton Results pattern

Repository:
- https://github.com/tektoncd/results

Tekton Results separates long-term result/log storage from the live Pipeline controller and allows execution resources to be garbage-collected after durable archival.

### Optimization

Apply the same lifecycle principle:

~~~text
Live Session/Attempt runtime state
 -> Run Receipt / durable domain records
 -> log/artifact archival
 -> ephemeral runtime/session resources may be GC'd
~~~

Do not keep live Worker/session state forever merely for audit.

Authoritative long-term objects:
- Run Receipt;
- structured events;
- manifests;
- Evidence;
- selected logs/artifacts by retention policy.

Raw PTY/runtime transcript can have shorter retention.

---

## 9. Lightweight PKI — step-ca

Repository:
- https://github.com/smallstep/certificates

step-ca supports:
- short-lived TLS/SSH certificates;
- ACME;
- OIDC/cloud identity provisioners;
- automated renewal;
- private CA operation.

### Decision

For M1 workload identity where enterprise PKI is unavailable:

~~~text
enterprise PKI
or
step-ca
~~~

is a lighter operational option than immediately deploying SPIRE.

Migration path:
- WorkloadIdentity contract stays provider-neutral;
- SPIRE can replace/augment step-ca at scale.

---

## 10. Promotion reconciliation — Argo CD / Flux practice

Repositories:
- https://github.com/argoproj/argo-cd
- https://github.com/fluxcd/flux2

GitOps systems emphasize:
- declared desired state;
- continuously observed actual state;
- reconciliation toward convergence;
- drift detection.

### Engineering-platform optimization

Release promotion should use the same conceptual model:

~~~text
Desired:
  exact approved Release Manifest digest
  exact target/environment/channel

Actual:
  observed external deployment state

Reconciliation:
  compare Desired vs Actual
  -> CONFIRMED
  -> DRIFTED
  -> UNKNOWN
  -> FAILED
~~~

This strengthens our existing external-operation reconciliation semantics.

Do not adopt Argo CD/Flux for embedded release unless the target happens to be Kubernetes/GitOps-native.

---

## 11. Supply-chain metadata APIs — Grafeas

Repository:
- https://github.com/grafeas/grafeas

Grafeas separates:
- Notes: metadata definitions;
- Occurrences: concrete observations for resources.

This resembles:
- vulnerability definition vs occurrence;
- provenance type vs artifact instance;
- policy finding vs concrete artifact finding.

### Decision

Useful conceptual reference for SupplyChainIntel read models.

Do not add Grafeas as core authority or M1 dependency.

GUAC + SBOM/OSV remains the stronger future impact-analysis direction.

---

## 12. Policy/config testing — Conftest

Repository:
- https://github.com/open-policy-agent/conftest

Conftest uses OPA/Rego to test arbitrary structured configuration.

### Optimization

M0 policy CI should include:
- opa test for policy units;
- table/property tests for authorization/assurance;
- optional Conftest checks against representative:
  - Requirement/Task manifests;
  - Release Manifests;
  - deployment/config files.

This turns policy assumptions into versioned tests.

---

## 13. Dependency capability analysis — Capslock

Repository:
- https://github.com/google/capslock

Capslock analyzes privileged capabilities reachable through Go dependency call graphs.

### Use

Optional security check for trusted platform-control binaries:
- Control API;
- Worker;
- Session Gateway;
- Action Gateway.

Potential rule:
- unexpected new filesystem/network/process capability in a trusted component triggers stronger review.

Not a general vulnerability scanner and not a release gate by default.

---

## 14. Scanner consolidation — Trivy

Repository:
- https://github.com/aquasecurity/trivy

Trivy combines:
- vulnerability scanning;
- SBOM;
- misconfiguration;
- secret detection;
- license scanning.

### Decision

SupplyChainIntel should not hard-code Syft+OSV.

Define scanner outputs in platform terms:
- SBOM Artifact;
- Vulnerability Evidence;
- Secret/Misconfiguration Finding.

Candidate implementations:
- Syft + OSV for transparent composability;
- Trivy for a broader single-tool option.

Choose per project/organization; keep findings normalized.

---

## 15. OpenLineage

Repository:
- https://github.com/OpenLineage/OpenLineage

OpenLineage standardizes Run/Job/Dataset lineage events with extensible facets.

### Assessment

Useful conceptual validation for:
- stable Run identity;
- input/output lineage;
- extensible metadata;
- backend-independent collection.

But it is data-pipeline oriented and does not model our engineering authority.

Decision:
- reference only;
- no direct domain dependency.

---

## 16. Refined implementation lessons

### A. Session access is a privileged access problem plus an engineering Run problem

Borrow JIT/session/audit mechanics from Teleport/Boundary.
Keep Run authority in engineering-platform.

### B. Artifact bytes and artifact distribution are separate

ArtifactStore owns bytes.
OCI/ORAS/Harbor may distribute selected artifacts.
Digest remains identity.

### C. Approval and admission are separate

Human Approval says an authorized human accepted an exact Release Manifest.

Release Admission says the exact manifest is still currently eligible to promote.

Both are required for high-risk production.

### D. Release is desired-state reconciliation

Never equate dispatch success with deployed truth.

### E. Internal events and integration events are separate

Internal audit/domain event remains authority.
CloudEvents is an interoperability projection.

### F. PKI and workload identity can mature incrementally

M1 can use enterprise PKI/step-ca.
SPIRE is a scale-triggered evolution.

---

## 17. New reference priority

### Immediate M0/M1 practice changes
- OPA + Conftest/OPA tests;
- step-ca fallback for short-lived mTLS if needed;
- CloudEvents projection contract;
- Release Admission semantics;
- desired-vs-actual promotion reconciliation;
- session grant modes / audit separation.

### M2/M3
- OCI/ORAS release artifact profile;
- Notation as optional OCI signature backend;
- Harbor when registry operations justify it;
- TUF/Uptane DistributionTrustProfile.

### Reference only
- Teleport/Boundary implementation;
- Argo CD/Flux;
- Grafeas;
- OpenLineage;
- Capslock.

---

## 18. Conclusion

Round 3 does not justify more mandatory infrastructure.

It improves five existing contracts:

1. Session Gateway becomes explicitly JIT/scoped/auditable.
2. Artifact model gains optional OCI distribution interoperability.
3. Release gains a last-moment Admission Decision before promotion.
4. external events gain a CloudEvents-compatible projection.
5. workload identity gains a lightweight step-ca path before SPIRE-scale deployment.

The design continues to converge toward fewer proprietary mechanisms and fewer mandatory dependencies.
