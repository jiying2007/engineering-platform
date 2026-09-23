# Reference-Aligned Implementation Profile v14

Date: 2026-09-23
Status: **Canonical implementation companion to architecture v1.2**
Supersedes: docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V13.md

Research basis:
- Open Source Reference Review Rounds 1–14
- Open Source Reference Synthesis Optimization
- Architecture v1.2

## 1. Goal

Profile v14 adds production-device identity, provisioning and attestation semantics without forcing a single hardware trust mechanism onto all products.

---

## 2. DeviceInstance

Physical device identity is separate from TargetRevision.

~~~text
DeviceInstance
~~~

May include:
- device ID;
- serial/lot/batch;
- hardware identity;
- board revisions;
- manufacturing metadata;
- trust anchor refs;
- ownership/enrollment;
- lifecycle.

TargetRevision remains the configuration class.

---

## 3. DeviceProvisioningReceipt

Immutable:

~~~text
DeviceProvisioningReceipt
~~~

Records:
- DeviceInstance;
- provisioning station;
- time;
- observed hardware identity;
- public credential/certificate refs;
- boot-trust configuration;
- initial bootloader/firmware digests;
- calibration refs;
- ownership/onboarding refs;
- operator/service authority;
- post-provision verification.

Never stores reusable private keys.

---

## 4. Provisioning authority

Provisioning may burn/modify:
- OTP/eFuse;
- boot key hash;
- debug lock;
- anti-rollback counter;
- secure storage;
- device certificate.

Treat as high-impact privileged operation:

~~~text
ProvisioningPlan
 -> authorized trusted station
 -> operation
 -> ProvisioningReceipt
 -> post-check
~~~

Ordinary Worker/Runtime flash access does not imply provisioning capability.

---

## 5. Device lifecycle

Suggested:

~~~text
UNPROVISIONED
 -> PROVISIONING
 -> ENROLLED
 -> ACTIVE
 -> QUARANTINED
 -> RMA / RETIRED
~~~

Allocation and health remain separate facets.

---

## 6. DeviceTrustProfile

Versioned:

~~~text
DeviceTrustProfile
~~~

Possible levels/capabilities:
- DEV_BASIC;
- SIGNED_BOOT;
- MEASURED_BOOT;
- REMOTE_ATTESTED;
- HIGH_ASSURANCE.

Policy may require:
- signed image;
- anti-rollback;
- hardware root;
- device certificate;
- measured boot;
- remote attestation;
- debug lock/state.

Target capability determines which mechanisms are possible.

---

## 7. BootTrustEvidence

For secure-boot-capable targets:

~~~text
BootTrustEvidence
~~~

Binds:
- DeviceInstance;
- bootloader/version;
- boot policy;
- signing key/cert identity;
- verified image digest;
- rollback/security counter;
- boot result;
- verifier/issuer.

Secure boot proves image acceptance/authenticity under boot policy, not functional correctness.

---

## 8. DeviceAttestationEvidence

For attestation-capable targets:

~~~text
DeviceAttestationEvidence
~~~

Binds:
- DeviceInstance;
- challenge/nonce;
- attestation mechanism/profile;
- measurement/claim values;
- verifier;
- trust root/key refs;
- timestamp/freshness;
- raw attestation;
- result.

Applicability is time/context sensitive.

---

## 9. Device trust state

Separate:

~~~text
trust_state:
  UNKNOWN
  PROVISIONED
  VERIFIED
  ATTESTED
  DEGRADED
  REVOKED
~~~

from:

~~~text
health_state:
  READY
  DIRTY
  MAINTENANCE
  QUARANTINED
  OFFLINE
~~~

and allocation state.

Policy/scheduling may require combinations of all three facets.

---

## 10. Device credentials

Control Plane stores:
- public identity;
- issuer;
- credential ID;
- generation;
- validity/status;
- rotation/revocation refs.

Private key stays in:
- secure element/TPM;
- device secure storage;
- HSM;
- trusted provisioning backend.

---

## 11. Ownership

Reserve:

~~~text
DeviceOwnershipRecord
OwnershipTransferReceipt
~~~

for:
- manufacturer to fleet owner;
- RMA;
- tenant transfer;
- lab/production ownership changes.

FIDO Device Onboard is a future implementation candidate, not mandatory.

---

## 12. Four independent trust layers

Keep separate:

~~~text
Engineering Release Authority
Distribution Trust
Device Boot Trust
Runtime/Device Attestation
~~~

No layer substitutes for another.

---

## 13. Rollback / anti-rollback

Release rollback eligibility must consider:
- functional compatibility;
- bootloader compatibility;
- security/version counter;
- downgrade policy;
- credential/key generation.

An old functionally good Artifact may be cryptographically forbidden to boot.

Rollback Plan must therefore bind an actually admissible target.

---

## 14. Trusted production stations

Add scheduler/trust classes as needed:
- PROVISIONING_STATION;
- PRODUCTION_FLASH_STATION;
- DEVICE_ATTESTATION_VERIFIER.

Capabilities are platform/enrollment attested.

They are not Worker self-reported flags.

---

## 15. Implementation candidates

Target-specific:
- Linux/TPM: Keylime/TPM2 concepts;
- Arm M-profile: Trusted Firmware-M / PSA concepts;
- MCU secure update: MCUboot or wolfBoot;
- hardware root-of-trust: Caliptra-like future hardware;
- onboarding: FDO;
- update distribution: RAUC/Mender/TUF/Uptane according to product.

No universal dependency is mandated.

---

## 16. Milestone mapping

### M0
Reserve/freeze:
- DeviceInstance trust fields;
- DeviceProvisioningReceipt;
- DeviceTrustProfile;
- BootTrustEvidence;
- DeviceAttestationEvidence;
- DeviceOwnershipRecord.

### M1
Synthetic fixtures only.

### M2
Lab/device identity and exact flashed/booted Artifact tracking.

### Production
Provisioning/signing/secure-boot/attestation according to real hardware security capability.

---

## 17. Final rules added by v14

1. Physical Device identity differs from Target identity.
2. Provisioning is privileged and receipt-producing.
3. Private provisioning/device keys never enter Control Plane or Runtime.
4. Secure boot, measured boot and remote attestation remain distinct.
5. Device trust, health and allocation are independent.
6. Attestation is freshness-bound.
7. Rollback obeys security counter/anti-rollback policy.
8. DUT identity and Worker workload identity are different trust domains.
9. Production station capability is externally trusted, not self-reported.
10. Release, distribution, boot and runtime trust remain layered.

Architecture v1.2 remains canonical.
Profile v14 is the current implementation companion.
