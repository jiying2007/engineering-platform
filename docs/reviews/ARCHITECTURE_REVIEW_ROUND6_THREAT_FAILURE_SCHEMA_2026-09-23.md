# Architecture Review — Round 6: Threat Model, Failure Model and M0 Schema Closure

Date: 2026-09-23
Reviewed baseline: `docs/architecture/AI_NATIVE_ENGINEERING_PLATFORM_V1_1.md`
Depends on: Round 2–5 reviews
Focus: hostile/compromised components, trust roots, prompt injection, supply-chain substitution, disaster recovery, audit integrity, and schema completeness.

## Result

The architecture remains valid, but this review finds one more P0 class:

> **Trust must be rooted, revocable and independently verifiable; recovery must reconcile durable business state with side effects that may already have escaped the platform.**

The system already models identity, Evidence issuers, immutable manifests, fencing and Human Authority. That is necessary but not sufficient when a trusted issuer is compromised, an artifact store is substituted, a CI workflow is modified, or PostgreSQL is restored to an earlier point after an external release has already happened.

No new top-level Plane is required. The missing semantics belong inside the Control, Evidence and Assurance boundaries.

---

# 1. Threat model: trust zones

Treat these as distinct trust zones:

1. **Experience edge**
   - WorkBuddy
   - Enterprise Connector
   - human browser/client

2. **Control core**
   - Control API
   - PostgreSQL
   - Temporal
   - Policy Engine
   - Identity/Authorization
   - Credential Broker

3. **Interactive data plane**
   - eng CLI
   - Session Gateway
   - Worker control/session channel

4. **Execution zone**
   - Ubuntu Worker
   - execution sandbox
   - Runtime provider
   - local tools/build scripts

5. **Privileged action zone**
   - Platform Action Gateway
   - Git/CI integrations
   - artifact publication
   - device/release operations

6. **Evidence/assurance zone**
   - CI verifier
   - Device Agent
   - verification service
   - review/approval service
   - attestation verifier

7. **External systems**
   - Git provider
   - CI provider
   - object store
   - package registries
   - model/runtime providers
   - OTA/release backend

Do not assume compromise of one zone implies compromise of all others.

---

# 2. P0 — Establish a cryptographic trust-root model

Identity alone is not enough for authoritative Evidence and approvals.

Define a Trust Root / Issuer Registry that records:

- issuer_id
- issuer_type
- trust_domain
- public key / certificate / attestation identity
- allowed evidence/action classes
- environment scope
- project/target scope
- valid_from / valid_until
- status: ACTIVE / SUSPENDED / REVOKED
- key/version identifier
- issuer software/build identity where applicable

Authoritative signed objects should carry:

```yaml
issuer:
  issuer_id: ...
  key_id: ...
signature:
  algorithm: ...
  value: ...
signed_payload_digest: ...
issued_at: ...
```

The verifier evaluates the issuer state and policy at the relevant time.

Do not hard-code one PKI technology into the domain model. X.509, workload identity, Sigstore-style identities, KMS-backed signing or enterprise PKI may implement the same abstract contract.

---

# 3. P0 — Key compromise and revocation must propagate impact

A trusted issuer can later become compromised.

Examples:
- CI signing identity stolen
- Device Agent certificate leaked
- release signing key exposed
- reviewer credential compromised

Revocation must create an impact analysis over all dependent objects.

Example dependency:

```text
Issuer key K
  -> Evidence EV-1/EV-2
  -> Verification VER-7
  -> Review REV-3
  -> Release REL-2
```

Policy decides whether historical signatures remain trusted based on:
- signature time
- key validity interval
- known-compromise time
- timestamp authority where used
- evidence class/risk

Possible outcomes:
- no impact
- future-only block
- verification requires re-run
- release quarantine
- emergency rollback/risk decision

Never simply delete or rewrite historical signed records.

---

# 4. P0 — Tamper-evident audit, not merely append-only database rows

An append-only application API does not prevent a privileged database administrator, compromised database account or backup restore from rewriting history.

Authority-bearing audit should be tamper-evident.

Recommended minimum:

```text
per-aggregate/event sequence
+
event/payload digest
+
hash chaining or immutable journal segment digest
+
periodic signed checkpoint
+
checkpoint copied to independently protected/WORM-capable storage
```

Do not build a blockchain.

Goal:
- detect removed/reordered/modified audit records;
- prove which audit head existed at a given signed checkpoint;
- preserve incident forensics across DB compromise.

M1 may implement simple periodic signed audit checkpoints; higher assurance can add WORM/object lock.

---

# 5. P0 — Control-plane administrator is not automatically Human Authority

Administrative capability and engineering authority must remain separate.

Examples:
- platform admin may enroll/revoke Worker;
- DBA may maintain PostgreSQL;
- security admin may rotate issuer keys;
- release owner may authorize production.

A platform administrator must not be able to manufacture a production approval merely because they administer the service.

Define distinct principal/authority classes and separation-of-duties policy.

Emergency security override is a separately audited Break-Glass Decision, not a hidden admin privilege.

---

# 6. P0 — Prompt injection threat model must assume all repository/context text is hostile

The baseline already labels repository text/logs/webpages as untrusted context. This needs explicit execution consequences.

Assume hostile text may instruct a Runtime to:
- ignore Requirement/Policy;
- exfiltrate secrets;
- modify CI workflow;
- fetch malicious dependencies;
- publish forged Evidence;
- use privileged tools;
- weaken tests;
- add hidden persistence.

Controls must be outside the model:

1. Runtime sandbox blocks unauthorized filesystem/network access.
2. Platform Action Gateway enforces privileged actions.
3. secret-bearing actions never expose reusable credential values.
4. untrusted context cannot change policy/capability grants.
5. formal Verification never trusts Runtime prose.
6. suspicious workflow/policy/verification-definition changes widen review requirements.
7. provider/tool responses are treated as data unless explicitly typed as authority.

Prompt injection is not solved by a better system prompt.

---

# 7. P0 — Generated code/build scripts are part of the hostile execution surface

Even if the Runtime itself is sandboxed, generated or repository-controlled build/test scripts can execute arbitrary code.

Therefore Formal Runs must treat:
- Makefiles/CMake scripts
- package lifecycle hooks
- code generators
- test binaries
- compiler plugins
- shell scripts
- post-build tooling

as untrusted execution inside the Run sandbox.

Privileged credentials must not be inherited into those processes.

Network egress should default-deny or be allowlisted by task/environment policy.

---

# 8. P0 — Dependency and package resolution must be controlled

A source commit alone does not fully identify a build if dependencies resolve dynamically.

Record and/or pin:
- package registry source
- dependency lockfile digest
- package artifact digest where possible
- container/base-image digest
- compiler/SDK dependencies
- model/data dependencies used in build/test
- mirror/proxy identity

Release-quality builds should avoid mutable tags such as `latest`.

When legacy toolchains cannot be hermetic, classify reproducibility and capture the resolved dependency set.

---

# 9. P0 — CI Evidence is trusted only if the verifier definition is trusted

A CI job can be modified to always return PASS.

Therefore CI Evidence must bind not only:
- source commit
- artifact digest
- CI run ID

but also:
- verification workflow/config digest
- verifier software/image digest
- policy/procedure revision
- trusted branch/ref or protected workflow identity
- issuer identity

Policy must reject authoritative Evidence from a verifier definition controlled by the untrusted change being verified unless explicitly allowed.

Example:
- PR changes product code and its own release verification workflow;
- the changed workflow must not automatically certify itself.

Changes to verifier/policy definitions require independent review or a trusted baseline verifier.

---

# 10. P0 — Artifact storage substitution must be detected

Database metadata pointing to `sha256:X` is insufficient if consumers download bytes without re-verifying.

Every Artifact fetch used for:
- build input
- verification
- device flash
- release
- archival export

must verify content digest before use.

Object-store key/path is a locator, not identity.

For multi-part/bundled artifacts, verify the manifest and every referenced content digest.

---

# 11. P0 — Source repository availability/immutability cannot be assumed forever

A Git commit SHA is strong identity while the object remains available, but repositories can be deleted, rewritten, migrated or become inaccessible.

For release/long-term audit classes, preserve:
- repository identity
- commit/tree digest
- required source snapshot/bundle Artifact according to retention policy
- relevant submodule/dependency commit identities

Do not rely exclusively on a future live Git provider to reconstruct a released product.

---

# 12. P0 — Verification freshness and revocation are distinct from subject equality

Evidence may become unusable even when the Subject Manifest is unchanged.

Reasons:
- calibration expired
- test fixture later found defective
- issuer compromised
- CVE/security advisory invalidates assessment
- procedure defect discovered
- field incident contradicts prior assumption

Therefore applicability evaluates:
- subject match
- issuer trust
- procedure validity
- fixture/calibration validity
- policy freshness
- explicit revocation/quarantine
- optional validity window

A byte-identical subject may require re-verification after trust conditions change.

---

# 13. P0 — Recovery after backup restore requires a Recovery Epoch and global reconciliation

Consider:
1. production Release REL-10 is promoted externally;
2. PostgreSQL commits the result;
3. later disaster recovery restores PostgreSQL to a snapshot from before promotion.

The database now says "not promoted" while production says "promoted".

Therefore after authoritative-state restore:

```text
enter RECOVERY / RECONCILIATION mode
  -> increment recovery_epoch
  -> freeze new irreversible operations
  -> reconcile external systems
  -> import/reconstruct confirmed side effects
  -> evaluate missing audit/artifact state
  -> authorize exit from recovery mode
```

All newly dispatched privileged operations carry the current recovery epoch where applicable.

Do not immediately resume normal release/device actions after DB restore.

---

# 14. P0 — Recovery requires authoritative backup/restore semantics per store

Define the role and recovery behavior of:

### PostgreSQL
Business authority; requires backup/PITR and reconciliation after rollback restore.

### Temporal
Orchestration state; recover/restart workflows, but never use it to overwrite PostgreSQL truth.

### Artifact/Object Store
Immutable content; missing release/evidence objects are integrity incidents, not ordinary cache misses.

### Audit checkpoint store
Independent tamper-evidence anchor.

### Credential/Trust store
Issuer/key status and signing capability; recovery must not resurrect revoked credentials.

### Session/transcript store
Operational/debug data; weaker durability acceptable than release evidence.

M0 must classify data by authority and recovery criticality.

---

# 15. P0 — Secret lifecycle needs explicit no-leak invariants

Credential Broker is correct, but define:

- secret never written into Run Input Manifest;
- secret never included in content-addressed Artifact;
- secret values are not structured events;
- transcript/log collection redacts known secret classes;
- sandbox receives ephemeral credential only when unavoidable;
- privileged action proxy is preferred over credential injection;
- credential TTL <= operation/run policy;
- revocation happens on Run termination/takeover where appropriate;
- secret material is zeroized/best-effort removed from temporary files/process environment.

Add automated secret-scanning gates for produced Artifacts/logs before publication when practical.

---

# 16. P0 — Approval replay must be prevented

An approval is a signed/recorded decision over an exact digest, but replay across environment/project/time must also fail.

Approval subject must include or bind:
- organization/project
- environment
- action type
- Release/Subject Manifest digest
- policy bundle digest
- authority scope
- nonce/decision ID
- validity window where applicable

The same production approval cannot be replayed to another environment or a later operation if policy requires one-time authorization.

---

# 17. P0 — Session attach tokens need audience, epoch and single-purpose scope

A short-lived attach token should bind:
- user/actor
- run_id
- execution_epoch
- audience = Session Gateway
- allowed operation: view / interact / takeover-support
- expiry
- one-time/session nonce where useful

A token for RUN-A must not attach to RUN-B.

After takeover, abort or new execution epoch, prior interactive write tokens are invalid.

Read-only observer sessions can have different permissions from steering sessions.

---

# 18. P0 — Runtime provider must be treated as an external trust boundary

Cloud Runtime providers may see context/source depending on policy.

For each provider profile record:
- provider/tenant identity
- deployment/model profile
- data retention/training policy classification
- supported confidentiality classes
- region/data-residency properties
- tool/network behavior
- authentication mode
- version/family identity where observable

Policy determines which context classification may be sent to which provider.

Provider response is untrusted reasoning output, not authority.

---

# 19. P0 — M0 common schema envelope is still missing

Before individual JSON Schemas proliferate, define a common envelope/profile.

Recommended common fields where applicable:

```yaml
schema_name:
schema_version:
canonicalization_version:

id:
organization_id:
project_id:
environment:

created_at:
created_by:

aggregate_version:

content_digest:
  algorithm:
  value:

correlation_id:
causation_id:

labels: {}
extensions: {}
```

Rules:
- which fields are hashed;
- which fields are metadata outside the hash;
- extension namespace/versioning;
- null vs absent semantics;
- max sizes;
- enum evolution policy;
- timestamp precision;
- ID case sensitivity.

Do not independently invent these rules in every schema.

---

# 20. P0 — Typed reference model is required

Avoid generic strings such as `ref: xyz`.

References need explicit type and immutable identity where authority-bearing:

```yaml
ref:
  kind: artifact
  id: ART-812
  content_digest: sha256:...
```

or:

```yaml
source_ref:
  provider: git
  repository_id: ...
  commit: ...
  tree_digest: ...
```

This reduces accidental comparison of business IDs, URLs and content hashes.

---

# 21. P0 — Define deletion/tombstone semantics

Immutable historical/audit records should not silently disappear.

Define:
- retention class
- legal/security deletion exception
- tombstone record
- deletion actor/authority/reason
- dependency impact
- whether content bytes, metadata or both are removed

If evidence bytes must be deleted, the system must preserve that the evidence once existed but is no longer available and Verification may become non-reconstructable.

Never make deletion indistinguishable from "never existed".

---

# 22. P1 — High-value approvals/release records should support independent signatures

For higher-assurance production environments, approval and Release Manifest may be signed using enterprise/KMS-backed keys independent of ordinary session authentication.

This improves:
- non-repudiation
- archival verification
- resistance to database tampering

Do not require this for every M1 developer action.

---

# 23. P1 — Security-sensitive definition changes need special review class

Changes to:
- policy bundles
- verifier workflow
- sandbox profile
- credential broker rules
- issuer registry
- release procedure
- canonicalization/schema code

should be classified as platform-control changes.

Require independent review and stronger CI than ordinary application changes.

The platform should not allow a Runtime to weaken the controls that govern its own Run and then rely on those weakened controls.

---

# 24. P1 — Introduce security incident/quarantine propagation

A security incident may affect a trust domain rather than one Artifact.

Examples:
- compromised Worker pool
- poisoned toolchain image
- compromised package mirror
- malicious verifier workflow

Represent an Incident/Quarantine Decision that can target:
- issuer
- worker class
- toolchain/environment digest
- dependency source
- artifact set
- release set

Run dependency/impact analysis to find affected Verification/Release records.

Incident can create new Work without mutating historical records.

---

# 25. P1 — Failure injection should test Byzantine-looking behavior, not only crashes

M1/M2 tests should inject:
- stale Worker continuing to send writes;
- duplicate callbacks with modified payload;
- artifact bytes not matching declared digest;
- Evidence signed by revoked issuer;
- CI PASS from untrusted verifier revision;
- Session token reused after epoch change;
- external operation timeout after actual success;
- DB restore behind external release state;
- Runtime prompt injection requesting credential exfiltration;
- malicious build script attempting network/host escape;
- policy child attempting to widen parent DENY.

A system that only survives clean process crashes is not sufficient.

---

# 26. Failure model matrix

| Failure / attack | Required behavior |
|---|---|
| Control API restart | no business-state loss; clients retry idempotently |
| PostgreSQL primary failover | aggregate versions/sequences preserved |
| PostgreSQL restore to older point | enter recovery epoch; freeze irreversible operations; reconcile externals |
| Temporal unavailable | business state remains authoritative; workflows resume later |
| Session Gateway restart | CLI reconnects; no change of Run authority |
| Worker disappears | lease expires; new fenced Attempt may start |
| stale Worker reconnects | state-changing callbacks rejected |
| Runtime crashes | resume from Checkpoint + reconcile side effects |
| artifact upload interrupted | artifact remains unfinalized; cannot be verified/released |
| artifact bytes substituted | digest verification fails closed |
| CI issuer revoked | new Evidence rejected; dependent trust impact evaluated |
| verifier workflow modified in PR | cannot self-certify authoritative release evidence |
| Device Agent loses network during flash | operation UNKNOWN; device quarantined/reconciled before reuse |
| production release timeout | UNKNOWN; query/reconcile release backend before retry |
| WorkBuddy unavailable | Formal execution may continue; human-required gates wait |
| policy service unavailable | privileged operation fails closed; local non-privileged work may continue |
| credential broker unavailable | privileged secret-backed action waits/fails closed |
| object store unavailable | no fake Evidence/Artifact completion |
| audit checkpoint store unavailable | policy decides whether high-risk release is blocked; alert emitted |
| issuer key compromised | revoke key; impact analysis; quarantine/reverify as policy requires |
| prompt injection | cannot bypass sandbox/policy/action gateway |
| malicious build script | contained in Run sandbox; no privileged credentials/host access |
| stale approval replay | rejected by subject/environment/action/validity binding |

---

# 27. M0 schema completeness checklist

Before M0 is complete, every authority-bearing schema should answer these questions:

### Identity
- What is the stable business ID?
- What namespace owns it?
- What is the immutable content digest?

### Versioning
- What schema version validates it?
- What canonicalization version hashes it?
- How are old versions verified?

### Actor / issuer
- Who created/issued it?
- Under which authority/trust domain?
- Which credential/key/version was used?

### Time
- Which timestamps are authoritative?
- Is there an expiry/validity window?
- What clock evaluates it?

### Lineage
- Which immutable parents/inputs does it depend on?
- Are references content-addressed/snapshotted?

### Policy
- Which policy bundle was evaluated?
- What environment/resource scope applies?

### Applicability
- What can revoke/quarantine/invalidate its current use?
- Is result separate from applicability?

### Concurrency
- Which aggregate version/epoch guards mutation?
- Which commands are idempotent?

### Recovery
- What happens after partial failure?
- Can the external side effect be reconciled?

### Retention
- How long is metadata/content retained?
- What happens if deletion is required?

If a schema cannot answer the relevant subset, it is not implementation-frozen.

---

# 28. Additional M1 no-go checks

Add to existing criteria:

- authoritative Evidence signature/issuer verification is enforced;
- revoked issuer cannot produce newly trusted Evidence;
- audit mutation/removal is detectable at least from signed checkpoint boundaries;
- platform admin cannot synthesize release approval without release authority;
- untrusted build script cannot access long-lived privileged credentials;
- artifact digest is re-verified at every authoritative consumption boundary;
- CI workflow modified by subject under test cannot automatically self-certify;
- stale attach/steering token fails after execution epoch change;
- approval cannot replay across project/environment/action;
- DB rollback restore enters recovery/reconciliation mode before new irreversible action;
- secret values are absent from manifests/events/published artifacts in acceptance tests;
- old Release audit remains reconstructable if live Git repository is unavailable according to retention policy.

---

# 29. Additional M2 no-go checks

- Device Agent identity/trust is verified;
- flash timeout produces UNKNOWN/quarantine instead of blind retry;
- target state is measured before and after authoritative test;
- fixture/calibration revocation propagates to affected Verification;
- multi-component Release Bundle is re-hashed/verified before deployment;
- update procedure survives injected power-loss points according to declared recovery policy;
- rollback is tested for supported source/target configurations, not merely documented.

---

# 30. Assessment

After six review rounds, the architecture is no longer missing a major functional block.

The remaining P0 work is now almost entirely about **making authority mechanically trustworthy**:

1. root trust in revocable issuers/keys;
2. make audit tampering detectable;
3. make hostile context/code unable to cross privilege boundaries;
4. make build/CI/artifact supply chain content-addressed and verified;
5. make disaster recovery reconcile with external reality;
6. make schemas carry enough identity/version/authority/lineage to preserve trust over years.

The architecture should not grow another plane or agent layer.

The next baseline should incorporate Round 4–6 P0 findings as v1.2, then move to ADR/schema/invariant-test implementation.
