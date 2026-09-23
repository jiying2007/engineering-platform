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
- [Reference-Aligned Implementation Profile v3 — current canonical implementation companion](docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V3.md)
- [Reference-Aligned Implementation Profile v2 — historical](docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V2.md)
- [Reference-Aligned Implementation Profile v1 — historical](docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V1.md)
- [Open Source Reference Review Round 1 — archived research](docs/research/OPEN_SOURCE_REFERENCE_REVIEW_2026-09-23.md)
- [Open Source Reference Review Round 2 — policy, identity, supply chain, embedded test](docs/research/OPEN_SOURCE_REFERENCE_REVIEW_ROUND2_2026-09-23.md)
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

- [M0 Reference Adoption Plan v3 — current](docs/roadmap/M0_REFERENCE_ADOPTION_PLAN_V3.md)
- [M0 Reference Adoption Plan v2 — historical](docs/roadmap/M0_REFERENCE_ADOPTION_PLAN_V2.md)
