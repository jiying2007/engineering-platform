# Reference-Aligned Implementation Profile v6

Date: 2026-09-23
Status: **Canonical implementation companion to architecture v1.2**
Supersedes: docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V5.md

Research basis:
- Open Source Reference Review Rounds 1–5
- Open Source Reference Synthesis Optimization
- Architecture v1.2

## 1. Goal

Profile v6 adds execution-specification, build-definition and diagnostic-evidence semantics discovered from Taskcluster, Gerrit, Nix and OpenBMC.

M1 dependencies remain small.

---

## 2. M1 stack remains unchanged

Required:
- PostgreSQL
- Temporal
- OPA
- S3/MinIO-compatible ArtifactStore
- OpenTelemetry
- Toxiproxy
- Git/CI
- Codex
- native Ubuntu execution

No distributed worker queue, Nix, Taskcluster, Robot Framework or OpenBMC framework is introduced.

---

## 3. Run Input Manifest vs ExecutionSpec

These are now explicitly separate.

### Run Input Manifest

Formal engineering lineage:
- Requirement/Task/Target;
- source trees;
- selected context;
- policy/capability references;
- parent/checkpoint lineage.

### ExecutionSpec

Concrete instructions for one Worker Attempt:
- schema/version;
- Run ID;
- Attempt ID;
- execution_epoch;
- Run Input Manifest digest;
- RuntimeProvider profile;
- Workspace/Execution profile;
- resource limits;
- network profile;
- capability grant reference;
- output/report contract;
- timeout/cancellation settings.

ExecutionSpec is immutable/content-addressed.

Worker validates supported schema/protocol before starting.

---

## 4. Worker execution compatibility

Worker advertises:
- protocol range;
- ExecutionSpec schema versions;
- Runtime adapter versions;
- sandbox features;
- supported output/report contracts.

Control Plane must not dispatch an unsupported ExecutionSpec.

Malformed or unsupported execution instructions fail before workspace mutation.

---

## 5. Scaled Worker scheduling remains deferred

M1:
- one/few Workers;
- direct assignment through current control channel.

Future scale:
- pull/claim execution queue;
- claim lease;
- heartbeat/reclaim;
- execution_epoch fencing;
- transactional state + claimability;
- Worker Fleet Manager separate from scheduler.

Temporal continues to own workflow orchestration.

The execution queue only solves placement/claiming.

---

## 6. Build Definition Manifest

Build intent is frozen before execution.

Add immutable/content-addressed:

~~~text
BuildDefinitionManifest
~~~

Includes:
- source/input digests;
- dependency graph/reference;
- builder/toolchain;
- arguments;
- build config;
- relevant environment inputs;
- workspace/environment profile;
- expected output roles;
- declared reproducibility class.

This is the build equivalent of Run Input Manifest.

---

## 7. Build Receipt

After execution, Build Receipt records reality:

- Build Definition digest;
- executor/Worker;
- actual environment;
- start/end/duration;
- cache information;
- logs;
- resource usage;
- actual output Artifact digests;
- deviations/warnings;
- tool versions discovered at runtime.

Rule:

> Build Definition says what should happen; Build Receipt says what did happen.

Artifact provenance links both.

---

## 8. Reproducibility is declared and optionally proven

Classes remain:
- REPRODUCIBLE;
- CONTROLLED;
- RECORDED_LEGACY;
- UNKNOWN.

Higher-assurance policy may require:
- repeat build;
- independent builder;
- output digest comparison.

Merely recording inputs does not prove reproducibility.

---

## 9. Approval carry-forward is explicit only

Default:

~~~text
subject digest changes
 -> approval no longer applicable
~~~

Optional authorized:

~~~text
ApprovalCarryForwardDecision
~~~

Requires:
- source approval ID;
- old/new subject digests;
- classified change;
- policy rule allowing carry-forward;
- rationale;
- decision actor/service.

This can reduce reapproval for proven trivial/no-impact changes without silent sticky approvals.

---

## 10. Automatic failure diagnostics

M2 Procedure Revision gains:

~~~text
failure_capture:
  triggers[]
  collectors[]
  timeout
  max_bytes
~~~

Potential collectors:
- serial/log tail;
- dmesg/kernel log;
- MCU fault/status registers;
- application trace;
- power/current/temperature trace;
- loaded firmware/component identities;
- fixture state;
- core dump;
- network state.

Failure produces immutable:

~~~text
FailureDiagnosticBundle
~~~

linked to the exact TestAttemptResult.

It is Raw Evidence, not a verdict.

---

## 11. Procedure implementation is framework-neutral

Procedure Revision may reference:
- pytest;
- pytest-embedded;
- OpenHTF-style recipe;
- Robot Framework suite;
- project-specific executable.

The platform owns:
- Procedure identity/revision;
- required resources;
- inputs;
- expected structured result contract;
- failure-capture contract;
- Evidence semantics.

The test framework owns test syntax/execution convenience only.

---

## 12. Worker/provider attestation

SchedulerAttribute provenance from v5 remains.

At scale/provider-backed workers, infrastructure-attested instance metadata may become:
- ENROLLMENT_ATTESTED;
- trusted Worker identity input.

Worker self-report cannot create privileged attributes.

---

## 13. Final M1 additions from v6

M1 now must produce:

~~~text
Run Input Manifest
 -> ExecutionSpec
 -> Run Attempt
 -> Run Receipt
~~~

and:

~~~text
Build Definition Manifest
 -> Build execution
 -> Build Receipt
 -> Artifact
~~~

This strengthens replayability without adding new infrastructure.

---

## 14. M2 additions from v6

M2 adds:
- automatic FFDC/Failure Diagnostic Bundle;
- procedure framework adapters only as needed;
- Device/resource scheduler semantics from v5.

---

## 15. Future scale additions

Only when needed:
- pull/claim Worker queue;
- Worker Fleet Manager;
- autoscaling;
- cloud/provider attestation;
- task/result-history service.

Taskcluster/Buildbot remain scale references, not required dependencies.

---

## 16. Final rules added by v6

1. **Formal engineering input and concrete Worker execution instructions are different immutable objects.**
2. **Build intent is frozen before build execution.**
3. **Build Receipt records actual execution and deviations.**
4. **Worker rejects unsupported/malformed execution specs before mutation.**
5. **Approval never carries to a new digest without an explicit policy-backed decision.**
6. **Device/HIL failures automatically capture diagnostic Raw Evidence.**
7. **Test framework syntax never becomes engineering authority.**

Architecture v1.2 remains canonical.
Profile v6 is the current implementation companion.
