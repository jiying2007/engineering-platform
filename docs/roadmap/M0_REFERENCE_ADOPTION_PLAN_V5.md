# M0 Reference Adoption Plan v5

Date: 2026-09-23
Status: **Current execution plan**
Supersedes: docs/roadmap/M0_REFERENCE_ADOPTION_PLAN_V4.md

Parents:
- docs/architecture/AI_NATIVE_ENGINEERING_PLATFORM_V1_2_FINAL.md
- docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V5.md

## 1. Principle

M0 still targets the smallest credible trusted engineering loop.

Round 4 adds schema precision for integration candidates, scheduler metadata and verification execution, but does not add large new M1 services.

---

## 2. Phase 1 — Core domain/data freeze

Freeze:

1. Common schema envelope.
2. Canonical serialization/digest profile.
3. Requirement/Task/Run/Artifact/Evidence/Verification schemas.
4. Subject/RunInput/RunReceipt/Closure manifests.
5. Command/Event/Steering/Session Grant envelopes.
6. Integration Subject v5:
   - candidate set;
   - base commit;
   - dependency graph;
   - candidate trees;
   - mode;
   - queue generation.
7. Integration Eligibility projection.
8. SchedulerAttribute schema with provenance.
9. Resource allocation/health facets.
10. TestDefinition/TestVariant/TestAttemptResult/TestVerdict/TestExoneration schemas.
11. Resolved Verification Execution Plan schema.
12. Quarantine schema.
13. VerificationClass enum.
14. Release Admission / Promotion Reconciliation schemas.
15. UNKNOWN external-operation semantics.

M1 only exercises a subset, but schema must not encode single-PR/single-test assumptions.

---

## 3. Phase 2 — PostgreSQL / Temporal / OPA / ArtifactStore

Unchanged from v4.

Required:
- PostgreSQL authority/outbox;
- Temporal workflow/replay/cancellation;
- OPA policy tests;
- exact-digest ArtifactStore.

Add representative OPA cases:
- Worker attempts to use WORKER_REPORTED attribute for a privileged scheduling decision -> deny;
- quarantined resource -> deny;
- stale Integration Candidate -> cannot merge.

---

## 4. Phase 3 — Integration Candidate spike

Implement M1 SINGLE mode only.

Flow:

~~~text
PR/change head
 + current target base
 -> construct exact candidate tree
 -> Integration Subject digest
 -> required CI
 -> Evidence
 -> Integration Eligibility
 -> merge dispatch
 -> fetch actual final commit/tree
 -> reconcile
~~~

Prove:
- base moves -> old candidate evidence becomes stale;
- final merge tree mismatch -> new candidate/verification required;
- GitHub status alone cannot cause merge without Control Plane eligibility.

Do not implement batch/speculative queue yet.

---

## 5. Phase 4 — Runtime/session

Unchanged core:
- Session Grant;
- Session Gateway;
- Session Supervisor;
- LocalUbuntuTransport;
- CodexProvider;
- Steering/Pause/Resume/Takeover;
- execution epoch.

Also freeze Worker SchedulerAttribute source rules even with one Worker.

---

## 6. Phase 5 — Artifact/Evidence/Attestation

Implement:
- Native Build Receipt;
- CI/test Evidence;
- thin Attestation Controller;
- standard-compatible projection;
- Verification.

Use a trivial TestDefinition/Variant/Attempt model for M1 CI results so M2 can extend it without schema breakage.

Example M1 variant:
- repository;
- candidate digest;
- OS/toolchain;
- CI job/profile.

---

## 7. Phase 6 — Resolved Verification Execution Plan

M1 version may be simple:

~~~text
ResolvedPlan
  verification_plan_digest
  candidate_subject_digest
  selected_checks[]
  skipped_checks[]
  quarantine_refs[]
  environment
~~~

Prove:
- required check cannot disappear without an explicit filtered/skipped reason;
- quarantine remains visible;
- plan digest changes if selected coverage changes.

Full target/device/fixture scheduling is deferred to M2.

---

## 8. Phase 7 — Release Admission / Promotion model

Same as v4.

Add invariant:
- candidate/merge subject used by Release Manifest must reconcile to final integrated source.

---

## 9. Phase 8 — Reliability / failure injection

Use Toxiproxy and fake Git/CI provider.

Add scenarios:
- base branch changes during CI;
- duplicate merge dispatch;
- merge succeeds but response lost;
- CI result arrives for stale candidate;
- Worker reconnect with stale scheduler attributes;
- resource is quarantined while queued work waits.

---

## 10. M2 resource scheduler spike

Implement from real HIL pilot:

### Resource facets
- allocation;
- health;
- lease/fencing;
- heartbeat;
- SchedulerAttributes.

### Lifecycle
- Acquire;
- heartbeat;
- Release -> DIRTY where needed;
- Janitor cleanup;
- Reaper stale lease;
- Quarantine.

Use labgrid underneath where practical.

Boskos/LUCI Swarming patterns guide state/scheduling semantics.

---

## 11. M2 Procedure / Result spike

Using motor/HIL pilot, implement:

- Procedure Revision;
- Resolved Verification Execution Plan;
- TestDefinition;
- TestVariant;
- TestAttemptResult;
- TestVerdict;
- TestExoneration;
- Local Result Sink;
- measurements/attachments;
- multi-resource topology.

Compare execution mechanics with:
- pytest;
- pytest-embedded;
- OpenHTF.

Use Zephyr Twister and Android Tradefed as model references.

---

## 12. M2 quarantine acceptance tests

Required:
- quarantined required test remains visible;
- quarantine cannot silently turn required coverage green;
- waiver/Assurance policy required where applicable;
- resource quarantine immediately blocks new authoritative allocation;
- historical results remain immutable.

---

## 13. Deferred M3 — speculative/multi-repo gating

Trigger:
- concurrent merge queue becomes bottleneck;
- multiple repositories have coupled changes;
- product integration needs pre-merge qualification across repos.

Then implement:
- INDEPENDENT/DEPENDENT/SERIAL modes;
- ordered candidate queues;
- cross-repo dependency DAG;
- speculative candidate sets;
- queue window/resource limit.

Use Zuul/Prow/LUCI CV practices as references.

---

## 14. Deferred M3+ — result history / continuous qualification

Only after volume justifies it:

- test history querying;
- flaky classification;
- historical trend;
- large result store;
- recurring fuzz/soak/regression;
- automatic bisection.

Use LUCI ResultDB / ClusterFuzz as references.

Do not build this before M1/M2 produces real result volume.

---

## 15. M0 exit criteria additions

M0 now also requires:

- Integration Subject supports exact candidate tree and future candidate-set fields;
- Integration Eligibility is distinct from provider merge status;
- scheduler attributes carry provenance;
- resource state schema separates allocation and health;
- test result schema separates attempt/verdict/exoneration;
- Resolved Verification Execution Plan preserves selected/skipped/quarantined checks;
- base-drift and stale-result integration tests pass.

M0 still does not require:
- resource farm;
- labgrid;
- ResultDB service;
- multi-DUT scheduler;
- speculative gating.

---

## 16. M1 target

~~~text
WorkBuddy
 -> Requirement READY
 -> Task Revision
 -> Run Input Manifest
 -> Codex Run
 -> exact Integration Subject
 -> CI/Test Attempts
 -> Artifact/Evidence
 -> Verification
 -> Integration Eligibility
 -> merge/reconcile
 -> Closure Manifest
 -> WorkBuddy
~~~

with the existing small dependency set.

This is now both:
- AI-native;
- merge-exact;
- evidence-driven.
