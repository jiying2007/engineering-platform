# M0 Reference Adoption Plan v8

Date: 2026-09-23
Status: **Current execution plan**
Supersedes: docs/roadmap/M0_REFERENCE_ADOPTION_PLAN_V7.md

Parents:
- docs/architecture/AI_NATIVE_ENGINEERING_PLATFORM_V1_2_FINAL.md
- docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V8.md

## 1. Principle

M0 continues to freeze semantics before broad infrastructure.

Round 7 adds schemas for report interchange, verification selection and BOM/finding metadata, but no new mandatory runtime service.

---

## 2. Core schema additions

Freeze:
1. TestReportArtifact.
2. ReportConversionReceipt.
3. VerificationSelectionDecision.
4. BOMArtifact.
5. Finding.
6. existing TestVariant/TestAttempt/TestVerdict/Exoneration schemas.
7. Resolved Verification Execution Plan negative-space fields.

---

## 3. Report normalization fixtures

Create fixtures for at least:
- simple JUnit-style report;
- one CTRF-compatible report;
- malformed/unknown report.

Prove:
- original report remains immutable;
- conversion has explicit provenance;
- unknown result does not silently disappear;
- normalized attempt IDs remain stable within declared identity rules.

Full report service is not required.

---

## 4. Verification selection fixtures

Create one safe/no-impact example and one ambiguous-impact example.

Safe example:
- docs-only change;
- policy permits reduced checks;
- omitted checks retain explicit reasons.

Ambiguous example:
- source/interface change;
- selector cannot prove bounded impact;
- policy expands to required/full plan.

Prove:
- selection cannot hide required checks;
- selector/version/digest is frozen.

---

## 5. BOM fixture

Generate/store one standard BOM Artifact for a representative build.

Preferred:
- CycloneDX or SPDX.

Prove:
- exact Artifact/Subject binding;
- BOM digest immutable;
- BOM is consumed as supply-chain fact, not Release approval.

No scanner service required in M0.

---

## 6. Finding fixture

Normalize one static/security finding into platform schema.

Prove:
- tool/raw report preserved;
- Finding != Risk Acceptance;
- applicability/status can change without rewriting raw finding.

---

## 7. Disposable integration test infrastructure

Where practical, use Testcontainers-style isolated dependencies for:
- PostgreSQL;
- MinIO/object store;
- OPA;
- Toxiproxy;
- provider mocks.

Acceptance:
- integration suite can run locally and in CI without persistent shared state;
- cleanup is automatic;
- tests can inject deterministic network faults.

---

## 8. Existing M0 path remains

Still required:
- canonicalization;
- Run Input / ExecutionSpec / Run Receipt;
- Build Definition / Build Receipt;
- Integration Subject / Eligibility;
- Session Grant/Gateway;
- Temporal/OPA/PostgreSQL;
- Attestation/Evidence/Verification;
- Release Admission/Reconciliation;
- OTel/CDEvents projection;
- failure injection.

---

## 9. M0 exit additions

M0 also requires:
- TestReportArtifact normalization fixture passes;
- VerificationSelectionDecision preserves omitted coverage;
- ambiguous impact expands coverage;
- one BOM Artifact is stored/bound to exact subject;
- one normalized Finding is retained distinctly from evaluation/risk.

M0 still does not require:
- Allure;
- Testkube;
- full CTRF adoption;
- CycloneDX/Spdx service;
- scanner platform;
- affected-test engine;
- GUAC.

---

## 10. M1 target

M1 formal engineering loop remains unchanged.

The new schemas ensure test/supply-chain integrations can grow later without breaking the authority model.
