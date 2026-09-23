# Open Source Reference Review — Round 14: Device Identity, Provisioning, Secure Boot and Attestation

Date: 2026-09-23
Status: **Archived research / implementation input**
Parent:
- docs/architecture/AI_NATIVE_ENGINEERING_PLATFORM_V1_2_FINAL.md
- docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V13.md

## 1. Scope

Round 14 reviewed:
- Keylime
- TPM2 tools / TPM2-TSS
- Trusted Firmware-M / PSA concepts
- MCUboot
- wolfBoot
- Caliptra
- FIDO Device Onboard
- Mender / RAUC / SWUpdate
- TUF / Uptane practices
- Open Horizon edge onboarding patterns

Focus:
- unique device identity;
- factory/manufacturing provisioning;
- secure/measured boot;
- remote attestation;
- device ownership/enrollment;
- firmware update trust;
- lifecycle and key rotation.

---

## 2. Device identity is not Target identity

Keep distinct:

~~~text
TargetRevision
  describes product/hardware/software configuration class

DeviceInstance
  identifies one physical unit
~~~

DeviceInstance may include:
- device_id;
- serial/lot/batch;
- hardware identity;
- board revisions;
- manufacturing metadata;
- trust anchor refs;
- current enrollment owner;
- lifecycle state.

Two devices can share one TargetRevision but have different trust/provisioning histories.

---

## 3. DeviceProvisioningReceipt

Add immutable:

~~~text
DeviceProvisioningReceipt
~~~

Captures a privileged manufacturing/enrollment operation.

May include:
- DeviceInstance;
- factory/line/station identity;
- operation time;
- hardware identity observations;
- provisioned certificate/key references;
- boot trust configuration;
- initial bootloader/firmware Artifact digests;
- serial/lot/calibration refs;
- ownership/onboarding token refs;
- operator/service authority;
- verification/attestation outputs.

Secret/private key material is never stored in the receipt.

Only key/certificate identifiers/public material references are recorded.

---

## 4. Provisioning is a privileged transform

Factory provisioning may change:
- OTP/eFuse;
- secure storage;
- device certificate;
- debug lock;
- boot key hash;
- anti-rollback state;
- ownership credentials.

Therefore provisioning is treated like an irreversible privileged operation:

~~~text
ProvisioningPlan
 -> Human/Manufacturing Authority as policy requires
 -> trusted provisioning station
 -> DeviceProvisioningReceipt
 -> post-provision verification
~~~

Runtime/ordinary Worker cannot perform it merely because it can flash firmware.

---

## 5. Device lifecycle

Suggested lifecycle:

~~~text
UNPROVISIONED
 -> PROVISIONING
 -> ENROLLED
 -> ACTIVE
 -> QUARANTINED
 -> RMA / RETIRED
~~~

Additional operational health/allocation facets remain separate.

Lifecycle is not the same as:
- online/offline;
- leased/free;
- healthy/dirty.

---

## 6. DeviceTrustProfile

Add versioned policy:

~~~text
DeviceTrustProfile
~~~

Defines which trust mechanisms are required for a device class/environment.

Examples:
- DEV_BASIC
- SIGNED_BOOT
- MEASURED_BOOT
- REMOTE_ATTESTED
- HIGH_ASSURANCE

Policy may require:
- signed firmware;
- anti-rollback;
- secure boot;
- hardware root of trust;
- measured boot;
- device certificate;
- remote attestation freshness;
- debug state/lock;
- key rotation/revocation support.

Not every MCU must support every profile.

---

## 7. Secure boot evidence

MCUboot/wolfBoot demonstrate portable signed-image/bootloader patterns.

For relevant targets, capture:

~~~text
BootTrustEvidence
~~~

Potential fields:
- bootloader identity/version;
- boot policy/profile;
- signing key/cert identity;
- verified image digest;
- rollback/security counter;
- boot result;
- device identity;
- issuer/verifier.

Secure boot means:
> device accepted an image under its boot trust policy.

It does not by itself prove functional correctness.

---

## 8. Measured boot / attestation

Keylime/wolfBoot/Caliptra/PSA practices distinguish:
- authentication/secure boot;
- measurement;
- remote attestation.

Add:

~~~text
DeviceAttestationEvidence
~~~

Includes:
- DeviceInstance;
- nonce/challenge;
- attestation mechanism/profile;
- measurement/PCR/claim values;
- boot/runtime measurements;
- verifier/issuer;
- trust root/key refs;
- timestamp/freshness;
- result;
- raw attestation Artifact refs.

Remote attestation is Evidence about device state/trust, not a Release approval.

---

## 9. Attestation freshness

Remote attestation is time/context sensitive.

Applicability depends on:
- nonce/freshness;
- device reboot/state transition;
- trust-root/key validity;
- measurement policy version;
- environment/action.

A valid old attestation cannot authorize arbitrary future high-risk actions.

Policy specifies freshness requirements.

---

## 10. Device trust state vs health state

Separate:

~~~text
trust_state:
  UNKNOWN
  PROVISIONED
  VERIFIED
  ATTESTED
  DEGRADED
  REVOKED

health_state:
  READY
  DIRTY
  MAINTENANCE
  QUARANTINED
  OFFLINE
~~~

A device may be healthy but untrusted.
A device may be trusted but currently offline/broken.

Scheduling and production operations can require both conditions.

---

## 11. Device credential lifecycle

Device credentials/trust anchors need:
- issuer;
- key/cert ID;
- generation;
- validity;
- rotation;
- revocation;
- ownership;
- replacement/RMA semantics.

Private secrets remain in:
- HSM/secure element;
- TPM;
- device secure storage;
- Credential/Trust backend.

Control Plane stores references/status, not reusable private keys.

---

## 12. Factory onboarding / ownership transfer

FIDO Device Onboard demonstrates zero-touch ownership onboarding with manufacturer, rendezvous and owner roles.

### Absorb conceptually

Reserve:

~~~text
DeviceOwnershipRecord
~~~

and optional:

~~~text
OwnershipTransferReceipt
~~~

for:
- manufacturer -> enterprise/fleet owner;
- RMA/replacement;
- lab -> production fleet;
- tenant transfer.

Do not require FDO protocol for M2.

Use FDO when product deployment/zero-touch onboarding actually needs it.

---

## 13. Device enrollment vs workload identity

Worker/Service identity and Device identity are separate.

~~~text
Worker
  workload identity
  manages engineering execution

DeviceInstance
  product/hardware identity
  is a DUT/fleet unit
~~~

SPIFFE/SPIRE may identify Workers/services.
TPM/PSA/device certificates may identify DUTs.

Do not force one identity system onto both.

---

## 14. Update trust layers

Keep separate:

1. **Release Authority**
   - engineering/human decision.

2. **Distribution Trust**
   - TUF/Uptane/signature/metadata.

3. **Device Boot Trust**
   - MCUboot/wolfBoot/secure boot.

4. **Runtime/Device Attestation**
   - Keylime/TPM/PSA/measured boot where applicable.

A secure bootloader cannot replace Human Release Authority.
A Human Approval cannot replace cryptographic image verification.

---

## 15. Rollback protection

For target classes that support it, Release/Target policy should record:
- security/version counter;
- allowed downgrade policy;
- rollback recovery exception;
- bootloader compatibility;
- key generation.

A functional rollback may conflict with anti-rollback policy.

Therefore rollback target must be both:
- operationally compatible;
- cryptographically/anti-rollback admissible.

---

## 16. Manufacturing/production station trust

Trusted provisioning/signing/production-flash stations are different from normal DEV_WORKER.

Potential trust class:

~~~text
PROVISIONING_STATION
PRODUCTION_FLASH_STATION
DEVICE_ATTESTATION_VERIFIER
~~~

Their capabilities are enrollment/policy-attested.

They use dedicated credential/trust paths.

A normal Codex worker cannot self-report these capabilities.

---

## 17. Device trust and Verification

Examples:
- dev functional HIL may allow DEV_BASIC device trust;
- final security/OTA qualification may require SIGNED_BOOT;
- production secret delivery may require fresh REMOTE_ATTESTED state.

Assurance Profile / DeviceTrustProfile determines minimum trust level.

---

## 18. M1 impact

None on runtime dependencies.

M0 reserves:
- DeviceProvisioningReceipt;
- DeviceTrustProfile;
- DeviceAttestationEvidence;
- DeviceOwnershipRecord;
- BootTrustEvidence.

One synthetic Device fixture is enough.

---

## 19. M2/production impact

During Device/HIL/production integration:
- establish DeviceInstance identity;
- bind Target Revision;
- record provisioning state where applicable;
- verify exact flashed/booted Artifact;
- add secure-boot/attestation evidence only for capable targets;
- separate lab device enrollment from production identity.

---

## 20. High-assurance/scale options

Candidates when needed:
- Keylime for Linux/TPM remote attestation;
- TPM2 stack/tools;
- Trusted Firmware-M / PSA attestation concepts;
- MCUboot/wolfBoot for MCU secure boot/update;
- Caliptra-like hardware root of trust for future platforms;
- FIDO Device Onboard for ownership onboarding;
- TUF/Uptane for high-assurance distribution.

These remain target-specific implementations.

---

## 21. New invariants

1. **Target identity and physical Device identity are distinct.**
2. **Provisioning is an auditable privileged operation with a receipt.**
3. **Private device/provisioning keys never become Control Plane data or Runtime secrets.**
4. **Secure boot, measured boot and remote attestation are distinct evidence classes.**
5. **Device trust state and operational health state are independent.**
6. **Attestation has freshness and trust-root applicability.**
7. **Rollback must satisfy both functional compatibility and anti-rollback/security policy.**
8. **Worker identity and DUT/device identity remain separate trust domains.**
9. **Production/provisioning station capability is externally attested/policy-managed, not self-reported.**
10. **Release approval, distribution trust, boot trust and runtime attestation remain separate layers.**

---

## 22. Conclusion

Round 14 closes the production-device trust gap without imposing one hardware security technology on all targets.

The platform owns:

> **Device identity, lifecycle, provisioning receipts, trust requirements and evidence applicability.**

TPM/PSA/MCUboot/FDO/etc. are target-specific mechanisms underneath that authority model.
