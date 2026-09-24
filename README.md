# engineering-platform

AI-native Engineering Control Plane for formal R&D execution.

## Positioning

This repository is the canonical implementation and specification home for an engineering platform that separates:

- **WorkBuddy** — corporate R&D front door
- **Engineering Control Plane** — workflow/state authority
- **Ubuntu Engineering Workers** — execution plane
- **Codex / Claude / future agents** — replaceable interactive runtime providers
- **Git / CI / Device / HIL** — engineering fact sources
- **Artifact / Evidence / Verification / Review / Release** — assurance chain
- **Human Authority** — production and irreversible decisions

The platform is intentionally **not** a WorkBuddy-to-Codex proxy, prompt platform, or multi-agent orchestration demo.

## Architecture baseline

See:

- [AI Native Engineering Platform v1.2 — FINAL / canonical](docs/architecture/AI_NATIVE_ENGINEERING_PLATFORM_V1_2_FINAL.md)
- [Reference-Aligned Implementation Profile v39 — current canonical implementation companion](docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V39.md)
- [Reference-Aligned Implementation Profile v38 — historical](docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V38.md)
- [Reference-Aligned Implementation Profile v36 — historical](docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V36.md)
- [Reference-Aligned Implementation Profile v35 — historical](docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V35.md)
- [Reference-Aligned Implementation Profile v29 — historical](docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V29.md)
- [Reference-Aligned Implementation Profile v25 — historical](docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V25.md)
- [Reference-Aligned Implementation Profile v21 — historical](docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V21.md)
- [Reference-Aligned Implementation Profile v20 — historical](docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V20.md)
- [Reference-Aligned Implementation Profile v17 — historical](docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V17.md)
- [Reference-Aligned Implementation Profile v16 — historical](docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V16.md)
- [Reference-Aligned Implementation Profile v15 — historical](docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V15.md)
- [Reference-Aligned Implementation Profile v14 — historical](docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V14.md)
- [Reference-Aligned Implementation Profile v13 — historical](docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V13.md)
- [Reference-Aligned Implementation Profile v12 — historical](docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V12.md)
- [Reference-Aligned Implementation Profile v11 — historical](docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V11.md)
- [Reference-Aligned Implementation Profile v10 — historical](docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V10.md)
- [Reference-Aligned Implementation Profile v9 — historical](docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V9.md)
- [Reference-Aligned Implementation Profile v8 — historical](docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V8.md)
- [Reference-Aligned Implementation Profile v7 — historical](docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V7.md)
- [Reference-Aligned Implementation Profile v6 — historical](docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V6.md)
- [Reference-Aligned Implementation Profile v5 — historical](docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V5.md)
- [Reference-Aligned Implementation Profile v4 — historical](docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V4.md)
- [Reference-Aligned Implementation Profile v3 — historical](docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V3.md)
- [Reference-Aligned Implementation Profile v2 — historical](docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V2.md)
- [Reference-Aligned Implementation Profile v1 — historical](docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V1.md)
- [Open Source Reference Review Round 1 — archived research](docs/research/OPEN_SOURCE_REFERENCE_REVIEW_2026-09-23.md)
- [Open Source Reference Review Round 2 — policy, identity, supply chain, embedded test](docs/research/OPEN_SOURCE_REFERENCE_REVIEW_ROUND2_2026-09-23.md)
- [Open Source Reference Review Round 3 — session, artifact, admission, distribution trust](docs/research/OPEN_SOURCE_REFERENCE_REVIEW_ROUND3_2026-09-23.md)
- [Open Source Reference Review Round 4 — large-scale engineering](docs/research/OPEN_SOURCE_REFERENCE_REVIEW_ROUND4_LARGE_SCALE_ENGINEERING_2026-09-23.md)
- [Open Source Reference Review Round 5 — task execution and reproducibility](docs/research/OPEN_SOURCE_REFERENCE_REVIEW_ROUND5_TASK_EXECUTION_REPRODUCIBILITY_2026-09-23.md)
- [Open Source Reference Review Round 6 — code intelligence and observability](docs/research/OPEN_SOURCE_REFERENCE_REVIEW_ROUND6_CODE_INTELLIGENCE_OBSERVABILITY_2026-09-23.md)
- [Open Source Reference Review Round 7 — test quality and change impact](docs/research/OPEN_SOURCE_REFERENCE_REVIEW_ROUND7_TEST_QUALITY_IMPACT_2026-09-23.md)
- [Open Source Reference Review Round 8 — requirements traceability](docs/research/OPEN_SOURCE_REFERENCE_REVIEW_ROUND8_REQUIREMENTS_TRACEABILITY_2026-09-23.md)
- [Open Source Reference Review Round 9 — MBSE and assurance case](docs/research/OPEN_SOURCE_REFERENCE_REVIEW_ROUND9_MBSE_ASSURANCE_CASE_2026-09-23.md)
- [Open Source Reference Review Round 10 — simulation and verification fidelity](docs/research/OPEN_SOURCE_REFERENCE_REVIEW_ROUND10_SIMULATION_VERIFICATION_FIDELITY_2026-09-23.md)
- [Open Source Reference Review Round 11 — metrics, progressive delivery, AI governance](docs/research/OPEN_SOURCE_REFERENCE_REVIEW_ROUND11_METRICS_PROGRESSIVE_AI_GOVERNANCE_2026-09-23.md)
- [Open Source Reference Review Round 12 — incident, regression, knowledge closure](docs/research/OPEN_SOURCE_REFERENCE_REVIEW_ROUND12_INCIDENT_REGRESSION_KNOWLEDGE_2026-09-23.md)
- [Open Source Reference Review Round 13 — ML experiment/model lineage](docs/research/OPEN_SOURCE_REFERENCE_REVIEW_ROUND13_ML_EXPERIMENT_MODEL_LINEAGE_2026-09-23.md)
- [Open Source Reference Review Round 14 — device identity/provisioning/attestation](docs/research/OPEN_SOURCE_REFERENCE_REVIEW_ROUND14_DEVICE_IDENTITY_PROVISIONING_ATTESTATION_2026-09-23.md)
- [Open Source Reference Review Round 15 — formal methods/model checking](docs/research/OPEN_SOURCE_REFERENCE_REVIEW_ROUND15_FORMAL_METHODS_MODEL_CHECKING_2026-09-23.md)
- [Open Source Reference Review Round 16 — interface contract compatibility](docs/research/OPEN_SOURCE_REFERENCE_REVIEW_ROUND16_INTERFACE_CONTRACT_COMPATIBILITY_2026-09-23.md)
- [Open Source Reference Review Round 17 — configuration and constraints](docs/research/OPEN_SOURCE_REFERENCE_REVIEW_ROUND17_CONFIGURATION_CONSTRAINTS_2026-09-23.md)
- [Open Source Reference Synthesis Rounds 11–17](docs/research/OPEN_SOURCE_REFERENCE_SYNTHESIS_ROUNDS11_17_2026-09-24.md)
- [Open Source Reference Review Round 18 — manufacturing and fleet](docs/research/OPEN_SOURCE_REFERENCE_REVIEW_ROUND18_MANUFACTURING_FLEET_2026-09-24.md)
- [Open Source Reference Review Round 19 — knowledge freshness](docs/research/OPEN_SOURCE_REFERENCE_REVIEW_ROUND19_KNOWLEDGE_FRESHNESS_2026-09-24.md)
- [Open Source Reference Review Round 20 — AI code review](docs/research/OPEN_SOURCE_REFERENCE_REVIEW_ROUND20_AI_CODE_REVIEW_2026-09-24.md)
- [Open Source/Public Practice Review Round 21 — empirical AI governance](docs/research/OPEN_SOURCE_REFERENCE_REVIEW_ROUND21_EMPIRICAL_AI_GOVERNANCE_2026-09-24.md)
- [Open Source Reference Review Round 22 — PLM, hardware design and engineering change](docs/research/OPEN_SOURCE_REFERENCE_REVIEW_ROUND22_PLM_HARDWARE_CHANGE_2026-09-24.md)
- [Open Source / Regulatory Reference Review Round 23 — PSIRT, CRA and vulnerability response](docs/research/OPEN_SOURCE_REFERENCE_REVIEW_ROUND23_PSIRT_CRA_VULNERABILITY_RESPONSE_2026-09-24.md)
- [Open Source Reference Review Round 24 — RMA, repair and failure analysis](docs/research/OPEN_SOURCE_REFERENCE_REVIEW_ROUND24_RMA_REPAIR_FAILURE_ANALYSIS_2026-09-24.md)
- [Open Source Reference Review Round 25 — product family and variants](docs/research/OPEN_SOURCE_REFERENCE_REVIEW_ROUND25_PRODUCT_FAMILY_VARIANTS_2026-09-24.md)
- [Open Source Reference Review Round 26 — supplier lifecycle, CAPA and compliance](docs/research/OPEN_SOURCE_REFERENCE_REVIEW_ROUND26_SUPPLIER_CAPA_COMPLIANCE_2026-09-24.md)
- [Open Source Reference Review Round 27 — machine-readable compliance / OSCAL](docs/research/OPEN_SOURCE_REFERENCE_REVIEW_ROUND27_COMPLIANCE_OSCAL_2026-09-24.md)
- [Open Source Reference Review Round 28 — digital twin and observed device state](docs/research/OPEN_SOURCE_REFERENCE_REVIEW_ROUND28_DIGITAL_TWIN_DEVICE_STATE_2026-09-24.md)
- [Open Source Reference Review Round 29 — FOSS / license compliance](docs/research/OPEN_SOURCE_REFERENCE_REVIEW_ROUND29_FOSS_LICENSE_COMPLIANCE_2026-09-24.md)
- [Open Source Reference Review Round 30 — reliability/FMEA/FRACAS negative result](docs/research/OPEN_SOURCE_REFERENCE_REVIEW_ROUND30_RELIABILITY_NEGATIVE_RESULT_2026-09-24.md)
- [Open Source / Regulatory Reference Review Round 31 — certification and battery safety](docs/research/OPEN_SOURCE_REFERENCE_REVIEW_ROUND31_CERTIFICATION_BATTERY_SAFETY_2026-09-24.md)
- [Open Source / Public Practice Review Round 32 — privacy and data lifecycle](docs/research/OPEN_SOURCE_REFERENCE_REVIEW_ROUND32_PRIVACY_DATA_LIFECYCLE_2026-09-24.md)
- [Open Source Reference Review Round 33 — field anomaly and prognostics](docs/research/OPEN_SOURCE_REFERENCE_REVIEW_ROUND33_FIELD_PROGNOSTICS_2026-09-24.md)
- [Public / Open Source Reference Review Round 34 — long-term compatibility](docs/research/OPEN_SOURCE_REFERENCE_REVIEW_ROUND34_LONG_TERM_COMPATIBILITY_2026-09-24.md)
- [Open Source Reference Review Round 35 — threat modeling](docs/research/OPEN_SOURCE_REFERENCE_REVIEW_ROUND35_THREAT_MODELING_2026-09-24.md)
- [Open Source Reference Review Round 36 — audit integrity and disaster recovery](docs/research/OPEN_SOURCE_REFERENCE_REVIEW_ROUND36_AUDIT_DISASTER_RECOVERY_2026-09-24.md)
- [Open Source Reference Review Round 37 — laboratory samples and supply-chain traceability](docs/research/OPEN_SOURCE_REFERENCE_REVIEW_ROUND37_LAB_SAMPLE_SUPPLY_CHAIN_TRACE_2026-09-24.md)
- [Public / Regulatory Reference Review Round 38 — Digital Product Passport](docs/research/OPEN_SOURCE_REFERENCE_REVIEW_ROUND38_DIGITAL_PRODUCT_PASSPORT_2026-09-24.md)
- [Public Standard Reference Review Round 39 — digital calibration and metrology](docs/research/OPEN_SOURCE_REFERENCE_REVIEW_ROUND39_DIGITAL_CALIBRATION_METROLOGY_2026-09-24.md)
- [Open Source Research Index](docs/research/OPEN_SOURCE_REFERENCE_INDEX.md)
- [Open Source Reference Synthesis — optimization decisions](docs/research/OPEN_SOURCE_REFERENCE_SYNTHESIS_OPTIMIZATION_2026-09-23.md)
- [AI Native Engineering Platform v1.1 — historical baseline](docs/architecture/AI_NATIVE_ENGINEERING_PLATFORM_V1_1.md)
- [AI Native Engineering Platform v1 — historical baseline](docs/architecture/AI_NATIVE_ENGINEERING_PLATFORM_V1.md)
- [Architecture Review Round 2](docs/reviews/ARCHITECTURE_REVIEW_ROUND2_2026-09-23.md)
- [Architecture Review Round 3 — implementation readiness](docs/reviews/ARCHITECTURE_REVIEW_ROUND3_IMPLEMENTATION_READINESS_2026-09-23.md)
- [Architecture Review Round 4 — trust, determinism and recovery](docs/reviews/ARCHITECTURE_REVIEW_ROUND4_TRUST_DETERMINISM_RECOVERY_2026-09-23.md)
- [Architecture Review Round 5 — embedded configuration and release integrity](docs/reviews/ARCHITECTURE_REVIEW_ROUND5_EMBEDDED_CONFIGURATION_RELEASE_2026-09-23.md)
- [Architecture Review Round 6 — threat, failure and schema closure](docs/reviews/ARCHITECTURE_REVIEW_ROUND6_THREAT_FAILURE_SCHEMA_2026-09-23.md)
- [Architecture Review Round 7 — integration and release transforms](docs/reviews/ARCHITECTURE_REVIEW_ROUND7_INTEGRATION_RELEASE_TRANSFORMS_2026-09-23.md)
- [Architecture Review Round 8 — readiness, risk and lifecycle](docs/reviews/ARCHITECTURE_REVIEW_ROUND8_READINESS_RISK_LIFECYCLE_2026-09-23.md)
- [Architecture Review Round 9 — external security cross-check](docs/reviews/ARCHITECTURE_REVIEW_ROUND9_EXTERNAL_SECURITY_CROSSCHECK_2026-09-23.md)
- [Open architecture findings — authoritative pre-v1.2 backlog](docs/reviews/OPEN_ARCHITECTURE_FINDINGS.md)

## Core invariants

1. Requirement is versioned.
2. Task Contract is versioned.
3. Every Formal Run has identity.
4. Every Runtime restart has Attempt identity.
5. Every Formal Run has a frozen Base SHA.
6. Artifact is immutable and content-addressed.
7. Evidence binds an exact Artifact / Subject.
8. Subject change invalidates affected Evidence.
9. Runtime claims are not Verification.
10. Permission is capability-based.
11. Production requires Human Authority.
12. Every Release traces to Requirement, Source, Artifact, Evidence, Review and Decision.

## Delivery strategy

Build a narrow vertical slice first. Do not start with multi-agent planning, skill marketplaces, or knowledge ingestion.

M0 freezes domain contracts and invariants. M1 proves a single WorkBuddy → Control Plane → Ubuntu → Codex → Git/CI → Artifact/Evidence → Closure path. Device/HIL, multiple runtimes, planner/skills, and knowledge follow only after the core loop is reliable.

## Implementation strategy

**Own authority, reuse mechanisms.**

The platform keeps Requirement/Run/Target/Evidence/Verification/Release authority in engineering-platform, while selectively reusing mature implementations behind replaceable adapters:

- Temporal — durable workflow (M1 required)
- OPA — authorization evaluator (M1 default)
- S3/MinIO-compatible store — Artifact/Evidence bytes (M1 required)
- OpenTelemetry — observability (M1 required)
- Toxiproxy — deterministic failure tests (M0/M1 required)
- in-toto/SLSA/Sigstore — provenance/attestation alignment
- OpenHands SDK / SWE-ReX / Cline — Runtime/Session design references
- labgrid + pytest/OpenHTF — M2 Device/HIL and procedure candidates
- SPIRE/OpenBao/Coder/Dagger/RAUC — milestone- or scale-triggered options

External component state never becomes hidden engineering authority.

## Current M0 execution plan

- [M0 Reference Adoption Plan v39 — current](docs/roadmap/M0_REFERENCE_ADOPTION_PLAN_V39.md)
- [M0 Reference Adoption Plan v38 — historical](docs/roadmap/M0_REFERENCE_ADOPTION_PLAN_V38.md)
- [M0 Reference Adoption Plan v36 — historical](docs/roadmap/M0_REFERENCE_ADOPTION_PLAN_V36.md)
- [M0 Reference Adoption Plan v35 — historical](docs/roadmap/M0_REFERENCE_ADOPTION_PLAN_V35.md)
- [M0 Reference Adoption Plan v29 — historical](docs/roadmap/M0_REFERENCE_ADOPTION_PLAN_V29.md)
- [M0 Reference Adoption Plan v25 — historical](docs/roadmap/M0_REFERENCE_ADOPTION_PLAN_V25.md)
- [M0 Reference Adoption Plan v21 — historical](docs/roadmap/M0_REFERENCE_ADOPTION_PLAN_V21.md)
- [M0 Reference Adoption Plan v20 — historical](docs/roadmap/M0_REFERENCE_ADOPTION_PLAN_V20.md)
- [M0 Reference Adoption Plan v17 — historical](docs/roadmap/M0_REFERENCE_ADOPTION_PLAN_V17.md)
- [M0 Reference Adoption Plan v16 — historical](docs/roadmap/M0_REFERENCE_ADOPTION_PLAN_V16.md)
- [M0 Reference Adoption Plan v15 — historical](docs/roadmap/M0_REFERENCE_ADOPTION_PLAN_V15.md)
- [M0 Reference Adoption Plan v14 — historical](docs/roadmap/M0_REFERENCE_ADOPTION_PLAN_V14.md)
- [M0 Reference Adoption Plan v13 — historical](docs/roadmap/M0_REFERENCE_ADOPTION_PLAN_V13.md)
- [M0 Reference Adoption Plan v12 — historical](docs/roadmap/M0_REFERENCE_ADOPTION_PLAN_V12.md)
- [M0 Reference Adoption Plan v11 — historical](docs/roadmap/M0_REFERENCE_ADOPTION_PLAN_V11.md)
- [M0 Reference Adoption Plan v10 — historical](docs/roadmap/M0_REFERENCE_ADOPTION_PLAN_V10.md)
- [M0 Reference Adoption Plan v9 — historical](docs/roadmap/M0_REFERENCE_ADOPTION_PLAN_V9.md)
- [M0 Reference Adoption Plan v8 — historical](docs/roadmap/M0_REFERENCE_ADOPTION_PLAN_V8.md)
- [M0 Reference Adoption Plan v7 — historical](docs/roadmap/M0_REFERENCE_ADOPTION_PLAN_V7.md)
- [M0 Reference Adoption Plan v6 — historical](docs/roadmap/M0_REFERENCE_ADOPTION_PLAN_V6.md)
- [M0 Reference Adoption Plan v5 — historical](docs/roadmap/M0_REFERENCE_ADOPTION_PLAN_V5.md)
- [M0 Reference Adoption Plan v4 — historical](docs/roadmap/M0_REFERENCE_ADOPTION_PLAN_V4.md)
- [M0 Reference Adoption Plan v3 — historical](docs/roadmap/M0_REFERENCE_ADOPTION_PLAN_V3.md)
- [M0 Reference Adoption Plan v2 — historical](docs/roadmap/M0_REFERENCE_ADOPTION_PLAN_V2.md)
