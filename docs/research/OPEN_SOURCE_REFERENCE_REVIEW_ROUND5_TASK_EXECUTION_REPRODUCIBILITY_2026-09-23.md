# Open Source Reference Review — Round 5

Date: 2026-09-23
Status: **Archived research / implementation input**
Parent:
- docs/architecture/AI_NATIVE_ENGINEERING_PLATFORM_V1_2_FINAL.md
- docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V5.md

## 1. Scope

Round 5 continued with engineering systems used for large CI/release organizations:

- Mozilla Taskcluster;
- Buildbot;
- Gerrit;
- Nix;
- OpenBMC test automation;
- Robot Framework.

The focus was:
- worker queue/claim semantics;
- worker fleet lifecycle;
- submission gates and approval applicability;
- build definition/reproducibility;
- embedded failure diagnostics.

---

## 2. Taskcluster Queue — claim/reclaim rather than push

Repository:
- https://github.com/taskcluster/taskcluster

Taskcluster Queue separates:
- pending tasks;
- claimed tasks;
- deadlines;
- dependency resolution.

Workers pull work with claimWork.
Long-running work renews/reclaims its claim.
Expired claims can be rescheduled.

A particularly strong practice is atomic scheduling:
the task run transition to pending and insertion into the claimable pending queue commit in the same database operation, avoiding "state says pending but no worker can claim it".

### Absorb

Future multi-Worker engineering-platform should prefer:

~~~text
Scheduler
 -> durable execution-ready record
 -> Worker pulls/claims
 -> execution lease
 -> periodic renew/heartbeat
 -> completion / failure / expiry
~~~

rather than Control Plane opening inbound execution connections to Workers.

This fits the current outbound Worker channel design.

### M1

One Worker may be directly assigned.

Do not build a generalized distributed task queue.

### Scale trigger

When there are many Workers:
- add pull/claim scheduler semantics;
- lease/claim is fenced by execution_epoch;
- state + claimability update must be transactional.

Temporal continues to own workflow orchestration.
The Worker queue owns only execution placement/claiming.

---

## 3. Worker Manager — capacity supply separated from work queue

Taskcluster Worker Manager separately:
- observes queue capacity demand;
- provisions workers;
- scans worker state;
- deprovisions workers;
- tracks provider-specific lifecycle.

Worker states are distinct from task states.

### Absorb

Future scale architecture should separate:

~~~text
Run Scheduler
  chooses eligible capacity

Worker Fleet Manager
  creates/removes capacity

Worker
  claims and executes work
~~~

Do not combine autoscaling/provisioning with Run workflow logic.

For fixed on-prem Ubuntu hosts, Worker Fleet Manager can remain a registry-only module.

---

## 4. Provider-attested Worker metadata

Taskcluster validates cloud-provider attested metadata for worker provisioning.

This reinforces Profile v5 SchedulerAttribute provenance.

### Absorb

When an infrastructure provider can attest:
- instance identity;
- host identity;
- hardware/security properties;

store the attribute source as ENROLLMENT_ATTESTED instead of WORKER_REPORTED.

This can later map to:
- cloud instance identity;
- TPM/device certificate;
- enterprise host inventory.

---

## 5. Versioned execution payload

Taskcluster Generic Worker validates task payloads against a JSON schema baked into the Worker version.

### Absorb

Add an immutable/versioned:

~~~text
ExecutionSpec
~~~

for Worker execution.

It references:
- Run ID / Attempt;
- execution_epoch;
- Run Input Manifest digest;
- RuntimeProvider profile;
- Workspace/Execution profile;
- resource limits;
- capability grant reference;
- expected output/report contract.

Worker advertises supported ExecutionSpec schema/protocol versions.

Unsupported/malformed spec fails before execution.

This is distinct from Run Input Manifest:
- Run Input Manifest = formal engineering input lineage;
- ExecutionSpec = concrete Worker execution instructions.

---

## 6. Gerrit Submit Requirements — gate policy is explicit and testable

Repository:
- https://github.com/GerritCodeReview/gerrit

Gerrit Submit Requirements:
- define when a change is submittable;
- support applicability conditions;
- are testable before deployment;
- support inheritance;
- can prevent child projects from overriding parent requirements;
- support non-uploader/non-author approvals;
- separate trigger votes from submit gates.

### Absorb

This reinforces:
- Authorization/Assurance policy inheritance;
- non-self-approval;
- policy testing before rollout;
- Integration Eligibility as an explicit projection.

Use OPA policy tests to reproduce these guarantees in engineering-platform.

---

## 7. Explicit approval carry-forward

Gerrit can copy an approval to a new patch set only when a configured copy condition says it remains applicable, e.g. for trivial/non-code changes.

Our current default is stricter: approval binds an exact digest.

### Optimization

Keep strict default:

~~~text
new subject digest
 -> old approval is not applicable
~~~

But support an explicit:

~~~text
ApprovalCarryForwardDecision
~~~

when policy proves a classified change is safe to carry.

Fields:
- source approval;
- old/new subject digest;
- change classification;
- policy rule;
- authorizing actor/service;
- rationale.

Never silently copy approval.

This reduces needless human reapproval without weakening audit.

---

## 8. Nix derivations — build definition before build receipt

Repository:
- https://github.com/NixOS/nix

Nix derivations describe:
- exact inputs;
- dependent derivations;
- system;
- builder;
- args;
- environment;
- expected outputs.

The build then realizes those declared outputs.

### Major optimization

Split current Build model into:

~~~text
Build Definition Manifest
 -> Build Execution
 -> Build Receipt
 -> Artifact
~~~

### Build Definition Manifest

Immutable/content-addressed:
- source/input digests;
- dependencies;
- builder/toolchain;
- arguments;
- relevant environment;
- build config;
- expected output roles;
- reproducibility profile.

### Build Receipt

Records actual:
- executor;
- timing/resources;
- cache use;
- logs;
- resolved environment;
- output digests;
- deviations.

This is stronger than recording build inputs only after execution.

M1 can generate a basic Build Definition even for legacy vendor builds.

---

## 9. Build determinism is a declared property

Nix emphasizes precisely defined inputs and deterministic outputs.

engineering-platform should not claim reproducibility merely because inputs were recorded.

Build Definition declares expected class:

~~~text
REPRODUCIBLE
CONTROLLED
RECORDED_LEGACY
UNKNOWN
~~~

Build/qualification policy may optionally perform repeat builds and compare output digests for stronger classes.

---

## 10. OpenBMC test automation — FFDC on failure

Repository:
- https://github.com/openbmc/openbmc-test-automation

OpenBMC test automation includes:
- power/reboot/update;
- Redfish/IPMI/SSH;
- error injection;
- SOL collection;
- FFDC collection;
- hardware/system suites;
- Robot Framework-based test organization.

### Strong embedded practice

Failure evidence should be captured automatically at the point of failure.

Add to Procedure Revision:

~~~text
failure_capture:
  triggers
  collectors[]
  timeout
  max_bytes
~~~

Potential collectors:
- serial tail;
- dmesg;
- application logs;
- MCU diagnostic registers;
- power/current trace;
- device state/version;
- coredump;
- test fixture state.

Output:

~~~text
Failure Diagnostic Bundle
~~~

as immutable Raw Evidence Artifact(s).

This is especially valuable for intermittent HIL/device failures.

---

## 11. Robot Framework — readable procedure layer

Repository:
- https://github.com/robotframework/robotframework

Robot Framework provides:
- keyword-driven acceptance tests;
- extensible libraries;
- result combination/post-processing.

### Assessment

Useful for human-readable system/acceptance procedures.

Do not make Robot Framework the canonical Procedure model.

Procedure Revision can reference:
- pytest package;
- OpenHTF recipe;
- Robot suite;
- project-specific executable;

through a generic executable/test-package reference.

The Result Sink normalizes outcomes.

---

## 12. Buildbot — master/worker remains a useful baseline

Buildbot confirms mature CI separation:
- master/control;
- workers;
- heterogeneous platform execution;
- metrics/status reporting.

No new design change is needed beyond existing Control Plane / Worker separation.

---

## 13. Consolidated lessons

1. **Workflow orchestration and execution claiming are separate problems.**
2. **Worker fleet capacity management is separate from task assignment.**
3. **Worker attributes can be infrastructure-attested.**
4. **Worker execution instructions deserve a versioned ExecutionSpec.**
5. **Approval carry-forward must be explicit and policy-backed.**
6. **Build definition exists before execution; Build Receipt exists after.**
7. **Failure diagnostics should be automatically captured as Raw Evidence.**

---

## 14. Milestone impact

### M0/M1
Add:
- ExecutionSpec schema;
- Build Definition Manifest schema;
- ApprovalCarryForwardDecision schema.

M1 implements:
- one Worker;
- direct execution assignment;
- no distributed claim queue.

### M2
Add:
- Procedure failure-capture contract;
- Failure Diagnostic Bundle.

### Scale/M3+
Add if required:
- pull/claim Worker scheduling;
- Worker Fleet Manager;
- provider-attested scheduler attributes.

---

## 15. Conclusion

Round 5 further separates intent from execution and execution from evidence:

~~~text
Run Input Manifest
  -> ExecutionSpec
  -> Worker execution

Build Definition Manifest
  -> Build execution
  -> Build Receipt
  -> Artifact

Procedure
  -> test attempt
  -> automatic FFDC on failure
  -> Raw Evidence
~~~

This makes formal engineering behavior easier to reproduce, audit and recover without requiring additional M1 infrastructure.
