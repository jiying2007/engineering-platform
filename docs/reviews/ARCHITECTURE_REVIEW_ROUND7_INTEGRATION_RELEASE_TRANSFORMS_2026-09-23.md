# Architecture Review — Round 7: Integration Subject, Merge Semantics and Release Artifact Transformation

Date: 2026-09-23
Reviewed baseline: `docs/architecture/AI_NATIVE_ENGINEERING_PLATFORM_V1_1.md`
Depends on: Round 2–6 reviews
Focus: PR/merge correctness, target-branch drift, release candidate identity, signing/packaging transforms, and exact-byte promotion.

## Result

The architecture's preferred rule "build once, verify, promote exact Artifact" is correct as a safety principle, but incomplete for real Git and embedded release workflows.

Two common transitions can change the subject after earlier verification:

1. **source integration**
   - PR head -> merge/rebase/squash/merge-queue commit

2. **release transformation**
   - unsigned firmware -> signed firmware
   - raw binaries -> OTA package
   - binaries -> encrypted/compressed bundle
   - build output -> version-stamped/package metadata output

The platform must explicitly model both. Otherwise it can still verify A and release B while every individual record appears valid.

---

## P0-1 — Introduce an Integration Subject

Development verification against a feature-branch commit is not automatically release verification.

Define an immutable Integration Subject containing:

- target repository identity
- target branch/ref policy
- target base commit
- change/PR head commit(s)
- exact candidate integration commit/tree
- merge strategy
- dependency PR/change set where applicable
- build configuration
- Target Revision / product configuration context

Examples:

```text
feature commit F
+
main commit M
+
merge strategy
=
integration candidate I
```

Release-quality CI/Verification should bind `I`, not merely `F`.

---

## P0-2 — Target-branch drift invalidates old integration assumptions

Scenario:

```text
PR verified against main@M1
main advances to M2
PR merges later
```

The old Verification does not automatically prove the new integrated state.

Policy must define one of:
- require merge-queue/candidate verification against current base;
- rebase/merge and rerun affected verification;
- prove no-impact under an explicit trusted policy.

Do not silently reuse feature-branch Verification after target-base changes.

---

## P0-3 — Squash/rebase/merge commit identity must be explicit

Different merge modes produce different source commits and sometimes different trees.

The platform records:
- development Run result commit/tree
- integration candidate commit/tree
- final merged commit/tree

Tree equality may justify reuse of some Evidence if policy allows, but commit-message/metadata-sensitive build processes can still differ.

Therefore reuse is a formal equivalence Decision, not an implicit assumption.

---

## P0-4 — Merge queue should be a first-class integration mechanism where available

For protected mainline workflows, preferred path:

```text
PR change
 -> integration candidate / merge-group commit
 -> authoritative CI/Verification
 -> merge exact candidate or equivalent proven tree
```

The platform should be provider-neutral, but its Git integration contract should support:
- merge-group/candidate identity
- final merge result identity
- reconciliation if provider creates a different final commit

---

## P0-5 — Multi-PR / multi-repo integration requires dependency identity

A change may rely on another change not yet on main.

Run/Integration Subject should be able to reference:
- prerequisite PR/change IDs
- exact commits
- exact trees
- repository roles

A Verification against "PR A + PR B" does not prove "PR A alone".

Dependency removal/change creates a different subject digest.

---

## P0-6 — Add an Artifact Derivation Graph

Not every release artifact is the direct output of one compiler.

Represent immutable derivation edges:

```text
source
  -> build
unsigned ELF/BIN
  -> signing
signed BIN
  -> packaging
OTA bundle
  -> encryption/compression
distribution package
```

Every derived Artifact records:
- parent Artifact digest(s)
- transform type
- transform implementation/version digest
- configuration
- trusted executor/issuer
- output digest
- transformation attestation/receipt

This preserves exact lineage without pretending all transformations are semantically neutral.

---

## P0-7 — Signing is a privileged transform, not a metadata flag

For firmware/OTA:

```text
unsigned artifact
 -> trusted signing service/HSM/KMS
 -> signed artifact with new bytes/digest
```

The signed bytes are a new Artifact.

Signing policy records:
- signing profile/key identity
- environment
- authorized Release/Subject digest
- signer issuer identity
- timestamp/nonce where relevant

The Runtime never receives the production signing key.

---

## P0-8 — Define verification reuse across transforms

Some Evidence can be reused across controlled transforms; some cannot.

Example:
- source/static-analysis result may remain applicable after signing;
- binary hash obviously changes after signing;
- boot/signature verification must target final signed bytes;
- OTA unpack/install behavior must target final OTA package;
- encryption/compression/package metadata may affect device behavior.

Each transform declares or policy defines affected verification dimensions.

Reuse requires:
- trusted transform receipt;
- exact parent/output digests;
- approved transform class/version;
- no-impact rule for specific Evidence classes.

Otherwise final Artifact requires re-verification.

---

## P0-9 — Final release bytes need final-stage verification

Before production promotion, perform at minimum the verification classes required for the exact final Release Bundle bytes.

Examples:
- digest/signature validation
- package manifest validation
- compatibility validation
- install/unpack validation
- boot/smoke verification for high-risk firmware as policy requires

Do not rely exclusively on tests against a pre-sign/pre-package artifact.

---

## P0-10 — Version stamping/build metadata must be deterministic or explicit transform

Common embedded builds inject:
- semantic version
- Git SHA
- build time
- release channel
- serial/config metadata

If this changes output bytes after Verification, model it as part of the build input or a controlled Artifact transform.

Avoid uncontrolled wall-clock timestamps that prevent reproducibility unless required and explicitly recorded.

---

## P0-11 — Source integration and Artifact promotion are separate state machines

Do not overload "Release" with Git merge status.

Suggested independent projections:

### Integration

```text
PROPOSED
-> CANDIDATE_CREATED
-> VERIFIED
-> MERGED
-> RECONCILED
```

### Release

```text
CANDIDATE
-> FINAL_ARTIFACTS_READY
-> VERIFIED
-> REVIEWED
-> AUTHORIZED
-> PROMOTED
-> CONFIRMED
```

A Work may merge source but not be production-released.

A hotfix Release may promote from an already integrated source revision.

---

## P0-12 — Merge success is an external side effect and must be reconciled

PR merge is not an in-database state transition.

Apply the same external-operation rules:

```text
MERGE_DISPATCHED
 -> CONFIRMED

or
 -> UNKNOWN
 -> RECONCILING
 -> CONFIRMED / SAFE_TO_RETRY / MANUAL
```

After confirmation, fetch the actual provider-created final commit/tree and compare with expected Integration Subject.

Mismatch creates a new subject and blocks automatic evidence reuse.

---

## P0-13 — Release promotion must verify remote bytes/state

Copying/promoting an Artifact into a release/OTA backend is also an external side effect.

After promotion:
- query backend;
- confirm exact artifact/bundle digest or equivalent signed identity;
- confirm target/environment/channel;
- record promotion receipt.

"HTTP 200" is not sufficient proof that production now contains the intended bytes.

---

## P0-14 — Rollback artifact/configuration must be pre-identified

A Release Manifest saying "rollback supported" should bind the exact rollback target:

- previous Release Manifest digest
- previous Bundle digest
- supported rollback procedure revision
- compatibility/data-migration constraints

Do not discover the rollback artifact only after a failed production promotion.

---

## P1-1 — Preserve integration/release candidate evidence separately from development evidence

Useful read model:

```text
Development Evidence
Integration Evidence
Release-Final Evidence
Field Evidence
```

All may reuse common raw data/provenance, but users should be able to see which stage actually established release authority.

---

## P1-2 — Artifact transform services have trust classes

Examples:
- BUILD
- SIGN
- PACKAGE
- ENCRYPT
- PROMOTE

A transform service may be allowed to sign but not build; a Worker may build but not production-sign.

Trust/capability policy binds transform class to issuer.

---

## P1-3 — Release candidate identity should survive storage movement

Release identity must use content digests/manifests, not:
- object-store path
- CDN URL
- OTA channel name
- filename

Those are mutable locators/distribution metadata.

---

## P1-4 — Mainline verification status is a projection, not permanent truth

A main commit may later lose release eligibility because:
- new CVE/security policy
- issuer revocation
- target compatibility update
- fixture/procedure invalidation
- field incident

Keep source integration history immutable, but calculate current release eligibility from current trust/policy/applicability.

---

## Required M0 additions

Add:

1. Integration Subject Manifest schema.
2. Integration state/operation schema.
3. Git provider reconciliation contract.
4. Artifact Derivation/Transform Receipt schema.
5. signing transform policy schema.
6. Evidence-reuse-across-transform rules.
7. final-release verification policy.
8. promotion receipt schema.
9. rollback target binding schema.

---

## Additional M1 no-go checks

Even before production signing is implemented, M1 Git/CI flow should prove:

- CI Evidence binds an exact integration candidate/tree;
- base branch advance makes old integration evidence non-authoritative unless explicit no-impact policy applies;
- merge result commit/tree is reconciled after provider merge;
- duplicate/timeout merge operation does not create ambiguous state;
- source result/PR head/final merge commit are not conflated.

---

## Additional M2/release no-go checks

- final signed/packaged Artifact has a distinct digest and derivation lineage;
- production key never enters Runtime/Worker sandbox;
- final Release Bundle validation runs against exact distributed bytes;
- transform receipt is issued by an authorized transform service;
- promotion backend is reconciled to exact intended bundle;
- rollback target/procedure is known before production authorization;
- evidence reuse after signing/packaging is explicit and policy-driven.

---

## Assessment

A safe engineering platform needs two exactness guarantees:

1. **source exactness** — what was tested is the exact integrated source state that actually entered the protected branch;
2. **artifact exactness** — what was released is the exact final transformed byte set whose lineage and required final-stage Verification are known.

The phrase "build once, verify, promote" should therefore be refined to:

> **freeze the exact integration subject; build immutable artifacts; apply only trusted, recorded transforms; verify every transform-sensitive property against the final bytes; promote and reconcile the exact Release Bundle.**
