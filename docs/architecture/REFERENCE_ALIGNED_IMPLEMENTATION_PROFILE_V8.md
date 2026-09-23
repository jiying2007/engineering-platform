# Reference-Aligned Implementation Profile v8

Date: 2026-09-23
Status: **Canonical implementation companion to architecture v1.2**
Supersedes: docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V7.md

Research basis:
- Open Source Reference Review Rounds 1–7
- Open Source Reference Synthesis Optimization
- Architecture v1.2

## 1. Goal

Profile v8 adds framework-neutral test-report ingestion, auditable change-impact selection and BOM-family interoperability.

M1 runtime dependencies remain unchanged.

---

## 2. TestReportArtifact

Introduce an immutable input/interchange artifact:

~~~text
TestReportArtifact
~~~

Fields include:
- artifact/content digest;
- format;
- format version;
- producer/tool version;
- source Procedure/Run/Attempt;
- logical test-run identifier;
- conversion provenance if transformed.

Candidate formats:
- CTRF;
- JUnit XML;
- Allure-native result;
- project-specific JSON/XML.

CTRF is preferred where available but remains pre-1.0 and does not define platform authority.

---

## 3. Result normalization

Canonical path:

~~~text
test framework
 -> TestReportArtifact / Result Sink
 -> normalized TestAttemptResult
 -> TestVerdict
 -> Attestation Controller
 -> Evidence
 -> Verification
~~~

Rules:
- parser/converter never changes original report bytes;
- conversion creates a receipt/provenance link;
- unknown fields/results fail explicit normalization rather than silently disappear;
- canonical TestAttempt identity is platform-defined.

---

## 4. Verification selection is a Decision

Add:

~~~text
VerificationSelectionDecision
~~~

Inputs:
- old/new subject/source digest;
- change set;
- dependency/compatibility graph;
- Verification Plan;
- Assurance Profile;
- selector/version.

Outputs:
- selected checks;
- omitted checks;
- reason per omitted check;
- coverage metadata;
- conservative fallback flag.

The Decision is content-addressed/auditable and feeds Resolved Verification Execution Plan.

---

## 5. Negative-space preservation

Resolved Verification Execution Plan must retain:

~~~text
selected
filtered
omitted
skipped
quarantined
reason
selection rule/version
~~~

This allows later reconstruction of:
- what ran;
- what did not run;
- why.

A green result with invisible omitted coverage is invalid design.

---

## 6. Conservative fallback

Change-impact/test-selection engines are optimization only.

Policy rule:

~~~text
if safe coverage reduction cannot be established
 -> expand to required/full Verification Plan
~~~

No selector confidence score alone grants release authority.

---

## 7. BOMArtifact family

Add immutable:

~~~text
BOMArtifact
~~~

Kinds:
- SBOM;
- HBOM;
- ML_BOM;
- CBOM;
- MBOM;
- CUSTOM.

Formats:
- CycloneDX;
- SPDX;
- other approved standards.

For embedded Target/Release, useful references may include:
- software components;
- hardware assemblies;
- model assets;
- cryptographic algorithms/keys/profiles.

BOM content does not itself approve compatibility or Release.

---

## 8. Supply-chain fact/evaluation split

SupplyChainIntel pipeline semantics:

~~~text
Inventory/BOM
 -> Scanner/Advisor Facts
 -> normalized Findings
 -> policy/risk evaluation
 -> Decision/Verification
~~~

Do not collapse:
- vulnerability fact;
- exploitability/VEX;
- policy violation;
- accepted risk

into one result.

---

## 9. Finding schema

Normalize static/security/license findings:

~~~text
finding_id
category
rule_id
severity
subject_digest
location/component
tool
tool_version
raw_report_ref
observed_at
status/applicability
~~~

A trusted Procedure/Issuer may turn a Finding Set into Evidence.

Otherwise it remains an Engineering Check or risk input.

---

## 10. Reporting UIs remain projections

Allure/Testkube/custom dashboards may visualize:
- test attempts;
- artifacts;
- history;
- trends.

They never own canonical TestVerdict or Verification.

---

## 11. M0/M1 testing optimization

Use disposable integration environments, e.g. Testcontainers-style patterns, for:
- PostgreSQL;
- ArtifactStore;
- OPA;
- Toxiproxy;
- provider mocks.

Goal:
- local/CI parity;
- isolated tests;
- automatic cleanup;
- fewer shared mutable test services.

This is a test implementation technique, not a runtime dependency.

---

## 12. Dependency maintenance

Automated dependency-update tools may open changes/PRs.

Those enter the normal path:

~~~text
automated change
 -> Requirement/Maintenance Work as policy defines
 -> Integration Subject
 -> Verification
 -> Release
~~~

Automation metadata may help risk classification but never bypasses gates.

---

## 13. Milestone mapping

### M0
Freeze:
- TestReportArtifact;
- VerificationSelectionDecision;
- BOMArtifact;
- Finding schema;
- report conversion receipt.

### M1
- integration tests use disposable infrastructure where practical;
- CI result ingestion can remain simple;
- no full test-report ingestion service required.

### M2
- framework report adapters;
- Local Result Sink;
- TestVariant/Attempt/Verdict;
- selected BOM generation;
- optional change-impact selector after trustworthy baseline coverage exists.

---

## 14. Final rules added by v8

1. **Native framework reports are immutable inputs, not canonical verdicts.**
2. **Normalization has explicit conversion provenance.**
3. **Change-impact selection is an auditable Decision, not hidden scheduler logic.**
4. **Omitted verification coverage is first-class data.**
5. **Uncertain impact expands verification coverage.**
6. **CycloneDX/SPDX BOMs are interoperable artifacts, not approval authority.**
7. **Supply-chain facts, evaluation and risk acceptance remain separate.**

Architecture v1.2 remains canonical.
Profile v8 is the current implementation companion.
