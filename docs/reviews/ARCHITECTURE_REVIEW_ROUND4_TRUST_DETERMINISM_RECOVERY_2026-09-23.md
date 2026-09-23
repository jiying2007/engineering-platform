# Architecture Review — Round 4: Trust, Determinism and Recovery Semantics

Date: 2026-09-23
Reviewed baseline: `docs/architecture/AI_NATIVE_ENGINEERING_PLATFORM_V1_1.md`
Focus: content-address determinism, evidence trust, checkpoint/side-effect consistency, revision invalidation, interactive command ordering, schema evolution, time and recovery semantics.

## Result

The v1.1 target architecture remains valid. No new top-level plane is required.

This round found a deeper class of issues: the architecture already has immutable IDs, manifests, evidence and fencing, but **immutability alone does not establish trust or determinism**.

Before M0 schema freeze, the platform must define:
- how bytes are canonicalized before hashing;
- who is allowed to issue trusted Evidence;
- how checkpoint/resume interacts with already-dispatched external side effects;
- how requirement/task revisions invalidate downstream subjects;
- how human steering is ordered against runtime output;
- how schema changes preserve historical verification.

These are P0 because mistakes here become permanent compatibility or audit problems.

---

## P0-1 — Define canonical serialization before using digests as authority

The architecture relies heavily on:
- Run Input Manifest digest
- Subject Manifest digest
- Release Manifest digest
- Closure Manifest digest
- Artifact content digest
- payload_digest

A digest is only authoritative if the exact byte representation is deterministic.

Two semantically equal JSON objects can otherwise hash differently because of:
- object key ordering
- whitespace
- number representation
- Unicode normalization
- omitted/default fields
- timestamp formatting

Freeze one canonical representation for every content-addressed structured object.

Recommended contract:

```text
business object
  -> schema-versioned canonical form
  -> canonical serialization
  -> digest algorithm
  -> content address
```

For JSON, use a defined canonicalization scheme such as RFC 8785/JCS or an equivalently strict in-house profile.

Also freeze:
- digest algorithm identifier, e.g. sha256
- digest string format
- whether the schema/version field is inside the hashed payload
- canonical timestamp format
- canonical enum casing
- Unicode handling
- unknown-field policy

Never hash a pretty-printed API response ad hoc.

---

## P0-2 — Separate business ID from content ID

Objects such as Requirement, Work, Run and Release need stable business identities.

Artifacts/manifests also need content identities.

Do not conflate the two.

Example:

```yaml
artifact_id: ART-812
content:
  algorithm: sha256
  digest: ...
schema_version: artifact/v1
```

Business ID answers:
> which platform record is this?

Content digest answers:
> are these exact immutable bytes identical?

The same content may be referenced by multiple business records; the same business object may have multiple immutable revisions/manifests.

---

## P0-3 — Trusted Evidence requires issuer identity, not just immutability

An immutable Evidence record is still untrustworthy if any Runtime or compromised Worker can publish arbitrary PASS Evidence.

Every Evidence record needs an issuer/trust envelope:

```yaml
evidence:
  evidence_id: EV-...
  subject_digest: ...
  producer:
    type: ci | verifier | device-agent | human | external-system
    identity: ...
    trust_domain: ...
    software_version: ...
  issued_at: ...
  payload_digest: ...
  signature_or_attestation_ref: ...
```

Policy defines which producer classes may satisfy which verification requirements.

Examples:
- Runtime self-report: may produce engineering claim, not authoritative test Evidence.
- ordinary Worker process: may upload raw output but not automatically certify production verification.
- trusted CI verifier: may issue build/test Evidence.
- enrolled Device Agent: may issue device execution Evidence.
- Human reviewer: may issue signed Review/Decision, not fake machine-test Evidence.

This creates an explicit Evidence Issuer Registry / trust policy.

---

## P0-4 — Raw Evidence and Evaluation Result must be separable

For many tests, especially device/HIL and statistical tests, the raw measurement should not be fused with the PASS/FAIL calculation.

Prefer:

```text
Raw Evidence Artifact
  -> Evaluation Procedure Revision
  -> Evaluation Result Evidence
```

Example:
- current/temperature trace is immutable raw data;
- threshold rules are versioned;
- PASS/FAIL is derived.

This allows:
- re-evaluation under corrected thresholds;
- auditing calculation bugs;
- comparison across procedure revisions;
- preserving raw truth when interpretation changes.

Never overwrite old PASS/FAIL when a rule changes; create a new evaluation against the same raw artifact.

---

## P0-5 — Checkpoint must not imply rollback of external side effects

A filesystem/runtime checkpoint can be resumed, but already-executed external operations may not be reversible.

Example:

```text
Checkpoint CP-10
  -> create PR
  -> trigger CI
  -> reserve device
  -> process crashes
  -> resume CP-10
```

Restoring CP-10 must not blindly repeat those operations.

Therefore Checkpoint stores a durable external-operation cursor/ledger reference, and resume performs:

```text
restore local state
  -> reconcile external operation ledger
  -> discover confirmed/unknown effects
  -> only then continue
```

Define safe-point semantics:
- local-only checkpoint
- externally-quiescent checkpoint
- checkpoint-with-pending-reconciliation

A checkpoint is not a distributed transaction snapshot.

---

## P0-6 — Unknown-outcome operations need first-class reconciliation state

The current external-operation ledger has PLANNED/DISPATCHED/CONFIRMED/RECONCILING/FAILED.

Add an explicit `UNKNOWN` or equivalent condition when a timeout occurs after dispatch but before confirmation.

Examples:
- GitHub accepted PR creation but response was lost.
- device flash command reached agent but connection dropped.
- release system promoted but callback timed out.

Required rule:

```text
DISPATCHED
 -> CONFIRMED
 -> FAILED-before-side-effect

or

DISPATCHED
 -> UNKNOWN
 -> RECONCILING
 -> CONFIRMED / SAFE_TO_RETRY / MANUAL_INTERVENTION
```

Never convert a timeout directly into "retry from scratch" for non-idempotent actions.

---

## P0-7 — Revision supersession needs a dependency graph and invalidation propagation

The baseline says Requirement changes trigger impact analysis, but the invalidation semantics are not yet formal.

Track immutable dependency edges such as:

```text
TaskRevision derives_from RequirementRevision
RunInputManifest references TaskRevision
SubjectManifest references Artifact + TaskRevision
Verification evaluates SubjectManifest
Review reviews SubjectManifest
ReleaseManifest references Verification/Review
```

When REQ r3 is superseded by r4, the platform must not mutate r3 children. It computes an impact graph and marks applicability/projections:

- still applicable
- requires review
- requires replan
- superseded
- invalid for current release

The rule must be dependency-based, not "invalidate everything" and not "leave everything valid".

---

## P0-8 — Define compatible vs material changes

Not every new Requirement/Task revision invalidates every Evidence item.

Add an impact classification model:

```text
NO_IMPACT
DOC_ONLY
IMPLEMENTATION_ONLY
VERIFICATION_IMPACT
SAFETY/RISK_IMPACT
FULL_REQUALIFICATION
```

This classification itself is a Decision with:
- actor/authority
- old/new revision digest
- dependency diff
- rationale
- policy bundle

Policy may force minimum impact for certain change classes.

This avoids both unsafe evidence reuse and wasteful full requalification.

---

## P0-9 — Interactive steering needs ordered durable commands

PTY text alone is not sufficient for authoritative steering because stream timing can race with runtime output and reconnects.

Every formal steering instruction should have:
- steering_id
- run_id
- expected execution_epoch
- actor
- sequence
- content digest
- created_at
- delivery state
- acknowledged_at
- optional supersedes/retracts reference

Recommended lifecycle:

```text
ACCEPTED
 -> DELIVERED
 -> ACKNOWLEDGED

or
ACCEPTED
 -> EXPIRED / REJECTED / STALE_EPOCH
```

The Session Gateway may display raw terminal input, but authoritative Steering is a structured Control Plane command.

This ensures audit can answer exactly which instruction the runtime received and in what order.

---

## P0-10 — Define command ordering and concurrency ownership

Concurrent actors may issue:
- pause
- steer
- takeover
- abort
- permission approval
- runtime-generated clarification

Freeze precedence/guards.

Example policy:
- ABORT dominates future steering.
- HUMAN_TAKEOVER transfers control_owner and rejects runtime write actions.
- PAUSE prevents new privileged action dispatch but may allow cleanup.
- stale expected aggregate version or execution epoch rejects command.
- release-takeover requires an explicit checkpoint/result snapshot.

Write this as a transition/guard table, not implicit handler code.

---

## P0-11 — Policy bundle composition/conflict rules are missing

Capability-based policy is correct, but the system also needs deterministic conflict resolution across:
- company policy
- business-unit policy
- project policy
- repository policy
- environment policy
- task-specific grant
- emergency/break-glass policy

Freeze one model.

Recommended secure default:
- explicit DENY dominates ALLOW;
- lower scopes may narrow but not silently widen parent restrictions;
- widening requires an explicitly authorized exception Decision;
- production policies are non-overridable except through named emergency policy;
- every effective decision records the full evaluated policy bundle digest.

Without this, "capability based" still produces unpredictable authorization.

---

## P0-12 — Schema evolution must preserve historical verification

Every durable payload needs:
- schema_name
- schema_version
- canonicalization_version

Readers must be able to verify historical signatures/digests years later.

Rules:
- never reinterpret an old hash using a new schema;
- additive API fields do not change historical canonical payloads;
- migration creates a new representation linked to the old one rather than rewriting immutable historical records;
- old schema validators remain available for retained release/audit data;
- unknown future fields must not silently participate differently in hashing.

M0 must define compatibility policy before v1 schemas are published.

---

## P0-13 — External references inside immutable manifests must themselves be immutable/snapshotted

A manifest is not truly immutable if it contains only mutable external URLs such as:
- branch name
- latest document URL
- mutable package tag
- "current" procedure
- wiki page URL

Every authority-bearing external reference must resolve to:
- immutable commit/revision/version, and/or
- captured snapshot Artifact with digest.

Human-friendly URLs may remain metadata only.

---

## P0-14 — Trusted time semantics are required for expiry/leases/approval

Sequence/version controls ordering, but leases, credential expiry and approval windows depend on time.

Freeze:
- Control Plane server time as authority for business timestamps;
- monotonic duration measurement within Worker where applicable;
- maximum tolerated clock skew;
- lease expiry evaluated server-side;
- approval/credential expiry evaluated by trusted service time, not model/Worker wall clock;
- timestamp precision/format in canonical payloads.

Do not rely on arbitrary workstation time for authority.

---

## P0-15 — Namespace/project identity should be promoted from P1 to M0

All business/resource IDs must have a scope boundary from the beginning.

At minimum define:

```text
organization
project/product
environment
repository/resource
```

Reasons:
- ID uniqueness
- authorization
- policy inheritance
- artifact visibility
- worker eligibility
- device ownership
- retention
- future multi-team use

M1 may have one project, but the schemas should not assume one global namespace.

---

## P1-1 — Define trust levels for Workers and verifiers

Not every Worker should be equally trusted.

Example classes:
- DEV_WORKER
- VERIFIED_BUILD_WORKER
- DEVICE_LAB_WORKER
- RELEASE_WORKER

Policy decides what Evidence/Artifact each class may issue.

A general developer machine may execute a Formal Run but should not automatically become a release signing authority.

---

## P1-2 — Device identity should include hardware/fixture attestation where practical

Device/HIL Evidence quality depends on knowing the real tested target.

Record:
- device_id
- board revision
- serial/identity source
- fixture ID/revision
- calibration version/status
- device-agent identity/version
- bootloader/firmware pre-state when relevant

High-value environments may later add hardware-backed attestation, but the domain model should not require it for M1/M2.

---

## P1-3 — Separate confidentiality labels from context trust labels

"UNTRUSTED" means the content must not control policy. It does not mean the content is public.

Add independent data classification:

```text
PUBLIC
INTERNAL
CONFIDENTIAL
RESTRICTED
SECRET_REFERENCE_ONLY
```

Enforce classification on:
- runtime provider routing
- transcript retention
- artifact store access
- external network/tool use
- WorkBuddy summaries
- model/provider eligibility

Trust and confidentiality are orthogonal.

---

## P1-4 — Define provenance graph query requirements

The platform value depends on being able to answer lineage questions efficiently.

M0/M1 should define read models for:
- Requirement -> Release
- Release -> exact Artifact -> source
- Artifact -> producer Run/Attempt
- Verification -> Evidence -> raw Artifact/procedure
- Run -> steering/takeover/policy decisions
- revision supersession impact

PostgreSQL relational tables are sufficient initially; do not add a graph database unless measurement shows a need.

---

## P1-5 — Define archival verification package

For long-lived products, Closure Manifest alone may reference services that disappear.

For important Release classes, generate an exportable verification package containing:
- manifests
- canonical schemas
- signatures/attestations
- evidence metadata
- required raw evidence/artifact references or bundled copies
- verification/review/decision records
- public keys/certificate chain references required for later validation

The goal is future audit independent of live Control Plane APIs.

---

## P1-6 — Disaster recovery objectives must be explicit

Define at least:
- PostgreSQL RPO/RTO
- object-store durability/replication
- Temporal recovery expectations
- key/credential broker recovery
- Session Gateway stateless recovery
- Worker re-enrollment/reconnect
- artifact manifest backup
- audit/event retention

Formal releases/evidence normally need stronger durability than transient sessions/transcripts.

---

## P1-7 — Add invariant/property testing, not only example tests

Architecture invariants should become executable tests.

Examples:
- stale execution epoch can never mutate Run;
- modified Release Manifest invalidates prior approval;
- stale Evidence can never satisfy current Verification;
- duplicate command cannot duplicate a privileged side effect;
- Human Takeover prevents Runtime writes;
- superseded revision cannot silently remain current;
- revoked issuer cannot create newly trusted Evidence.

Use state-machine/property-based tests in addition to ordinary unit tests.

---

## P1-8 — Avoid event-sourcing everything

The architecture needs append-only audit events but does not require full event sourcing as the sole state model.

Recommended:
- relational current projections are authoritative business state;
- immutable audit/event journal records important transitions;
- selected immutable manifests/evidence are content addressed;
- rebuild-from-all-events is not an M1 requirement.

This keeps M1 operationally simpler while preserving auditability.

---

## Required additions to M0 contract freeze

Add these to the existing M0 list:

1. Canonical serialization and digest profile.
2. ID/namespacing profile.
3. Schema compatibility/evolution policy.
4. Evidence Issuer / trust-domain schema.
5. Raw Evidence vs Evaluation Result schema.
6. External-operation UNKNOWN/reconciliation semantics.
7. Revision dependency/invalidation graph.
8. Change-impact classification policy.
9. Structured Steering command schema and ordering.
10. Command precedence/transition guard table.
11. Policy inheritance/conflict-resolution contract.
12. Trusted-time/lease-expiry contract.
13. Run Input Manifest and Run Receipt schemas.
14. Archival/retention classification model.
15. Executable invariant/property-test specification.

---

## M1 additional go/no-go checks

Add:

- canonical digest is stable across independent implementations;
- old schema fixture remains verifiable after schema upgrade;
- unauthorized Evidence issuer cannot satisfy Verification;
- PASS evidence becomes inapplicable when Subject Manifest changes;
- raw evidence can be re-evaluated without mutation;
- checkpoint resume does not repeat confirmed side effects;
- UNKNOWN external operation reconciles before retry;
- concurrent pause/takeover/steer has deterministic result;
- steering to stale execution epoch is rejected;
- parent DENY cannot be silently widened by child policy;
- lease/approval expiry is server-authoritative;
- multi-project IDs/resources cannot cross authorization boundaries;
- release audit can be reconstructed from immutable data without trusting current mutable URLs.

---

## Final assessment

The architecture is converging well.

The largest residual risk is no longer "wrong component architecture"; it is **semantic ambiguity at authority boundaries**.

The next freeze should prioritize five properties:

1. **Determinism** — the same immutable subject always hashes the same way.
2. **Authenticity** — trusted Evidence has a verifiable issuer.
3. **Consistency** — resume/retry never silently duplicates external effects.
4. **Propagation** — revisions deterministically affect downstream applicability.
5. **Ordering** — concurrent human/runtime commands have deterministic authority and sequence.

Once these are specified, the project can safely transition from architecture review into schema/ADR implementation.
