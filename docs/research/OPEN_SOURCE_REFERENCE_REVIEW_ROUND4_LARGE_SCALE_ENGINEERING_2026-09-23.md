# Open Source Reference Review — Round 4

Date: 2026-09-23
Status: **Archived research / implementation input**
Parent:
- docs/architecture/AI_NATIVE_ENGINEERING_PLATFORM_V1_2_FINAL.md
- docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V4.md

## 1. Scope

Round 4 focused on large-scale engineering organizations and systems rather than tool catalogs:

- Kubernetes Prow/Tide and Boskos;
- Chromium LUCI Change Verifier, Swarming and ResultDB;
- Zuul speculative gating;
- Zephyr Twister;
- Android Trade Federation;
- Yocto OEQA/testimage;
- Meta Buck2;
- BuildStream/Pants;
- OSS-Fuzz/ClusterFuzz.

The goal was to identify mature patterns for:
- merge gating;
- multi-repo integration;
- scarce-resource scheduling;
- hardware lifecycle;
- test result semantics;
- build reproducibility;
- large-scale continuous qualification.

---

## 2. Integration gating — Prow Tide

Reference:
- https://github.com/kubernetes-sigs/prow
- https://docs.prow.k8s.io/docs/components/core/tide/

Tide manages a pool of mergeable PRs and ensures they have up-to-date passing tests against the latest base branch before merge.

Important behaviors:
- base branch movement makes older test results stale;
- PRs are automatically retested;
- batch testing can qualify multiple PRs together;
- merge pool state is distinct from individual PR state;
- merge mode is explicit.

### Absorb

Our existing Integration Subject should support a queue/batch context:

~~~text
IntegrationCandidateSet
  target_branch
  base_commit
  ordered_changes[]
  dependency_graph
  candidate_tree
  merge_strategy
  queue_generation
~~~

Evidence binds the exact candidate set, not merely a PR head.

When base_commit or ordered_changes change:
- candidate digest changes;
- prior Integration Evidence becomes inapplicable.

No new top-level aggregate is required; this can be an immutable Integration Subject subtype/manifest.

---

## 3. Speculative multi-change / multi-repo gating — Zuul

Reference:
- https://zuul-ci.org/docs/zuul/latest/gating.html

Zuul's dependent pipeline tests changes exactly as they are expected to merge while still running many gate jobs in parallel.

Key practices:
- speculative execution;
- ordered change queue;
- changes behind a failed change are re-tested without it;
- cross-project dependencies form a DAG;
- related projects can share queues;
- pipeline window controls resource pressure;
- different pipeline semantics: independent, dependent, serial.

### Major optimization

Extend Integration Subject with:

~~~text
integration_mode:
  INDEPENDENT
  DEPENDENT
  SERIAL

candidate_set:
  base
  ordered changes
  cross-repo dependencies
  expected resulting trees
~~~

For multi-repo embedded products:
- Linux/BSP change can depend on MCU protocol change;
- test tooling/fixture change can be included in the same candidate set;
- release qualification can evaluate one combined engineering state before any individual merge.

This is more precise than "PR A depends on PR B" metadata alone.

### Do not copy

Do not build a Zuul clone in M1.

M1 can use one change at a time.
The schema must merely avoid blocking future speculative/multi-repo gating.

---

## 4. Resource lifecycle — Boskos

Reference:
- https://github.com/kubernetes-sigs/boskos

Boskos models scarce resources with:
- resource type/name;
- state;
- owner;
- acquire/release/update;
- heartbeat-like last update;
- stale-owner reaping;
- dirty-resource janitor cleanup;
- static and dynamic resources.

Important state idea:

~~~text
free
 -> busy
 -> dirty
 -> cleaning
 -> free
~~~

plus stale-resource recovery.

### Absorb

Our Device/Worker/Fixture resource model should separate:

1. **allocation ownership**
2. **operational health/cleanliness**

A resource can be:
- not leased but dirty;
- leased and healthy;
- unleased but quarantined;
- stale-owned and waiting reaping.

Recommended facets:

~~~text
allocation_state:
  AVAILABLE
  LEASED
  RECLAIMING

health_state:
  READY
  DIRTY
  MAINTENANCE
  QUARANTINED
  OFFLINE

lease:
  owner_run
  fencing_token
  heartbeat
  expires_at
~~~

Add internal equivalents of:
- Reaper: recover stale ownership;
- Janitor: restore dirty resources to READY.

This is stronger than one flat Device state enum.

---

## 5. Trusted scheduler dimensions — LUCI Swarming

References:
- LUCI Swarming source/docs
- https://chromium.googlesource.com/infra/luci/luci-go/

Swarming uses dimensions to match tasks to bots.

Critically, LUCI distinguishes trusted dimensions set by the server from arbitrary dimensions a bot can self-report.

It also has:
- pools;
- bot IDs;
- task dimensions;
- quarantined/maintenance states;
- task tags;
- source revision/context metadata.

### Major optimization

Worker/Device capability metadata needs provenance.

Do not trust:

~~~text
worker says: production_signer=true
worker says: secure_lab=true
~~~

Introduce:

~~~text
SchedulerAttribute:
  key
  value
  source:
    PLATFORM_TRUSTED
    ENROLLMENT_ATTESTED
    WORKER_REPORTED
    DISCOVERED
  observed_at
  expires_at
~~~

Scheduling/policy rules specify which attribute sources are acceptable.

Examples:
- CPU/RAM free space may be worker-reported;
- environment=production must be platform-trusted;
- attached debug probe may be discovered;
- release-signer capability must be enrollment/policy trusted.

This closes a subtle privilege-escalation hole.

---

## 6. Change verification — LUCI CV

Repository:
- https://github.com/luci/luci-go (cv package)

LUCI Change Verifier:
- manages pre-submit verification;
- starts Runs based on project/change state;
- manages tryjobs;
- supports combinable/multi-CL runs;
- submits changes after checks;
- has explicit stable APIs and end-to-end fake-service testing.

### Absorb

Strengthen Integration domain separation:

~~~text
Change
 -> Verification Candidate
 -> Verification Run(s)
 -> Candidate Verdict
 -> Merge/Submit
 -> Reconcile final commit/tree
~~~

Do not let Git provider statuses become the sole merge authority.

Control Plane should calculate its own Integration Eligibility projection from:
- candidate digest;
- required checks;
- current Evidence;
- approvals/policy;
- base freshness.

Provider status is an output projection.

---

## 7. Test result semantics — LUCI ResultDB

References:
- LUCI ResultDB source
- Chromium ResultDB integration

Important model distinctions:
- Invocation;
- Test ID;
- Test Variant;
- individual Test Result/attempt;
- expected vs unexpected;
- final Variant status;
- flaky;
- exonerated;
- artifacts;
- query/history.

Chromium uses ResultDB as the authoritative test-result source rather than trusting arbitrary test JSON.

### Major optimization

Our Procedure/Evidence model should distinguish:

~~~text
TestDefinition
TestVariant
TestAttemptResult
TestVerdict
TestExoneration
~~~

Example:

~~~text
TestDefinition:
  motor.low_speed_lock

Variant:
  target=PCR02-V3
  motor_fw=0.6.0
  fixture=HIL-A
  temperature=25C

Attempts:
  PASS
  FAIL
  PASS

Verdict:
  FLAKY

Exoneration:
  optional separate authorized record
~~~

A retry does not erase the failed attempt.

An exoneration does not mutate the attempt result.

This aligns strongly with our existing immutable Evidence principle.

---

## 8. Result Sink pattern

Chromium test harnesses send structured results to ResultDB through ResultSink rather than letting local report files determine global pass/fail.

Android Tradefed also supports dedicated Result Reporter abstractions and ResultDB reporters.

### Absorb for M2

Add an internal local collector pattern:

~~~text
Test Process
 -> Local Result Sink
     -> test attempt events
     -> measurements
     -> artifacts/log refs
 -> Procedure Execution Receipt
 -> Attestation Controller
 -> Evidence
~~~

The local Result Sink:
- validates structured test records;
- adds invocation/variant context;
- buffers/uploads;
- is not Release authority.

This reduces every test harness needing direct Control Plane API knowledge.

Do not require this in M1.

---

## 9. Zephyr Twister — resolved test plan and hardware map

Reference:
- https://docs.zephyrproject.org/latest/develop/twister/

Twister models:
- TestCase;
- TestSuite;
- TestInstance = suite on a platform;
- target board/revision;
- hardware map;
- fixtures;
- multiple DUTs;
- device flash/run;
- filtering/skipping;
- quarantine;
- machine-readable testplan/report outputs.

### Major optimization

Separate declared Verification Plan from resolved execution plan.

~~~text
Verification Plan
  says what must be verified

Resolved Verification Execution Plan
  says exactly which:
    test definition
    variant
    target revision
    device(s)
    fixture(s)
    procedure revision
    environment
    attempt policy
  will execute
~~~

The resolved plan is immutable and content-addressed.

It records:
- selected tests;
- filtered tests;
- skipped tests and reasons;
- quarantined tests/platforms;
- required fixture capabilities;
- required device count.

This prevents silent coverage loss.

---

## 10. Multi-DUT and fixture topology

Twister supports tests requiring multiple devices and fixture matching.

### Absorb

Procedure requirements should support:

~~~text
required_resources:
  - role: primary_dut
    target: ...
  - role: peer_dut
    target: ...
  - role: fixture
    capability: gpio_loopback
    affinity_group: channel_a
~~~

Resource scheduler must allocate a compatible set atomically or fail the plan.

This is important for:
- robot + dock;
- two-board communication;
- audio source/receiver;
- paired motor/control fixtures.

---

## 11. Quarantine is explicit test metadata

Twister can quarantine tests/platforms rather than silently ignoring unstable cases.

### Absorb

Quarantine must remain explicit in Verification coverage:

~~~text
QUARANTINED
  reason
  owner
  created_at
  expiry/review date
  affected tests/targets
~~~

A required AC cannot become PASS merely because the failing test was quarantined.

Policy decides whether quarantine:
- blocks Verification;
- requires Waiver/Risk Acceptance;
- is non-blocking for a specific Assurance Profile.

---

## 12. Android Trade Federation — device manager / scheduler / invocation

References:
- Android Tradefed architecture
- source.android.com

Tradefed separates:
- Device Manager: device allocation and online state;
- Test Command Scheduler: associates commands with devices;
- Build Provider: prepares build resources/BuildInfo;
- Test Invocation: actual execution;
- Result Reporter: consumes lifecycle/result events;
- log saver/metric collectors;
- retry/rescheduling.

### Absorb

This validates splitting our M2 architecture into:

~~~text
Resource Manager
Scheduler
Input/Build Resolver
Procedure Invocation
Result Sink/Reporter
Evidence/Verification
~~~

Do not collapse all of these into DeviceLabProvider.

It also reinforces:
- device allocation state != online state;
- execution lifecycle != reporting lifecycle;
- logs/metrics are attachments, not verdicts.

---

## 13. Yocto OEQA — verification classes by layer

References:
- Yocto Test Environment Manual
- OEQA/testimage

Yocto distinguishes:
- selftests of build tooling;
- runtime image tests;
- SDK tests;
- package ptests;
- build-performance tests.

### Absorb

Verification Plan should classify evidence by layer:

~~~text
BUILD_SYSTEM
BUILD_OUTPUT
TARGET_RUNTIME
PACKAGE_COMPONENT
SDK_TOOLCHAIN
PERFORMANCE
HIL
SECURITY
~~~

A PASS in one class does not substitute for another.

For embedded Linux this avoids conflating:
- "rootfs built"
- "image boots"
- "service works"
- "performance acceptable".

---

## 14. Build graph practices — Buck2 / BuildStream / Pants

Repositories:
- https://github.com/facebook/buck2
- https://github.com/apache/buildstream
- https://github.com/pantsbuild/pants

Common mature practices:
- explicit dependency graphs;
- hermetic/controlled execution;
- content-addressed/cache-keyed actions;
- fine-grained invalidation;
- remote cache/execution;
- build graph introspection.

### Absorb

No build-system migration is required.

But Build Receipt should explicitly capture:
- resolved dependency graph digest or reference;
- action/build definition digest;
- cache key where meaningful;
- local vs controlled/hermetic mode;
- dependency declaration completeness.

Reproducibility class remains important.

---

## 15. Continuous qualification — OSS-Fuzz / ClusterFuzz

Repositories:
- https://github.com/google/oss-fuzz
- https://github.com/google/clusterfuzz

Relevant practices:
- continuous execution independent of one release;
- crash deduplication;
- testcase minimization;
- regression/bisection;
- automatic issue lifecycle;
- large-scale distributed qualification.

### Future optimization

Add a general concept after M2/M3:

~~~text
Continuous Qualification
~~~

It runs against current/recent product configurations outside one Work.

Examples:
- fuzzing;
- soak tests;
- stability loops;
- audio/KWS regression;
- long-run motor/device tests.

Findings attach to existing Release/Artifact and can trigger:
- Incident;
- Quarantine;
- Regression Work;
- bisection.

Do not add this to M1.

---

## 16. Consolidated lessons

### A. Integration is a queue/DAG problem, not only a PR problem

Use exact speculative candidate sets and invalidate evidence when queue/base composition changes.

### B. Resource health and allocation are orthogonal

A free device can be dirty/quarantined.
A leased device can go offline.

### C. Scheduler attributes have trust provenance

Never authorize high-risk scheduling based on worker self-report.

### D. Test attempt != test verdict

Keep every attempt immutable.
Derive flaky/expected/exonerated verdicts separately.

### E. Declared plan != resolved plan

Verification must preserve exactly what was selected, filtered, skipped, quarantined and executed.

### F. Hardware topology is part of the test subject

Multi-DUT/fixture relationships must be explicit.

### G. Result storage should be structured and independent of local test report files

A test's stdout/JSON should not be the global authority.

---

## 17. Implementation priority impact

### M0 schema changes
- enrich Integration Subject for candidate sets/dependency DAG;
- add SchedulerAttribute provenance;
- reserve Resource allocation/health facets;
- reserve TestDefinition/TestVariant/TestAttempt/Verdict/Exoneration schemas;
- add Resolved Verification Execution Plan schema.

### M1 implementation
Only Integration Candidate freshness is relevant immediately.

Do not build:
- test ResultDB;
- resource farm;
- multi-DUT scheduler

inside M1.

### M2
Implement resource facets, lab scheduling, resolved test plan and result model during real HIL pilot.

### M3+
Consider continuous qualification and speculative multi-change gating if scale justifies it.

---

## 18. Conclusion

Round 4 strengthens the existing architecture without adding new top-level systems.

The most important newly absorbed principles are:

1. **Test exactly the integration state that will merge.**
2. **Treat integration as an ordered candidate set/DAG when multiple changes interact.**
3. **Separate resource ownership from resource health.**
4. **Trust scheduler capabilities according to provenance, not self-report.**
5. **Separate test attempts, verdicts and exonerations.**
6. **Freeze the resolved verification execution plan, including skipped/quarantined coverage.**

These are strong production-proven patterns from Kubernetes, Chromium, Zuul, Zephyr, Android and Yocto and are directly applicable to engineering-platform.
