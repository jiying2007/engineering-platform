# M0 Reference Adoption Plan v35

Date: 2026-09-24
Status: **Current execution plan**
Supersedes: docs/roadmap/M0_REFERENCE_ADOPTION_PLAN_V29.md

Parents:
- docs/architecture/AI_NATIVE_ENGINEERING_PLATFORM_V1_2_FINAL.md
- docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V35.md

## 1. Principle

Rounds 31–35 add only schema/fixture coverage for:
- conformity/certification;
- privacy/data lifecycle;
- prognostics;
- long-term compatibility;
- threat modeling.

No new M1 production service is required.

---

# Track A — Certification

## 2. Certification fixture

Create:
- one CertificationRequirementSet;
- one ConformityAssessmentCase;
- one CertificationSample;
- one CertificationTestReportArtifact;
- one Certificate/Declaration artifact.

Prove:
- exact Target/Variant/sample identity is bound;
- changing a material hardware/config item triggers CertificationDeltaAssessment;
- old report remains historical but may become inapplicable.

---

## 3. Battery qualification fixture

Use synthetic:
- cell/pack design;
- BMS/config;
- physical Procedure report;
- optional ModelArtifact/simulation.

Prove:
- simulation Evidence and physical certification Evidence remain distinct;
- changed battery/BMS/config cannot reuse report by product name alone.

---

# Track B — Privacy

## 4. DataAssetProfile fixture

Create representative:
- source-code/prompt context;
- device telemetry;
- RMA log/audio object.

Assign:
- sensitivity;
- allowed purpose;
- retention;
- provider/export restrictions.

Prove:
- DEBUG permission does not imply MODEL_TRAINING permission;
- provider transfer is policy-evaluated.

---

## 5. Retention/disposition fixture

Create one:
- DataRetentionPolicy;
- DataDispositionReceipt.

Cases:
- archive;
- secure delete;
- legal/compliance hold.

Prove:
- disposition is auditable without preserving deleted sensitive content.

---

## 6. Privacy-transform fixture

Create:
- SensitiveDataFinding;
- PrivacyTransformReceipt;
- redacted output Artifact.

Prove:
- transform profile/version is digest-bound;
- transformed output is distinct Artifact;
- detector result is not automatic legal classification.

---

# Track C — Prognostics

## 7. Health-estimate fixture

Create:
- TelemetryWindowArtifact;
- HealthModelArtifact;
- HealthStateEstimate;
- PrognosticEstimate.

Include uncertainty.

Prove:
- exact model/input window bound;
- changed Target/config can invalidate applicability;
- estimate is not treated as observed fact.

---

## 8. Maintenance-decision fixture

Create:
- AnomalyFinding;
- MaintenanceRecommendation;
- accepted/rejected Decision;
- MaintenanceActionReceipt.

Prove:
- recommendation cannot directly execute privileged service action;
- actual maintenance outcome feeds later model qualification without rewriting prediction.

---

# Track D — Long-term compatibility

## 9. Compatibility-envelope fixture

Use synthetic product:
- Linux;
- main MCU;
- motor MCU.

Define:
- VersionSkewPolicy;
- CompatibilityEnvelope;
- one unsupported combination.

Prove:
- pairwise API-compatible but envelope-invalid combination is rejected;
- skew is directional.

---

## 10. Upgrade-order fixture

Define:
- old system state;
- target state;
- UpgradeOrderConstraint.

Prove:
- valid target does not imply every transition order is valid;
- SystemUpdateManifest must obey the order;
- rollback path is checked separately.

---

## 11. Support-policy fixture

Create:
- ProductSupportPolicy;
- old LTS line;
- current line;
- EOL line.

Prove:
- technical compatibility != currently supported;
- historical Evidence remains after EOL;
- new fleet targeting can reject EOL versions.

---

# Track E — Threat modeling

## 12. Platform threat-model fixture

Create one ThreatModelArtifact for:
- Control Plane;
- Worker;
- Session Gateway;
- Action Gateway;
- Artifact/Evidence store;
- Tool/MCP boundary.

Include threats:
- stale Worker mutation;
- credential exfiltration;
- malicious MCP/tool description;
- prompt/tool-output injection;
- unauthorized production action;
- Evidence tampering.

---

## 13. Threat trace fixture

Create:
- ThreatScenario;
- linked Security Requirement;
- ThreatMitigationDecision;
- Verification Evidence.

Prove:
- threat mitigation does not close without explicit Decision/Evidence;
- residual risk remains visible.

---

## 14. Threat freshness fixture

Change one:
- ToolProfile;
- Interface Contract;
- data flow.

Prove:
- ThreatModelReviewReceipt is required;
- old model is marked stale/inapplicable according to policy.

---

# Track F — Existing M0 work

## 15. Retain all v29 requirements

All earlier M0 work remains:
- canonical serialization;
- Run/Build/Integration/Release chains;
- PLM/hardware/PSIRT/RMA/variant;
- supplier/CAPA/compliance/twin/FOSS;
- AI governance/review;
- Device trust;
- ML lineage;
- Interface compatibility;
- ResolvedConfiguration;
- TraceLink;
- Incident/Knowledge;
- OTel/CDEvents;
- formal model-check spike;
- failure injection.

---

## 16. M0 exit additions

M0 additionally requires:
- certification delta/applicability fixture;
- privacy purpose/retention/disposition fixture;
- one privacy transform;
- one uncertainty-bearing prognostic estimate;
- one maintenance recommendation/Decision path;
- compatibility-envelope/skew/upgrade-order fixture;
- ProductSupportPolicy fixture;
- one platform ThreatModel with traceable mitigation Evidence;
- threat-model freshness trigger.

No certification/privacy/prognostics/threat-modeling service is required.

---

## 17. M1 target

M1 remains the same narrow trusted engineering loop.

The added fixtures ensure product certification, sensitive-data handling, fleet health, long-term support and security-by-design attach later without redesigning the authority model.
