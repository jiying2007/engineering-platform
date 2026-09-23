# Reference-Aligned Implementation Profile v5

Date: 2026-09-23
Status: **Canonical implementation companion to architecture v1.2**
Supersedes: docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V4.md

Research basis:
- docs/research/OPEN_SOURCE_REFERENCE_REVIEW_2026-09-23.md
- docs/research/OPEN_SOURCE_REFERENCE_REVIEW_ROUND2_2026-09-23.md
- docs/research/OPEN_SOURCE_REFERENCE_REVIEW_ROUND3_2026-09-23.md
- docs/research/OPEN_SOURCE_REFERENCE_REVIEW_ROUND4_LARGE_SCALE_ENGINEERING_2026-09-23.md
- docs/research/OPEN_SOURCE_REFERENCE_SYNTHESIS_OPTIMIZATION_2026-09-23.md

## 1. Goal

Profile v5 absorbs large-scale CI/test/device practices without expanding M1 dependency breadth.

New emphasis:
- exact candidate-set integration;
- resource allocation vs health separation;
- trusted scheduler metadata;
- test attempt/verdict separation;
- resolved verification execution plan.

---

## 2. M1 stack remains unchanged

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
- Toxiproxy

No resource farm or ResultDB-like service is added to M1.

---

## 3. Integration Subject v5

Integration Subject now explicitly supports candidate sets.

Minimum concepts:

~~~text
integration_subject_id
mode
target_branch
base_commit
ordered_changes[]
dependency_graph
candidate_repository_trees[]
merge_strategy
queue_generation
content_digest
~~~

Modes:

~~~text
SINGLE
INDEPENDENT
DEPENDENT
SERIAL
~~~

M1 only implements SINGLE.

Schemas support future:
- multiple PRs;
- cross-repo dependencies;
- speculative gating;
- batch qualification.

Any change in:
- base commit;
- ordered change set;
- dependency graph;
- candidate tree

creates a new digest.

Prior Integration Evidence is not silently reused.

---

## 4. Integration Eligibility is platform-owned

Do not equate GitHub status/mergeability with engineering authority.

Control Plane derives:

~~~text
IntegrationEligibility
  candidate_digest
  base_freshness
  required_checks
  evidence_status
  approvals
  policy
  blockers
  verdict
~~~

Git provider statuses are projections/output.

For M1:
- exact candidate tree is verified;
- merge result is reconciled;
- base movement makes stale evidence inapplicable.

Prow Tide/LUCI CV/Zuul remain references for later batch/speculative scheduling.

---

## 5. Scheduler attributes have provenance

Worker/Device/Fixture capability metadata is not uniformly trusted.

Use:

~~~text
SchedulerAttribute
  key
  value
  source
  observed_at
  expires_at
  evidence_ref?
~~~

Source:

~~~text
PLATFORM_TRUSTED
ENROLLMENT_ATTESTED
WORKER_REPORTED
DISCOVERED
~~~

Policy may require specific source classes.

Examples:
- free_memory: WORKER_REPORTED is acceptable;
- production_environment: PLATFORM_TRUSTED;
- attached_probe: DISCOVERED;
- release_signer: ENROLLMENT_ATTESTED/PLATFORM_TRUSTED.

A Worker cannot self-report itself into a privileged pool.

---

## 6. Resource state uses orthogonal facets

For Worker/Device/Fixture resources:

### allocation_state

~~~text
AVAILABLE
LEASED
RECLAIMING
~~~

### health_state

~~~text
READY
DIRTY
MAINTENANCE
QUARANTINED
OFFLINE
~~~

### lease

~~~text
owner_run
fencing_token
heartbeat
expires_at
~~~

Internal lifecycle services:
- Reaper: recover stale leases/owners;
- Janitor: clean DIRTY resource before READY;
- Quarantine handling: block new trusted use.

This is introduced concretely in M2 for device resources.
M1 Worker model should already avoid a single overloaded status field.

---

## 7. Verification Plan vs Resolved Execution Plan

Declared Verification Plan answers:

> what proof is required?

Resolved Verification Execution Plan answers:

> exactly what will run on which target/resources and why?

Add immutable:

~~~text
ResolvedVerificationExecutionPlan
~~~

Includes:
- Verification Plan digest;
- selected TestDefinitions;
- TestVariants;
- Target Revision;
- required devices/resources;
- fixture topology;
- Procedure Revision;
- environment;
- attempt/retry policy;
- filtered/skipped/quarantined items with reason;
- scheduler constraint snapshot.

This plan is content-addressed.

M1 may use a trivial one-check plan.
M2 requires full implementation.

---

## 8. Test result model v5

Reserve platform-native records:

~~~text
TestDefinition
TestVariant
TestAttemptResult
TestVerdict
TestExoneration
~~~

### TestDefinition
Stable test identity and semantic purpose.

### TestVariant
Exact execution dimensions/configuration.

Example:
- target revision;
- firmware bundle;
- fixture;
- environment;
- build mode.

### TestAttemptResult
Immutable result of one execution attempt.

### TestVerdict
Derived aggregate for a TestVariant.

Example statuses:
- EXPECTED;
- UNEXPECTED;
- FLAKY;
- EXONERATED;
- SKIPPED;
- INCOMPLETE.

### TestExoneration
Separate authorized explanation/exception.

Rules:
- retry never erases previous failure;
- exoneration never rewrites attempt result;
- Verification decides whether verdict/exoneration is acceptable for Assurance Profile.

---

## 9. Expected result is contextual

Borrow LUCI's useful distinction.

A raw result:
- PASS;
- FAIL;
- SKIP;
- CRASH;
- TIMEOUT

is separate from whether that outcome is expected for the current variant.

This enables:
- negative tests;
- known unsupported variants;
- expected failure cases

without conflating them with generic PASS/FAIL.

Expectedness must be declared by Procedure/Test definition or authorized policy, not dynamically invented after seeing a result.

---

## 10. Local Result Sink enters at M2

For HIL/device/test framework integrations:

~~~text
Test Process
  -> Local Result Sink
      -> attempt events
      -> measurements
      -> attachment refs
      -> logs
  -> Procedure Execution Receipt
  -> Attestation Controller
  -> Evidence
~~~

Benefits:
- test code does not need Control Plane API credentials;
- uniform variant/invocation metadata;
- structured results independent of stdout/JUnit quirks;
- buffered upload/retry.

Result Sink is an internal component, not a new authority.

---

## 11. Procedure resource topology

Procedure Revision may require multiple resources atomically:

~~~text
required_resources:
  - role: primary_dut
    target_ref: ...
  - role: dock
    target_ref: ...
  - role: fixture
    capability: ...
    affinity_group: ...
~~~

Scheduler either allocates a compatible set or does not start the TestInstance.

This supports:
- robot + charging dock;
- paired communication devices;
- audio source/receiver;
- fixture/device affinity.

---

## 12. Quarantine is visible coverage loss

Quarantine record:

~~~text
quarantine_id
subject_kind
subject_ref
reason
owner
created_at
review_at
expires_at?
decision_ref
~~~

A quarantined required test/platform does not disappear from coverage.

Resolved Execution Plan records it.

Verification policy decides:
- BLOCK;
- REQUIRE_WAIVER;
- ALLOW for low-risk/non-required scope.

This prevents silent green builds due to skipped unstable tests.

---

## 13. Verification classes by engineering layer

Use explicit classes inspired by Yocto/large embedded systems:

~~~text
BUILD_SYSTEM
BUILD_OUTPUT
PACKAGE_COMPONENT
SDK_TOOLCHAIN
TARGET_RUNTIME
PERFORMANCE
HIL
SECURITY
STATIC_ANALYSIS
~~~

One class cannot silently satisfy another.

Example:
- build success != target boot success;
- target boot != application function;
- function pass != performance qualification.

---

## 14. Build Receipt refinement

Build Receipt adds:
- resolved dependency graph digest/reference;
- action/build definition digest;
- cache key if applicable;
- hermetic/control mode;
- dependency declaration completeness.

No requirement to adopt Buck2/Bazel/BuildStream.

These fields let legacy and modern build backends report comparable provenance.

---

## 15. Continuous Qualification is future domain behavior

After M2/M3, support recurring qualification not tied to one Work:

~~~text
ContinuousQualificationRun
~~~

Examples:
- fuzzing;
- soak;
- long-run device stability;
- KWS/audio regression;
- performance regression.

Findings attach to immutable Artifact/Release and may trigger:
- Incident;
- Quarantine;
- Regression Work;
- bisection/reproduction.

OSS-Fuzz/ClusterFuzz are design references.

Do not add this to M1.

---

## 16. Milestone mapping

### M0/M1
Implement:
- Integration Subject candidate freshness;
- SchedulerAttribute schema/provenance fields;
- reserve resource facets;
- reserve test/result schemas;
- trivial Resolved Verification Execution Plan.

### M2
Implement:
- Device resource facets/Reaper/Janitor;
- DeviceLabProvider;
- ProcedureExecutionService;
- Local Result Sink;
- TestVariant/Attempt/Verdict;
- multi-resource topology;
- quarantine coverage.

### M3+
Implement only if needed:
- speculative/batched candidate queues;
- cross-repo dependent gating scheduler;
- continuous qualification service;
- large result-history service.

---

## 17. Final rules added by v5

1. **Integration evidence binds an exact candidate set, not a PR label.**
2. **Provider merge status is not Integration Eligibility authority.**
3. **Scheduler capabilities have provenance and trust class.**
4. **Resource allocation and resource health are independent.**
5. **Declared Verification Plan and resolved execution plan are different immutable records.**
6. **Test attempt, expectedness, verdict and exoneration are separate.**
7. **Quarantined coverage remains visible.**
8. **Local test report files are not global Verification authority.**

Architecture v1.2 remains canonical.
Profile v5 is the current implementation companion.
