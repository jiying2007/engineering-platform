# Open Source Reference Review — Round 7: Test Interchange, Quality Gates and Change Impact

Date: 2026-09-23
Status: **Archived research / implementation input**
Parent:
- docs/architecture/AI_NATIVE_ENGINEERING_PLATFORM_V1_2_FINAL.md
- docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V7.md

## 1. Scope

Round 7 reviewed:
- CTRF
- Allure
- Testkube
- CodeQL
- Semgrep
- Nx affected graph
- Testcontainers
- OSS Review Toolkit
- CycloneDX
- SPDX
- Renovate

Focus:
- framework-neutral test-result ingestion;
- quality/security findings;
- change-based test selection;
- BOM standards;
- integration-test infrastructure;
- automated maintenance.

---

## 2. Test report interchange — CTRF

Repository:
- https://github.com/ctrf-io/ctrf

CTRF defines a framework/language-agnostic JSON format for test execution results and aggregation.

Important:
- normative JSON Schema;
- logical-run model;
- flat test-case representation;
- stable identity/correlation fields;
- explicit extension points;
- currently pre-1.0 and still evolving.

### Optimization

Introduce:

~~~text
TestReportArtifact
~~~

Metadata:
- format;
- format_version;
- producer/tool;
- logical run reference;
- content digest;
- source Attempt/Procedure;
- conversion provenance if converted.

Supported formats may include:
- CTRF;
- JUnit XML;
- Allure result format;
- project-specific structured report.

### Boundary

TestReportArtifact is interchange/input data.

It does not replace:
- TestVariant;
- TestAttemptResult;
- TestVerdict;
- Evidence;
- Verification.

---

## 3. Result normalization pipeline

Recommended M2 ingestion:

~~~text
Test Framework
 -> native report
 -> TestReportArtifact
 -> Report Adapter / Local Result Sink
 -> normalized TestAttemptResult(s)
 -> TestVerdict
 -> Attestation Controller
 -> Evidence
 -> Verification
~~~

This prevents platform domain schemas from becoming tied to pytest, Robot, Allure, JUnit or CTRF.

CTRF is a preferred interchange candidate when producer support is available, but the platform remains tolerant of other formats.

---

## 4. Allure — presentation is not authority

Repository:
- https://github.com/allure-framework/allure2

Allure provides rich multi-language test-report visualization.

Decision:
- reporting/UI tool only;
- may consume normalized test outputs;
- never becomes result authority;
- platform does not require Allure.

---

## 5. Testkube — useful control/test separation reference

Repository:
- https://github.com/kubeshop/testkube

Testkube demonstrates:
- executing many test frameworks;
- agent/control-plane separation;
- centralized test results/artifacts/logs;
- multiple triggers;
- MCP/AI integration.

Useful validation:
- test execution framework should remain pluggable below Procedure semantics;
- result aggregation belongs outside individual test runners.

Do not adopt Testkube for M2 unless Kubernetes becomes the dominant test execution substrate.

---

## 6. Change-impact / affected-test selection

Nx and similar large-repo build systems calculate affected projects/tasks from:
- file changes;
- dependency graph;
- target graph.

### Optimization

Introduce a typed:

~~~text
VerificationSelectionDecision
~~~

which proposes the resolved verification subset.

Inputs:
- old/new source/subject digest;
- changed files/components;
- dependency/compatibility graph;
- Assurance Profile;
- Verification Plan;
- selection engine/version.

Outputs:
- selected checks;
- omitted checks;
- reason per omission;
- conservative fallback indicator;
- confidence/coverage metadata.

### Rule

Test-impact analysis is an optimization, not proof.

~~~text
Affected-test selection
!=
Verification authority
~~~

The resulting selection is frozen into Resolved Verification Execution Plan.

If the selector cannot prove safe reduction, policy falls back to broader/full required coverage.

---

## 7. Selection must preserve negative space

A major audit requirement:

Do not store only what was selected.

Store:
- selected;
- filtered;
- omitted;
- skipped;
- quarantined;
- reason;
- rule/selector version.

This makes "why did we not run test X?" answerable later.

---

## 8. CycloneDX / SPDX — BOM Artifact family

Repositories:
- https://github.com/CycloneDX/specification
- https://github.com/spdx/spdx-spec

CycloneDX supports:
- SBOM;
- SaaSBOM;
- HBOM;
- ML-BOM;
- CBOM;
- MBOM;
- OBOM;
- VDR/VEX;
- attestations.

SPDX provides a mature system/software component and license/compliance model.

### Optimization

Generalize supply-chain metadata into:

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

Preferred formats:
- CycloneDX where its broader BOM family fits;
- SPDX where ecosystem/license/compliance needs fit.

For embedded products, Target/Release may reference:
- software BOM;
- hardware BOM;
- model BOM;
- crypto BOM

as separate immutable artifacts.

They remain descriptive evidence/context, not Release authority by themselves.

---

## 9. Supply-chain analysis pipeline — ORT practice

Repository:
- https://github.com/oss-review-toolkit/ort

ORT separates:
- Analyze;
- Download;
- Scan;
- Advise;
- Evaluate;
- Report.

### Absorb

SupplyChainIntel should preserve stage provenance:

~~~text
Inventory
 -> Scan
 -> Advisory enrichment
 -> Policy evaluation
 -> Finding/Report
~~~

A scanner output and a policy violation are different records.

This matches our:
- raw facts;
- evaluation;
- risk/decision separation.

Do not make ORT a core dependency.

---

## 10. Security/static analysis

CodeQL/Semgrep and similar tools can produce normalized:

~~~text
Finding
  rule
  severity
  location
  source_tree_digest
  tool/version
  raw_report_ref
~~~

If run in an approved trusted Procedure, a Finding Set can become Verification Evidence.

Otherwise it is an Engineering Check/security signal.

CodeQL licensing/CLI constraints must be reviewed separately for private-code usage.

---

## 11. Testcontainers — M0/M1 test infrastructure

Repositories:
- https://github.com/testcontainers/testcontainers-go
- related Testcontainers projects

### Strong practical optimization

Use Testcontainers or equivalent disposable dependencies in integration tests for:
- PostgreSQL;
- MinIO/S3-compatible store;
- OPA;
- Toxiproxy;
- selected mocks/proxies.

Temporal can use an appropriate test/dev environment.

Benefit:
- repeatable local/CI integration tests;
- automatic cleanup;
- less shared mutable test infrastructure.

This is a development/test dependency, not runtime architecture.

---

## 12. Automated dependency maintenance — Renovate

Repository:
- https://github.com/renovatebot/renovate

Renovate can discover/update dependencies and create PRs with update metadata.

Future use:
- create Maintenance Work/Requirement from dependency update PR;
- route through the same Integration/Verification/Release authority chain.

Automated dependency PRs never bypass normal engineering gates.

Not an M1 dependency.

---

## 13. Test verdict display/reporting

Human-facing dashboards may use:
- Allure;
- Testkube;
- custom UI;
- future ResultDB-style UI.

But the canonical TestVerdict remains in engineering-platform.

Presentation tools are projections.

---

## 14. M0/M1 impact

### M0
Freeze:
- TestReportArtifact schema;
- VerificationSelectionDecision schema;
- BOMArtifact schema;
- normalized Finding schema.

### M1
Use Testcontainers where practical for infrastructure integration tests.

M1 test results may still come directly from CI structured outputs.
Full report normalization is M2-oriented.

### M2
Implement:
- report adapters;
- local Result Sink;
- TestVariant/Attempt/Verdict;
- BOM generation where useful;
- affected-test selection only after full coverage baseline exists.

---

## 15. New invariants

1. **Framework report format is not the canonical TestAttempt model.**
2. **Test selection is an optimization and must preserve omitted coverage/reasons.**
3. **Selector output is frozen into the Resolved Verification Execution Plan.**
4. **If safe reduction is uncertain, policy expands coverage rather than guessing.**
5. **BOMs are immutable descriptive artifacts, not Release authority.**
6. **Scanner facts and policy evaluation are distinct records.**
7. **Reporting dashboards are projections, never result authority.**

---

## 16. Conclusion

Round 7 mainly improves interoperability and auditability:
- CTRF-compatible test-report ingestion without domain lock-in;
- explicit change-impact selection decisions;
- CycloneDX/SPDX BOM-family support;
- repeatable M0/M1 infrastructure tests using disposable environments.

No new production M1 service is required.
