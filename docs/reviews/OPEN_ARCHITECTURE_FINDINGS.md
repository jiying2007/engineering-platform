# Open Architecture Findings

Date: 2026-09-23
Status: **Authoritative review backlog before v1.2**
Sources:
- Architecture Review Round 2
- Architecture Review Round 3
- Architecture Review Round 4
- Architecture Review Round 5

## Purpose

This document consolidates unresolved architecture findings that must be incorporated into the next baseline before M0 contracts are frozen.

The goal is to stop review findings from fragmenting across documents.

---

## P0 — Domain and authority

- [ ] Separate stable aggregate identities from immutable revision/child records.
- [ ] Keep Verification, Review and Decision as distinct domain concepts.
- [ ] Add Target / Target Revision as a formal authority.
- [ ] Add machine-readable Release Bundle / component-set semantics.
- [ ] Add versioned interface/protocol/ABI contracts.
- [ ] Define migration/rollback compatibility and product-level update ordering.
- [ ] Add immutable Baseline identity for relative/regression verification.
- [ ] Reserve project/organization/environment namespace from M0.

---

## P0 — Content identity and schema determinism

- [ ] Freeze canonical serialization profile for all content-addressed structured objects.
- [ ] Freeze digest algorithm identifier and digest string format.
- [ ] Separate business IDs from content digests.
- [ ] Freeze schema_name/schema_version/canonicalization_version behavior.
- [ ] Define historical schema-verification compatibility rules.
- [ ] Require mutable external references to be snapshotted or resolved to immutable revisions.
- [ ] Freeze trusted timestamp/time-expiry semantics.

---

## P0 — Run, attempt and interactive session consistency

- [ ] Add fenced execution epoch to active Run Attempt.
- [ ] Add immutable Run Input Manifest.
- [ ] Add provider-neutral immutable Checkpoint.
- [ ] Add immutable Run Receipt.
- [ ] Preserve explicit parent Run / checkpoint lineage for runtime switching/resume.
- [ ] Add structured Steering command with sequence, epoch and acknowledgement.
- [ ] Freeze command precedence/transition guards for pause/steer/takeover/abort/approval.
- [ ] Separate Control API from streaming Session Gateway.
- [ ] Define worker identity/enrollment/mTLS/revocation/protocol negotiation.

---

## P0 — External side effects and recovery

- [ ] Treat external side effects as at-least-once.
- [ ] Add idempotency keys and expected aggregate versions.
- [ ] Add external operation ledger.
- [ ] Add UNKNOWN outcome and reconciliation-before-retry semantics.
- [ ] Define checkpoint safe-point/reconciliation semantics.
- [ ] Prevent checkpoint restore from replaying already-confirmed external effects.
- [ ] Add device and Run fencing tokens.
- [ ] Freeze break-glass/degraded-mode behavior.

---

## P0 — Evidence, trust and verification

- [ ] Evidence records are immutable historical statements.
- [ ] Evidence applicability is derived separately from test result.
- [ ] Add Evidence Issuer identity and trust-domain policy.
- [ ] Separate raw measurement Evidence from derived Evaluation Result.
- [ ] Verification binds exact Subject Manifest digest.
- [ ] Verification records exact procedure, baseline, fixture/environment and policy inputs.
- [ ] Add statistical/repeated-test contract for non-deterministic verification.
- [ ] Add configuration-drift detection before authoritative Device/HIL verification.
- [ ] Quarantined issuer/device/fixture/artifact cannot create new trusted Verification.

---

## P0 — Policy, identity and human authority

- [ ] Connector service credential is not Human Authority.
- [ ] Add validated actor assertion/delegation for authority-bearing actions.
- [ ] Capability policy is actor/resource/environment scoped.
- [ ] Define deterministic policy inheritance/conflict rules.
- [ ] Explicit DENY dominates ALLOW unless named authorized exception applies.
- [ ] Production/non-overridable policy rules are defined.
- [ ] Approval binds exact Subject or Release Manifest digest.
- [ ] Define quorum/separation-of-duties fields.
- [ ] Keep privileged platform actions outside untrusted Runtime sandbox.

---

## P0 — Embedded configuration and release integrity

- [ ] Release Manifest binds exact Target Revision.
- [ ] Release Bundle binds exact component artifact set.
- [ ] Machine-readable compatibility constraints are evaluated before verification/release.
- [ ] Software version, Artifact digest and compatibility identity are distinct.
- [ ] Release records supported source configurations and migration paths.
- [ ] Product-level multi-component update state machine has safe intermediate states.
- [ ] Verification records actual tested target/hardware/configuration.
- [ ] Release promotion cannot replace one component without creating a new manifest/approval.
- [ ] Build environment/reproducibility level is recorded.
- [ ] Worker/CLI/Connector/Device Agent protocol version negotiation is defined.
- [ ] Platform upgrade compatibility for active Runs/Workflows is defined.

---

## P1 — Supply chain and archival

- [ ] Build provenance/attestation support is provider-neutral.
- [ ] SBOM support is optional but first-class where required.
- [ ] Two-phase Artifact upload/finalization.
- [ ] Immutable release artifacts have retention/lock policy.
- [ ] Closure Manifest is generated for closed Work.
- [ ] Exportable archival verification package is defined.
- [ ] Post-release field Evidence/Incident attaches without mutating historical Release.

---

## P1 — Operations and scale

- [ ] Worker resource/capability inventory.
- [ ] Admission control.
- [ ] Priority/fair scheduling and project quotas.
- [ ] Provider/API/CI/storage/device cost metrics.
- [ ] Data-residency/provider eligibility policy.
- [ ] Retention/redaction policy by record class.
- [ ] Disaster recovery RPO/RTO.
- [ ] Quarantine semantics across Worker/Device/Fixture/Artifact/Release/Issuer.
- [ ] OpenTelemetry-compatible trace/correlation propagation.

---

## M0 contract package required before implementation

The next architecture baseline should not be called implementation-frozen until these artifacts exist:

1. ADR: domain authority and aggregate boundaries.
2. ADR: PostgreSQL authority / Temporal orchestration.
3. ADR: Worker trust, enrollment and Session Gateway.
4. ADR: local tools vs privileged Platform Action Gateway.
5. ADR: content addressing, canonical serialization and manifests.
6. ADR: Evidence issuer/trust and Verification semantics.
7. ADR: policy composition, Human Authority and separation of duties.
8. ADR: idempotency, external side effects and reconciliation.
9. ADR: break-glass/degraded operation.
10. ADR: embedded Target/Variant/Compatibility/Release Bundle.
11. ADR: schema/protocol evolution.
12. JSON Schema for core domain records.
13. JSON Schema for Subject/Run Input/Run Receipt/Release/Closure manifests.
14. Command/Event envelope schema.
15. Steering command schema.
16. State transition/guard tables.
17. Policy schema.
18. Initial OpenAPI.
19. Worker control protocol.
20. Session Gateway protocol.
21. Device Agent protocol.
22. Failure-injection matrix.
23. Executable invariant/property-test specification.

---

## M1 no-go conditions

M1 is NO-GO if any of these remain true:

- content-addressed manifests do not hash deterministically;
- untrusted Runtime/Worker can self-issue trusted PASS Evidence;
- stale execution epoch can mutate Run state;
- confirmed external side effects can be replayed by resume;
- timeout of unknown external operation is retried blindly;
- concurrent steering/takeover/abort ordering is undefined;
- policy inheritance can silently widen parent restrictions;
- Human approval is not tied to an exact immutable digest;
- Release can swap a component artifact after approval;
- actual Device/HIL configuration is not checked against the subject;
- historical schema data cannot still be verified after upgrade;
- active Formal Runs can be stranded by Control Plane/Worker protocol upgrade.

---

## Current architecture status

### Stable and accepted

- WorkBuddy as front door, not direct Runtime proxy.
- Engineering Control Plane as workflow/state authority.
- Ubuntu as Engineering Execution Plane.
- Runtime providers are replaceable and interactive.
- Session Supervisor is first-class.
- Git/CI/Device/HIL are engineering fact sources.
- Runtime claims are not Verification.
- Human Takeover is first-class.
- Explore vs Formal boundary.
- Artifact/Evidence/Verification/Review/Release separation.
- PostgreSQL as business authority; Temporal as durable orchestration.
- Human Authority for production/irreversible operations.
- Capability-based policy.
- Build-once / verify / promote exact Artifact as preferred rule.

### Not yet implementation-frozen

- manifest canonicalization;
- trusted Evidence issuer model;
- revision/invalidation propagation;
- side-effect reconciliation semantics;
- structured steering ordering;
- target/product configuration authority;
- compatibility/migration/update semantics;
- schema/protocol evolution.

---

## Recommendation

Do not add more top-level architecture.

The next work should be:
1. incorporate this P0 backlog into a v1.2 baseline;
2. freeze ADRs and schemas;
3. write invariant tests before large implementation;
4. then start the M1 vertical slice.

The architecture review phase should now optimize for **semantic closure**, not additional features.
