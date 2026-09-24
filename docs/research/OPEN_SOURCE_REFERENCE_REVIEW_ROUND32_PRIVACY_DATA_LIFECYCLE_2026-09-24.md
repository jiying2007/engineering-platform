# Open Source / Public Practice Review — Round 32: Privacy and Engineering Data Lifecycle

Date: 2026-09-24
Status: **Archived research / implementation input**

References:
- NIST Privacy Framework
- OpenDP
- Microsoft Presidio concepts
- DataHub / OpenMetadata governance and lineage practices
- existing Artifact, TelemetryProfile, ToolProfile, RMA and Knowledge semantics

## 1. Key conclusion

Privacy and sensitive-data governance must follow the full data lifecycle:

~~~text
collect
 -> process
 -> use
 -> share/export
 -> retain
 -> transform/de-identify
 -> archive
 -> delete/dispose
~~~

The durable engineering model should track purpose, scope, lineage, retention and transformations without turning engineering-platform into a full privacy-management suite.

---

## 2. DataAssetProfile

Add:

~~~text
DataAssetProfile
~~~

for engineering-relevant data classes.

Examples:
- device telemetry;
- crash dump;
- logs;
- audio/video;
- RMA customer/device data;
- prompts/tool outputs;
- training/evaluation datasets;
- diagnostic bundles;
- security incident data.

Fields:
- data class/category;
- sensitivity/confidentiality;
- personal/sensitive-data flags;
- owner/steward;
- source;
- allowed environments;
- export/share restrictions;
- retention policy;
- storage class;
- redaction/de-identification requirements.

---

## 3. DataProcessingActivity

Add:

~~~text
DataProcessingActivity
~~~

Binds:
- DataAssetProfile;
- exact source/system;
- purpose;
- processing steps;
- recipients/destinations;
- Runtime/tool/provider;
- storage location/class;
- jurisdiction/market where relevant;
- retention;
- output Artifact/Data refs;
- policy version.

This records why/how data is processed rather than only where bytes are stored.

---

## 4. Purpose limitation

Formal processing should declare:

~~~text
ProcessingPurpose
~~~

Examples:
- DEBUG;
- VERIFICATION;
- RMA_DIAGNOSIS;
- MODEL_TRAINING;
- PRODUCT_ANALYTICS;
- SECURITY_INVESTIGATION;
- SUPPORT;
- COMPLIANCE.

A DataAsset approved for one purpose is not automatically approved for all purposes.

Purpose participates in Action/Tool policy where relevant.

---

## 5. DataRetentionPolicy

Add versioned:

~~~text
DataRetentionPolicy
~~~

Defines:
- data class/scope;
- retention duration/event trigger;
- archival tier;
- deletion/anonymization action;
- legal/compliance hold behavior;
- exception process;
- owner;
- review/version.

Retention is policy-driven, not hard-coded in storage implementation.

---

## 6. DataDispositionReceipt

Deletion/disposition should be auditable:

~~~text
DataDispositionReceipt
~~~

Records:
- data/object scope;
- requested/triggered reason;
- policy;
- action:
  DELETE
  SECURE_ERASE
  ANONYMIZE
  ARCHIVE
  RETAIN_ON_HOLD
- execution provider;
- result;
- exceptions;
- observed_at.

The receipt proves an action was attempted/confirmed under policy; it does not expose deleted sensitive content.

---

## 7. SensitiveDataFinding

PII/sensitive-data detectors such as Presidio can produce:

~~~text
SensitiveDataFinding
~~~

Fields:
- exact Artifact/data ref;
- category/type;
- location/range;
- detector/version;
- confidence;
- raw report ref;
- status.

Detector output is a Finding, not an automatic legal/privacy classification decision.

---

## 8. PrivacyTransformReceipt

For redaction/de-identification:

~~~text
PrivacyTransformReceipt
~~~

Binds:
- input data digest/ref;
- transformation type;
- tool/version;
- parameters/policy;
- output Artifact;
- validation Evidence;
- limitations.

Examples:
- redact identifiers;
- blur faces;
- remove audio segments;
- tokenize/pseudonymize;
- aggregate;
- differential-privacy release.

Changing the transform/profile creates a new output Artifact.

---

## 9. OpenDP practice

OpenDP is a strong reference for privacy-preserving statistical computation, especially differential privacy.

Use it when a real analytics/data-sharing use case needs formal privacy guarantees.

Do not introduce differential privacy merely because telemetry exists.

If used:
- privacy parameters/mechanism are part of PrivacyTransformReceipt;
- utility/privacy tradeoff is explicit;
- tool/version and assumptions are retained.

---

## 10. Privacy assessment

Reuse Compliance/Assessment model rather than adding a new authority plane.

A privacy-focused:

~~~text
ComplianceProfile
~~~

or optional:

~~~text
PrivacyRiskAssessmentArtifact
~~~

may capture:
- data-flow inventory;
- processing purposes;
- risks;
- mitigations;
- residual risk;
- reviewer/owner;
- Evidence.

NIST Privacy Framework is a useful control/outcome reference.

---

## 11. Data lineage

Use TraceLink / DataHub/OpenMetadata-style lineage to answer:
- where did this data come from?
- which Run/Device/customer/session produced it?
- which Dataset/model was derived from it?
- where was it exported?
- which Knowledge/Incident references it?

High-volume raw telemetry stays outside core DB; metadata/lineage stays inside.

---

## 12. RMA privacy

Returned devices can contain:
- Wi-Fi credentials;
- account tokens;
- audio/video;
- location/history;
- user logs.

RMA Procedure must bind an explicit DataProcessingActivity and retention/disposition policy before unrestricted extraction.

"Engineering debug" is not a blanket purpose.

---

## 13. AI Runtime privacy

Run Input/ToolProfile/TelemetryProfile should control:
- whether source/prompts may leave company boundary;
- provider retention/training policy assumptions;
- whether full tool results are logged;
- sensitive-data redaction;
- data-region/provider restrictions;
- content retention.

RuntimeInstructionProfile is guidance only.
Tool/Provider policy is enforcement.

---

## 14. Dataset/privacy lineage

DatasetArtifact may contain personal/sensitive data.

Dataset metadata should reference:
- DataAssetProfile;
- allowed ProcessingPurpose;
- retention;
- consent/legal-basis refs when applicable;
- de-identification status;
- access restrictions.

Model release does not erase upstream data obligations.

---

## 15. DataSubject / external rights requests

If product/legal requirements need it, reserve external connector semantics for:
- access/export;
- deletion;
- correction;
- restriction.

engineering-platform need not become a consumer privacy request portal.

It should be able to trace affected engineering data and record execution receipts where engineering systems participate.

---

## 16. Fides status

The previously identified Fides repository is now archived.

Decision:
- do not introduce it as a new hard dependency;
- retain useful data-map/privacy-automation concepts only.

---

## 17. M0/M1 impact

M0 reserves:
- DataAssetProfile;
- DataProcessingActivity;
- ProcessingPurpose vocabulary;
- DataRetentionPolicy;
- DataDispositionReceipt;
- SensitiveDataFinding;
- PrivacyTransformReceipt.

M1 implements only:
- privacy-aware Telemetry/Artifact retention defaults;
- explicit handling for prompts/source/tool content.

No privacy platform is required.

---

## 18. M2/M3 impact

As real data use grows:
- classify device/RMA/telemetry data;
- bind retention/disposition;
- add PII detection/redaction;
- govern model-training datasets;
- produce privacy assessment packages where required.

---

## 19. Invariants

1. Data collection/storage and permitted purpose are separate concepts.
2. Data use is purpose/policy scoped.
3. Retention/deletion is versioned policy and produces auditable disposition receipts.
4. Sensitive-data detector output is a Finding, not legal truth.
5. De-identification/redaction is an immutable transform with provenance.
6. High-volume telemetry may remain outside core DB while lineage/metadata stays traceable.
7. RMA/debug purpose does not justify unrestricted user-data extraction.
8. AI Runtime data handling is enforced by provider/tool policy, not prompt text.
9. Dataset/model lineage retains upstream privacy obligations.
10. Archived privacy tools are not adopted solely because their design was useful.

## 20. Conclusion

The durable privacy rule is:

> **For every sensitive engineering data asset, the platform should be able to state where it came from, why it may be used, where it went, how long it may remain, and what transformation or deletion actually happened.**
