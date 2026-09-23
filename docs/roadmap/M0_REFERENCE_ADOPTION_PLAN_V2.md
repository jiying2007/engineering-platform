# M0 Reference Adoption Plan v2

Date: 2026-09-23
Status: **Execution checklist / current**
Supersedes: docs/roadmap/M0_REFERENCE_ADOPTION_PLAN.md

Parents:
- docs/architecture/AI_NATIVE_ENGINEERING_PLATFORM_V1_2_FINAL.md
- docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V2.md

## 1. Exit principle

M0 does not end when documents are written.

M0 ends when the selected boundaries have:
- ADR;
- schema/protocol;
- spike evidence;
- failure behavior;
- replacement strategy;
- executable invariant tests.

---

## 2. Track A — Workload identity

### Spike A1 — SPIFFE/SPIRE

Decision target: ADOPT or REFERENCE-ONLY.

Prototype identities for:
- Control API;
- Session Gateway;
- one Ubuntu Worker;
- Platform Action Gateway.

Validate:
- workload attestation;
- short-lived SVID;
- mTLS;
- rotation;
- revocation;
- Worker reconnect after cert rotation.

Go if:
- identity lifecycle is simpler/safer than custom worker certificates;
- Linux host deployment overhead is acceptable;
- trust-domain mapping is clear.

No-Go if:
- operational cost is disproportionate for the deployment model.

Deliver:
- WorkloadIdentityBackend ADR;
- trust-domain naming convention;
- Worker identity integration test.

---

## 3. Track B — Authorization and assurance policy

### Spike B1 — OPA vs Cedar

Decision target: choose AuthorizationEngine implementation.

Common scenarios:
- actor may steer Run;
- actor may takeover;
- Worker may request Git push;
- Device Agent may flash dev target;
- Release Owner may authorize production;
- child project policy attempts to widen parent deny.

Evaluate:
- Go integration;
- policy schema/type safety;
- test tooling;
- explainability;
- bundle/versioning;
- static analysis;
- latency;
- fail-closed behavior.

Keep:
- AssurancePolicyEngine as separate domain contract.

OpenFGA is deferred unless relationship complexity proves need.

Deliver:
- AuthorizationEngine ADR;
- policy examples/tests;
- policy bundle canonicalization rules.

---

## 4. Track C — Secret lifecycle

### Spike C1 — OpenBao

Decision target: ADOPT or compatible SecretBackend.

Prototype:
- Git credential lease;
- short-lived CI credential;
- automatic expiry/revocation;
- Run abort/takeover revocation.

Validate:
- no reusable secret in workspace;
- auditability;
- credential lease tied to Run/action;
- broker outage fails privileged actions closed.

Deliver:
- SecretBackend ADR;
- Credential Broker integration;
- secret no-leak tests.

---

## 5. Track D — Durable workflow

### Spike D1 — Temporal

Decision target: ADOPT.

Validate:
- human wait;
- Worker wait/reconnect;
- Activity heartbeat;
- cancellation;
- retry;
- Continue-As-New;
- workflow versioning;
- PostgreSQL outbox boundary.

Deliver:
- workflow conventions;
- Work/Run sample workflow;
- replay/versioning test;
- duplicate side-effect test.

---

## 6. Track E — Runtime and Session

### Spike E1 — OpenHands / SWE-ReX / Cline comparison

Decision target: reference/selective reuse.

Prototype:
- RuntimeBackend;
- ExecutionBackend;
- WebSocket attach;
- local PTY;
- remote shell backend;
- reconnect;
- structured Steering;
- long-running process observation;
- checkpoint.

Validate:
- provider session never becomes Run authority;
- execution_epoch enforced;
- Human Takeover revokes runtime write;
- headless and interactive modes share one core contract.

### Spike E2 — Runtime capability profiles

Implement:
- PLAN;
- BUILD;
- DEBUG;
- REVIEW.

Validate permissions from platform, not agent UI mode.

### Spike E3 — RepositoryMap

Create a non-authoritative, tree-digest-bound context artifact.

Validate:
- reproducible enough for same tree/generator;
- stale map rejected when tree changes;
- no policy/authority data derived from summary.

---

## 7. Track F — Attestation and trust

### Spike F1 — in-toto/SLSA projection

Produce:
- build provenance;
- test Evidence;
- transform receipt.

Validate:
- exact subject digests;
- platform-native fields preserved or referenced;
- offline verification.

### Spike F2 — Sigstore/cosign

Validate:
- digest-based signing;
- KMS/PKI/keyless option;
- trust-root update;
- revoked/invalid identity rejection.

### Spike F3 — Attestation Controller

Pattern:
- observe completed execution;
- reload authoritative result;
- snapshot;
- canonicalize;
- sign;
- publish Evidence.

Must prove:
- build/test step cannot self-issue trusted Evidence;
- subject mismatch fails closed.

Deliver:
- AttestationBackend ADR;
- Attestation Controller protocol;
- issuer/trust schemas.

---

## 8. Track G — Build

### Spike G1 — Build Receipt v2

Freeze fields:
- source/input digests;
- build definition;
- flags/config;
- relevant environment;
- toolchain/image;
- dependency set;
- executor;
- logs;
- timing;
- output digest;
- cache/provenance.

### Spike G2 — Controlled BuildBackend

Compare:
- Dagger;
- BuildKit-style execution.

Use a container-friendly repository.

### Spike G3 — Remote build semantics

Do not deploy remote execution yet.

Model compatibility with:
- CAS;
- action digest;
- result cache;
- log stream.

Deliver future RemoteBuildBackend contract only.

---

## 9. Track H — Device/HIL resource control

### Spike H1 — labgrid

Representative motor/HIL lane:
- remote resource;
- serial;
- power/reset;
- flash;
- measurement/log source.

Must prove:
- engineering lease/fencing is authoritative;
- stale lease cannot operate device;
- exact Artifact/Target/Procedure identities reach raw Evidence;
- device loss creates quarantine/reconciliation.

Deliver:
- DeviceLabBackend ADR;
- LabgridAdapter.

---

## 10. Track I — Test procedure execution

### Spike I1 — pytest/OpenHTF comparison

Model one Procedure with:
- phases;
- serial interaction;
- measurement;
- threshold/spec;
- raw attachment;
- teardown.

Compare:
- plain pytest;
- pytest-embedded;
- OpenHTF concepts.

Decision target:
- choose Procedure execution style without coupling engineering authority to test framework.

Must prove:
- raw measurement survives evaluation rule changes;
- Procedure Revision is platform-owned;
- test framework cannot self-authorize Verification.

Deliver:
- TestProcedureBackend ADR;
- Procedure/Phase/Measurement/Attachment schemas.

---

## 11. Track J — Supply-chain intelligence

### Spike J1 — Syft

Generate SBOM Artifact for:
- one Linux/rootfs/container-style artifact;
- one relevant source/filesystem case.

### Spike J2 — OSV-Scanner

Generate vulnerability Evidence and bind it to exact SBOM/Artifact.

### Spike J3 — GUAC feasibility

Optional.

Ingest:
- SBOM;
- SLSA/in-toto;
- OSV result.

Query:
- vulnerability -> package -> artifact -> release.

Decision:
- adopt only as derived/read model if useful.

Never make GUAC core authority.

---

## 12. Track K — Failure injection

### Spike K1 — Toxiproxy

Inject:
- Session Gateway latency/drop;
- Worker reconnect;
- Temporal/API network interruption;
- object store timeout;
- mock Git/CI timeout;
- release backend timeout after simulated success.

Must validate UNKNOWN/reconciliation semantics.

### Future
Use Chaos Mesh only if deployment becomes Kubernetes-heavy.

Deliver:
- NetworkFaultBackend test harness;
- automated failure matrix.

---

## 13. Track L — Workspace environment

### Spike L1 — Dev Containers compatibility

For one modern repository:
- parse/use devcontainer metadata;
- record environment definition digest;
- reuse in local/CI where practical.

Keep NativeProfile for embedded legacy SDK.

Deliver:
- WorkspaceEnvironmentSpec schema.

---

## 14. Track M — Embedded release/update

### Spike M1 — RAUC
Linux OTA candidate.

### Spike M2 — MCUboot feasibility
Assess one MCU family without forcing migration.

### Spike M3 — Uptane practice review
Map high-assurance key/metadata role separation to Release security profile.

### Future
- SWUpdate;
- Mender;
- hawkBit.

Deliver:
- ReleaseBackend taxonomy;
- update security profile;
- fit matrix by target type.

---

## 15. Dependency admission checklist

A hard dependency needs:
- active maintenance;
- acceptable license;
- security/update process;
- pinned compatibility range;
- adapter boundary;
- observability;
- degraded/failure behavior;
- upgrade test;
- migration/exit path;
- no hidden engineering authority.

---

## 16. M0 exit criteria

M0 is complete when:

- WorkloadIdentityBackend decision made;
- AuthorizationEngine decision made;
- SecretBackend decision made;
- Temporal conventions implemented;
- Runtime/Execution/Session contracts frozen;
- Attestation Controller + standard projection frozen;
- Build Receipt/Backend contracts frozen;
- DeviceLabBackend decision made from real lab spike;
- TestProcedureBackend frozen;
- SBOM/Vulnerability evidence path demonstrated;
- deterministic failure injection runs in CI;
- ReleaseBackend taxonomy frozen;
- every hard dependency has adapter/failure/exit semantics.

Only then proceed to the main M1 implementation branch.
