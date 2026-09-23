# M0 Reference Adoption Plan v14

Date: 2026-09-23
Status: **Current execution plan**
Supersedes: docs/roadmap/M0_REFERENCE_ADOPTION_PLAN_V13.md

Parents:
- docs/architecture/AI_NATIVE_ENGINEERING_PLATFORM_V1_2_FINAL.md
- docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V14.md

## 1. Principle

Round 14 adds device trust/provisioning contracts without introducing hardware-security infrastructure into M1.

M0 uses synthetic fixtures only.

---

## 2. Device identity fixture

Create one DeviceInstance linked to one TargetRevision.

Prove:
- two DeviceInstances may share TargetRevision;
- device lifecycle is separate from health/allocation;
- device identity does not rely on mutable hostname/IP.

---

## 3. Provisioning receipt fixture

Create a synthetic DeviceProvisioningReceipt.

Include:
- station identity;
- initial firmware/bootloader digests;
- public certificate/key references;
- calibration reference;
- post-provision check.

Prove:
- no private key material is stored;
- receipt is immutable;
- provisioning authority differs from ordinary flash capability.

---

## 4. DeviceTrustProfile fixture

Freeze a small profile vocabulary:
- DEV_BASIC;
- SIGNED_BOOT;
- MEASURED_BOOT;
- REMOTE_ATTESTED;
- HIGH_ASSURANCE.

Prove policy can:
- require only supported capability;
- reject device whose current trust state is below requirement;
- keep health/allocation checks separate.

---

## 5. Boot/attestation Evidence fixtures

Create synthetic:
- BootTrustEvidence;
- DeviceAttestationEvidence.

Prove:
- exact DeviceInstance and Artifact are bound;
- attestation includes freshness/challenge;
- expired/stale attestation becomes inapplicable;
- secure boot PASS does not satisfy functional Verification.

---

## 6. Rollback fixture

Create:
- current Release;
- old rollback Release;
- anti-rollback/security counter.

Prove:
- functionally compatible but cryptographically inadmissible rollback is rejected;
- Release rollback plan must identify an admissible artifact/bundle.

---

## 7. Existing M0 critical path remains

Still required:
- core schema/canonicalization;
- TraceLink;
- ML/data Artifact reservations;
- ToolProfile/RuntimeQualification;
- Incident/Reproduction/KnowledgeCandidate;
- Run Input / ExecutionSpec / Receipt;
- Build Definition / Receipt;
- Integration Subject / Eligibility;
- Session Grant;
- PostgreSQL/Temporal/OPA;
- Artifact/Evidence/Verification;
- Release Admission/Reconciliation;
- OTel/CDEvents;
- TestReport/BOM/Finding;
- failure injection.

---

## 8. M0 exit additions

M0 additionally verifies:
- Device and Target identity separation;
- provisioning receipt semantics;
- private key no-leak rule;
- attestation freshness;
- trust/health/allocation facets;
- anti-rollback-aware rollback fixture.

No Keylime/TPM/TF-M/FDO/MCUboot deployment is required.

---

## 9. M1 target

M1 remains unchanged.

Round 14 only ensures production/device trust can attach later without redesigning Device/Release authority.
