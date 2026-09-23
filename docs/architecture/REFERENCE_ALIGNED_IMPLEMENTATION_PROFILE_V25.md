# Reference-Aligned Implementation Profile v25

Date: 2026-09-24
Status: **Canonical implementation companion to architecture v1.2**
Supersedes: docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V21.md

Research basis:
- Open Source Reference Review Rounds 1–25
- Architecture v1.2

## 1. Goal

Profile v25 adds four durable product-lifecycle capabilities while keeping PLM/MES/ERP/CRM external:

1. hardware design/BOM and engineering change;
2. product-security/PSIRT response;
3. RMA/repair state preservation;
4. product-family/SKU/variant resolution.

M1 runtime dependencies remain unchanged.

---

# Part A — Hardware / PLM / ECO

## 2. Hardware design and product structure

Use:
- HardwareDesignArtifact
- ProductStructureSnapshot
- DesignExportReceipt
- ExternalPLMRef

Hardware design/formal manufacturing outputs are immutable/content-addressed.

EDA source and exported Gerber/BOM/P&P/documentation are distinct artifacts linked by DesignExportReceipt.

---

## 3. EngineeringChangePackage

Add immutable:

~~~text
EngineeringChangePackage
~~~

Binds:
- old/new TargetRevision;
- old/new ProductStructureSnapshot;
- HardwareDesignArtifact changes;
- BOM delta;
- Interface Contract changes;
- reason;
- impact;
- required Verification;
- approvals;
- EffectivityRule.

Suggested state:
- PROPOSED
- ANALYZED
- REVISION_READY
- VERIFYING
- APPROVED
- EFFECTIVE
- REJECTED/CANCELLED/SUPERSEDED

Approval and production effectivity are separate.

---

## 4. EffectivityRule

Supports:
- time/date;
- serial/lot/batch;
- factory/site;
- SKU/variant;
- supplier/material;
- production order.

This connects engineering change to real manufacturing/fleet population.

---

## 5. As-designed versus as-built

~~~text
AsDesigned
  TargetRevision
  ProductStructureSnapshot
  HardwareDesignArtifacts

AsBuilt
  DeviceInstance
  ManufacturingResultReceipt
  actual parts/config/calibration
~~~

Substitution/rework never rewrites the nominal design baseline.

---

# Part B — Product Security / PSIRT

## 6. ProductSecurityCase

Add stable lifecycle object:

~~~text
ProductSecurityCase
~~~

Aggregates:
- vulnerability IDs/findings;
- candidate affected products/releases;
- severity/risk;
- embargo/disclosure;
- applicability assessments;
- remediation Work;
- advisory/VEX/CVE refs;
- regulatory cases;
- closure.

---

## 7. VulnerabilityApplicabilityAssessment

Exact-subject assessment binds:
- vulnerability;
- Artifact/Release/Target;
- SBOM;
- runtime/config;
- Procedure/tool;
- status;
- justification;
- Evidence.

Prefer interoperable VEX status semantics:
- under_investigation;
- affected;
- not_affected;
- fixed.

A scanner component match is not product affectedness.

---

## 8. Security interchange

Use Artifact types:
- VEXArtifact
- SecurityAdvisoryArtifact

Preferred standards:
- OpenVEX/CycloneDX VEX/CSAF as fits ecosystem;
- CVE typed references/records.

Internal ProductSecurityCase remains authority.

---

## 9. SecuritySupportPolicy and remediation

Add:
- SecuritySupportPolicy
- SecurityRemediationPlan

Support window/version policy is explicit.
Temporary mitigation and permanent fix are distinct.

Security closure requires remediation/fix Verification.

---

## 10. RegulatoryNotificationCase

Jurisdiction-specific reporting is isolated in:

~~~text
RegulatoryNotificationCase
~~~

Binds:
- regulation/jurisdiction;
- trigger/case;
- awareness time;
- reporting stages/deadlines;
- submitter/authority;
- receipts;
- embargo/confidentiality.

Generic domain does not hard-code one regulation.

Current CRA obligations may be implemented as one policy profile.

---

# Part C — RMA / Repair

## 11. ExternalServiceCaseRef and ReturnReceipt

CRM/ERP/warranty system remains external.

engineering-platform uses:
- ExternalServiceCaseRef
- ReturnReceipt

ReturnReceipt starts the technical investigation and preserves reported/observed returned state.

---

## 12. DeviceStateSnapshot

Immutable states:
- AS_BUILT
- AS_SHIPPED
- FIELD_OBSERVED
- AS_RETURNED
- PRE_REPAIR
- POST_REPAIR
- AS_REPLACED

Rule:

> Never repair before freezing the returned technical state.

---

## 13. FailureAnalysisRecord and RepairActionReceipt

FailureAnalysisRecord links:
- symptoms;
- reproduction;
- teardown;
- measurements;
- hypotheses;
- root-cause Decision;
- Evidence.

RepairActionReceipt records:
- replaced serialized parts;
- firmware/config/calibration changes;
- station/technician;
- Procedure;
- Evidence;
- resulting Device state.

Repair is append-only technical history.

---

## 14. Repair requalification

Repair completion may require:
- functional test;
- calibration;
- secure/provisioning check;
- production/HIL test;
- exact POST_REPAIR snapshot.

ERP "repair closed" does not imply engineering PASS.

---

# Part D — Product Families / Variants

## 15. FeatureModelArtifact

Immutable model of valid product configuration space:
- features/options;
- constraints;
- dependencies;
- exclusions;
- attributes;
- defaults.

FeatureIDE/Kconfig-style semantics are references.

---

## 16. ProductVariantDefinition

One commercial/market variant intent:
- product family;
- SKU/market;
- selected features;
- hardware options;
- region/regulatory attributes;
- external ERP/PLM refs;
- effectivity.

SKU is not TargetRevision.

---

## 17. Variant resolution

~~~text
FeatureModelArtifact
 + ProductVariantDefinition
 + platform/default constraints
 -> VariantResolutionReceipt
 -> ResolvedConfigurationArtifact
 -> TargetRevision
~~~

VariantResolutionReceipt records:
- selected/defaulted/implied features;
- rejected conflicts;
- resolver/version;
- final digest.

---

## 18. VariantConstraintEvidence

Proves the requested variant/configuration is valid/satisfiable.

Configuration validity is not functional Verification.

---

## 19. Variant coverage and evidence reuse

Use:
- VariantCoveragePlan
- VariantEquivalenceDecision
- SupportedVariantSet

Cross-variant Evidence reuse is explicit and scoped.

Default is no implicit equivalence.

---

## 20. Release/manufacturing scope

Release Manifest / ProductionRecipe bind:
- ProductVariantDefinition;
- TargetRevision;
- ProductStructureSnapshot;
- ResolvedConfigurationArtifact;
- exact component Release Bundle.

Sales SKU alone is insufficient engineering identity.

---

# Part E — Milestone Strategy

## 21. M0 reserve/freeze

Add:
- ExternalPLMRef
- HardwareDesignArtifact kinds
- DesignExportReceipt
- ProductStructureSnapshot
- EngineeringChangePackage
- EffectivityRule
- ProductSecurityCase
- VulnerabilityApplicabilityAssessment
- VEXArtifact
- SecurityAdvisoryArtifact
- SecuritySupportPolicy
- SecurityRemediationPlan
- RegulatoryNotificationCase
- ExternalServiceCaseRef
- ReturnReceipt
- DeviceStateSnapshot
- FailureAnalysisRecord
- RepairActionReceipt
- FeatureModelArtifact
- ProductVariantDefinition
- VariantResolutionReceipt
- VariantConstraintEvidence
- VariantCoveragePlan
- VariantEquivalenceDecision
- SupportedVariantSet

Synthetic fixtures only.

---

## 22. M1

No new services:
- no PLM;
- no KiCad/KiBot requirement;
- no PSIRT platform;
- no RMA/ERP connector;
- no feature-model service.

The schemas simply remain compatible with future product lifecycle integration.

---

## 23. M2/M3

Adopt based on actual product needs:
- KiCad/KiBot design export;
- PLM/ERP connector;
- PSIRT/SBOM/VEX workflow;
- service/RMA connector;
- variant/Target resolution;
- manufacturing effectivity and fleet segmentation.

---

## 24. Final rules added by v25

1. Hardware/part revisions used formally are immutable.
2. EDA manufacturing outputs are reproducibly derived from exact design sources.
3. Engineering change approval and production effectivity are distinct.
4. As-designed and as-built states are both preserved.
5. Vulnerability scanner match is not product affectedness.
6. Product-security case, VEX/advisory, remediation and regulatory reporting are distinct.
7. Returned device state is frozen before repair.
8. Repair actions never erase original manufacturing/field history.
9. Product family rules, commercial SKU, resolved variant and TargetRevision are distinct.
10. Cross-variant Evidence reuse requires explicit equivalence.
11. Requirements/Verification/Release can be scoped to variant predicates.
12. External PLM/MES/ERP/CRM systems remain domain integrations, not hidden engineering authority.

Architecture v1.2 remains canonical.
Profile v25 is the current implementation companion.
