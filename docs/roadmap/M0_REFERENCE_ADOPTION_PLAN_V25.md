# M0 Reference Adoption Plan v25

Date: 2026-09-24
Status: **Current execution plan**
Supersedes: docs/roadmap/M0_REFERENCE_ADOPTION_PLAN_V21.md

Parents:
- docs/architecture/AI_NATIVE_ENGINEERING_PLATFORM_V1_2_FINAL.md
- docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V25.md

## 1. Principle

Rounds 22–25 add only schema/fixture coverage for:
- hardware/product structure change;
- product security response;
- RMA/repair;
- product family/variant resolution.

No new M1 service is required.

---

# Track A — Hardware/PLM

## 2. Hardware design fixture

Create synthetic:
- schematic Artifact;
- PCB Artifact;
- BOM/ProductStructureSnapshot;
- DesignExportReceipt;
- ERC/DRC Evidence.

Prove:
- source design and manufacturing outputs have different digests;
- manufacturing outputs bind exact source/export config/tool;
- changing BOM/design creates new Target/hardware subject.

---

## 3. Engineering change fixture

Create:
- TargetRevision r1;
- EngineeringChangePackage;
- TargetRevision r2;
- EffectivityRule.

Prove:
- approval does not immediately rewrite all existing devices;
- r1 historical manufacturing remains intact;
- effectivity selects only intended population;
- impact analysis identifies affected interfaces/tests.

---

# Track B — Product Security

## 4. ProductSecurityCase fixture

Use one synthetic CVE/security finding.

Create:
- ProductSecurityCase;
- VulnerabilityApplicabilityAssessment;
- VEXArtifact;
- SecurityRemediationPlan.

Cases:
- scanner finds component but product NOT_AFFECTED with justification;
- product AFFECTED and fix Release created.

Prove:
- scanner result != affectedness;
- VEX projection does not replace internal case;
- fix merge alone does not close case.

---

## 5. Regulatory reporting fixture

Create one generic:

~~~text
RegulatoryNotificationCase
~~~

with:
- awareness time;
- policy/version;
- 24h/72h-style staged deadlines as test data;
- submission receipts.

Prove:
- deadline is calculated from policy + awareness;
- legal/regulatory case is distinct from internal ProductSecurityCase;
- external submission status cannot mutate engineering facts.

No real CRA connector required.

---

# Track C — RMA

## 6. Return-state fixture

Create:
- DeviceInstance;
- AS_BUILT snapshot;
- AS_RETURNED snapshot.

Prove:
- repair cannot overwrite AS_RETURNED;
- field firmware/config differences remain visible.

---

## 7. Repair fixture

Create:
- FailureAnalysisRecord;
- RepairActionReceipt;
- one serialized component replacement;
- POST_REPAIR snapshot;
- post-repair Verification.

Prove:
- old/new component lineage retained;
- ERP ticket closure cannot substitute for engineering requalification;
- NO_FAULT_FOUND case can retain real reproduction/test Evidence.

---

# Track D — Variants

## 8. Feature model fixture

Create one family:
- base product;
- two hardware options;
- one region option;
- one invalid combination.

Freeze:
- FeatureModelArtifact;
- ProductVariantDefinition;
- VariantResolutionReceipt.

Prove:
- invalid combination is rejected;
- implied/default values participate in final digest;
- SKU != TargetRevision.

---

## 9. Variant Evidence fixture

Create:
- Variant A;
- Variant B;
- one shared test candidate.

Default:
- Evidence cannot be reused.

Then add:
- VariantEquivalenceDecision scoped to one Evidence class.

Prove:
- reuse occurs only within declared scope;
- changing variant dimensions invalidates equivalence as defined by policy.

---

## 10. SupportedVariantSet fixture

Create one Release that supports:
- explicit variants A/B;
- excludes C.

Prove Release Admission rejects a Device/Target outside the supported set.

---

# Track E — Existing M0

## 11. Retain all v21 work

All previous M0 requirements remain:
- canonicalization;
- Run/Build/Integration chains;
- Interface compatibility;
- ResolvedConfiguration;
- ToolProfile / AIUsagePolicy / Runtime qualification;
- Review governance;
- Manufacturing/Fleet;
- Knowledge lifecycle;
- Device trust;
- ML lineage;
- Incident/RMA feedback;
- OTel/CDEvents;
- formal model-check spike;
- failure injection.

---

## 12. M0 exit additions

M0 additionally requires:
- hardware design/export lineage fixture;
- ECO/effectivity fixture;
- product-security/VEX fixture;
- regulatory-reporting policy fixture;
- AS_RETURNED → repair → POST_REPAIR fixture;
- variant-resolution/constraint fixture;
- supported-variant admission fixture.

M0 does not require:
- PLM/MES/ERP/CRM;
- KiCad/KiBot;
- PSIRT product;
- feature-model server.

---

## 13. M1 target

M1 remains unchanged operationally.

The expanded fixtures only guarantee that future hardware/product-lifecycle systems can attach to the same authority model.
