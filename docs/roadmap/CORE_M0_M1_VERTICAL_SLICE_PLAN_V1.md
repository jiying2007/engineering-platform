# Core M0 / M1 Vertical Slice Plan v1

Date: 2026-09-24
Status: **CURRENT EXECUTION PLAN**

Parents:
- docs/architecture/EMBEDDED_AI_ENGINEERING_PLATFORM_CORE_V1.md
- docs/architecture/EMBEDDED_DOMAIN_CAPABILITY_MODEL_V1.md
- docs/extensions/EXTENSION_CATALOG_V1.md

## 1. Principle

M0/M1 are intentionally narrow.

Do not inherit all historical research schemas.

M0 freezes only contracts required to run one real embedded engineering task safely.
M1 proves one useful end-to-end vertical slice.

---

# M0 — Core Contract Freeze

## 2. Required objects

Freeze only:

~~~text
WorkItem
TaskContract
MaterialManifest
TargetContext
RunInputManifest
Run
RunAttempt
Session
SteeringCommand
Checkpoint
ActionRequest
ActionReceipt
ExternalOperation
ArtifactRef
DeliveryReceipt
EvidenceRef
VerificationPlan
VerificationReport
ReviewReport
ClosureReceipt
HypothesisRegistry
~~~

Cross-cutting:
- typed refs;
- canonical digest;
- audit event;
- execution_epoch;
- recovery_epoch.

Nothing else is a Core M0 blocker.

---

## 3. WorkItem / TaskContract fixture

Create a real-looking embedded Feature task.

Minimum:
- source WorkBuddy/ticket ref;
- owner;
- target;
- exact repo/base;
- Acceptance Criteria;
- capability/skill route;
- allowed actions;
- expected output.

Prove:
- immutable TaskContract digest;
- changed scope => new TaskContract revision.

---

## 4. Material readiness fixture

Cases:
- complete Feature;
- Debug with authoritative logs;
- Debug missing both reproduction/log;
- device task missing exact device/firmware.

Prove:
- missing mandatory material => BLOCKED;
- DEGRADED requires explicit approval.

---

## 5. Run / Attempt / epoch fixture

Simulate:
- Attempt A epoch 1;
- disconnect;
- Attempt B epoch 2;
- late callback from A.

Prove:
- only epoch 2 mutates Run;
- stale callback is rejected;
- restart history preserved.

---

## 6. Session / Steering fixture

Implement:
- Observe;
- structured Steering;
- Pause;
- Resume;
- Checkpoint;
- Human Takeover;
- Abort.

Prove:
- command ordering/idempotency;
- takeover revokes Runtime writes;
- checkpoint is immutable.

---

## 7. Action Gateway fixture

Implement one benign privileged action:
- CI dispatch or Git push mock.

States:
- PLANNED;
- DISPATCHED;
- CONFIRMED;
- UNKNOWN;
- RECONCILING;
- SAFE_TO_RETRY/MANUAL.

Prove:
- lost response => UNKNOWN, not automatic retry.

---

## 8. Delivery / Evidence / Verification fixture

Create:
- DeliveryReceipt from Engineering Run;
- independent EvidenceRef from CI/test;
- VerificationReport.

Prove:
- Runtime claim cannot populate Verification result;
- subject/digest mismatch invalidates Evidence applicability.

---

## 9. Debug Hypothesis fixture

Create:
- 3 hypotheses;
- one supported;
- one rejected;
- one inconclusive.

Prove:
- Observed vs Inferred distinction;
- root cause cannot be confirmed without Evidence.

---

## 10. Recovery fixture

Create:
- signed/chained audit checkpoint;
- synthetic external operation after backup point;
- restore/new recovery_epoch;
- RECOVERY_RECONCILIATION.

Prove:
- old authority rejected;
- external operation reconciled before retry.

---

## 11. M0 exit criteria

M0 exits only when:

- all Core schemas are stable enough for M1;
- canonical digests deterministic;
- epoch fencing tests pass;
- Steering/Takeover tests pass;
- unknown external operation reconciliation passes;
- Runtime claim != Verification enforced;
- Material readiness fails closed;
- recovery_epoch behavior passes;
- initial embedded capability/Skill registry is loadable.

M0 explicitly does **not** require:
- PLM/MES/RMA;
- DPP/GS1;
- PSIRT/CRA;
- privacy platform;
- manufacturing/fleet;
- certification;
- Digital Twin;
- prognostics;
- ASAM/UDS/SOVD;
- Skill marketplace;
- autonomous Planner;
- Knowledge platform.

---

# M1 — First Real Vertical Slice

## 12. Target flow

~~~text
WorkBuddy or CLI Work Item
 -> TaskContract
 -> Material Readiness
 -> explicit embedded Skill route
 -> Run Input Manifest
 -> Ubuntu Worker
 -> Codex interactive Session
 -> Steering
 -> Git change
 -> Build/CI
 -> Artifact / DeliveryReceipt
 -> Evidence
 -> Verification
 -> Closure
 -> result back to WorkBuddy/CLI
~~~

---

## 13. M1 implementation scope

### Control API
Must support:
- create/get WorkItem;
- create frozen TaskContract;
- create/start Run;
- observe Run;
- send Steering;
- pause/resume;
- checkpoint;
- takeover/abort;
- register Delivery/Evidence;
- request Verification;
- close Work.

### CLI
Commands:
- `eng work create`
- `eng work show`
- `eng run start`
- `eng run status`
- `eng run steer`
- `eng run pause`
- `eng run resume`
- `eng run checkpoint`
- `eng run takeover`
- `eng evidence add`
- `eng verify`

### Worker
Must provide:
- registration;
- heartbeat;
- worktree/workspace;
- Session Supervisor;
- Codex adapter;
- epoch fencing;
- event stream;
- checkpoint;
- sandbox execution.

### Embedded domain
Must provide:
- capability registry;
- initial Skill contracts;
- task routing;
- Material Readiness;
- Debug Hypothesis Registry.

### Assurance
Must provide:
- VerificationPlan;
- Evidence applicability;
- VerificationReport;
- optional ReviewReport.

---

## 14. M1 infrastructure

Default:
- Go;
- PostgreSQL;
- Temporal;
- OPA;
- S3/MinIO-compatible store;
- OpenTelemetry;
- Git/CI;
- Ubuntu Worker;
- Codex.

Test-only:
- Toxiproxy;
- disposable PostgreSQL/MinIO where practical.

Do not add infrastructure because a future extension may need it.

---

## 15. M1 real pilot A — Feature

Choose a bounded embedded Feature.

Required:
- real repository;
- full base SHA;
- explicit Acceptance Criteria;
- one or more Skill routes;
- Codex interactive execution;
- at least one Human Steering event;
- real build/CI;
- DeliveryReceipt;
- independent Verification.

Success criteria:
- task reaches Closure;
- no unauthorized action;
- exact source/artifact/evidence chain complete;
- engineer can interrupt/correct without restarting from scratch.

---

## 16. M1 real pilot B — Debug

Choose a real Linux/BSP or MCU issue.

Required:
- MaterialManifest;
- original log/reproduction;
- HypothesisRegistry;
- at least two candidate hypotheses;
- discriminating experiment;
- confirmed/rejected evidence;
- fix or explicit BLOCK;
- regression Verification.

Success criteria:
- platform improves evidence discipline and debugging cycle rather than merely producing prose.

---

## 17. M1 quality metrics

Track:
- Work cycle time;
- time to first useful Evidence;
- Human Steering count;
- restart/recovery success;
- Verification first-pass;
- rework;
- incorrect PASS;
- unauthorized actions;
- Runtime cost;
- engineer feedback.

Do not use:
- token count;
- generated LOC;
- prompt count

as standalone productivity measures.

---

## 18. M1 exit

M1 exits only after:
- Feature pilot passes;
- Debug pilot passes or correctly BLOCKs on missing evidence;
- no incorrect PASS;
- no unauthorized privileged action;
- execution epoch/reconnect demonstrated;
- Human Takeover demonstrated;
- CI/Evidence/Verification provenance exact;
- initial Skills have retained real invocation evidence.

Then proceed to M2 Device/HIL.

---

# M2 — Device/HIL

Primary additions:
- Device Registry;
- lease/fencing;
- DeviceLab backend;
- flash/program action;
- Procedure execution;
- MeasurementResult;
- HIL Evidence;
- diagnostic snapshots.

Activate relevant Tier A extensions only.

---

# M3 — Scale and Product Delivery

Only after M2:
- second Runtime;
- fleet/OTA;
- manufacturing;
- PSIRT/security;
- ML;
- certification;
- broader compatibility;
- product lifecycle adapters.

Exact order follows real product demand.

---

## 19. Anti-scope-creep rule

No M0/M1 requirement may be added because:
- a research document exists;
- a standard is popular;
- a future department may need it;
- a schema would be nice to have.

Addition requires:
- real consumer;
- observed implementation gap;
- bounded pilot;
- explicit exit criterion.

---

## 20. Immediate implementation order

1. repository/code skeleton;
2. core Go types and canonical digest;
3. embedded capability/Skill registry;
4. Material Readiness;
5. Run/Attempt epoch guard;
6. Session Supervisor contract;
7. Action Ledger;
8. Control API + CLI;
9. Codex local adapter;
10. Git/CI pilot;
11. Evidence/Verification;
12. Feature pilot;
13. Debug pilot.

Do not resume broad architecture research before items 1–12 expose a real gap.
