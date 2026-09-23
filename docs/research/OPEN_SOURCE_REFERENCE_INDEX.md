# Open Source Reference Research Index

Date: 2026-09-24
Status: **Current research index**
Architecture:
- docs/architecture/AI_NATIVE_ENGINEERING_PLATFORM_V1_2_FINAL.md
Implementation:
- docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V21.md
Execution:
- docs/roadmap/M0_REFERENCE_ADOPTION_PLAN_V21.md

## 1. Research rounds

| Round | Focus | Main references | Main absorbed result |
|---|---|---|---|
| 1 | Agent/runtime/workspace/workflow/device/OTA | OpenHands, Coder, SWE-ReX, Temporal, in-toto, Sigstore, labgrid, RAUC | Own authority, reuse mechanisms |
| 2 | Policy/identity/secrets/supply-chain/device tests | OPA, Cedar, SPIRE, OpenBao, Tekton Chains, BuildKit, GUAC, OpenHTF, MCUboot, Toxiproxy | External policy/identity; independent Attestation Controller; Device/Test split |
| 3 | Session/artifact/distribution/admission | Teleport, Boundary, OCI/ORAS, Harbor, TUF, CloudEvents, step-ca, Argo/Flux | JIT Session Grant; digest != locator; Release Admission; desired/actual reconciliation |
| 4 | Large-scale CI/test/device scheduling | Prow, Zuul, LUCI, Boskos, Twister, Tradefed, Yocto | exact candidate sets; trusted scheduler attributes; attempt/verdict split; resolved test plan |
| 5 | Worker execution/build reproducibility | Taskcluster, Gerrit, Nix, OpenBMC, Robot Framework | ExecutionSpec; Build Definition before Receipt; explicit approval carry-forward; FFDC |
| 6 | Code intelligence/observability | SCIP, Kythe, Tree-sitter, OTel GenAI, CDEvents | CodeIntelligenceArtifact; official GenAI telemetry; standard external CI/CD events |
| 7 | Test interchange/quality/change impact | CTRF, Allure, Testkube, CodeQL, Nx, ORT, CycloneDX/SPDX | TestReportArtifact; auditable selection; BOMArtifact; Finding normalization |
| 8 | Requirements traceability | Capra, StrictDoc, Doorstop, OpenFastTrace, ReqIF concepts | TraceLink; trace matrix/coverage projection; explicit impact traversal |
| 9 | MBSE/high assurance | SysML v2, OpenSysML, Capella, Resolute, SACM/GSN | ModelArtifact; optional AssuranceCaseArtifact for A3/A4 |
| 10 | Simulation/emulation/HIL fidelity | Renode, QEMU, Gazebo, Webots, syzkaller | VerificationEnvironmentClass; explicit environment equivalence; fidelity ladder |
| 11 | Engineering metrics / progressive delivery / AI governance | DevLake, OpenSLO, OpenFeature, Argo Rollouts, ToolHive, Docker MCP Gateway, Promptfoo | derived MetricDefinition; PromotionPlan; ToolProfile; RuntimeQualificationProfile |
| 12 | Incident / regression / knowledge closure | Keep, Sentry, ClusterFuzz, mozregression, rustc-perf, Rundeck | Incident; ReproductionCase; bisection; FixVerification; curated KnowledgeCandidate |
| 13 | ML experiment / dataset / model lineage | MLflow, MLMD, Kubeflow, ClearML, Metaflow, W&B, CML | DatasetArtifact; ExperimentDefinition; MLModelArtifact; CalibrationArtifact; qualification lineage |
| 14 | Device identity / provisioning / attestation | Keylime, TPM2, TF-M/PSA, MCUboot, wolfBoot, FDO, Caliptra | DeviceProvisioningReceipt; DeviceTrustProfile; Boot/Attestation Evidence |
| 15 | Formal methods / model checking | TLA+, Alloy, CBMC, Kani, Frama-C, VeriFast | FormalSpecificationArtifact; formal Evidence; model-check platform invariants |
| 16 | Interface contract compatibility | Buf, Pact, AsyncAPI, Schemathesis, Protovalidate | multidimensional compatibility; ConsumerContract; breaking-change Evidence |
| 17 | Configuration / constraints / resolved config | JSON Schema, CEL, CUE, KCL, HCL/Jsonnet | ResolvedConfigurationArtifact; layered schema/constraint/policy validation |
| 18 | Manufacturing traceability / fleet rollout | BaSyx, OpenTAP, OPC UA/open62541, ERPNext, hawkBit, Mender | trusted Station/Recipe/Result receipts; per-device rollout; SystemUpdateManifest |
| 19 | Knowledge freshness / revalidation | DataHub, OpenMetadata, Backstage concepts | KnowledgeItem lifecycle; freshness policy; validation receipts; scoped trusted retrieval |
| 20 | AI code review quality / independence | PR-Agent, reviewdog, Code Review Bench, AACR-Bench, Google review guidance | ReviewExecution/Finding/Resolution; reviewer qualification; independence and replay corpus |
| 21 | Empirical AI coding governance | DORA, METR, OpenSSF public guidance | AIUsagePolicy; RuntimeInstructionProfile; paired metrics; versioned measurement studies |

---

## 2. Current M1 hard dependencies

Use now:
- PostgreSQL
- Temporal
- OPA
- S3/MinIO-compatible ArtifactStore
- OpenTelemetry
- Toxiproxy for tests
- Git/CI provider
- Codex
- native Ubuntu execution
- existing enterprise PKI or lightweight short-lived mTLS mechanism

Optional M1:
- step-ca if enterprise PKI is unavailable
- dev/test AttestationSigner

---

## 3. M2 likely integrations

- labgrid — hardware resource/control
- pytest / pytest-embedded / OpenHTF patterns — procedure execution
- production AttestationSigner — Sigstore/cosign/KMS/PKI
- Syft/OSV or Trivy — BOM/security findings
- RAUC or project-specific ReleaseProvider
- Renode/QEMU/simulation where target/procedure benefits

---

## 4. Scale/high-assurance triggers

Introduce only when justified:
- SPIRE — many workers/services/sites and workload-identity scale
- OpenBao — new self-hosted secret backend needed
- Coder — remote/cloud workspace provisioning
- Harbor/OCI registry — replication/RBAC/scanning/registry operations
- GUAC — supply-chain relationship query scale
- LAVA — large shared physical-device farm
- Remote Execution API — distributed build scale
- Mender/hawkBit — fleet rollout
- TUF/Uptane — high-assurance distribution/update trust
- AssuranceCaseArtifact — A3/A4/regulatory assurance
- SysML/Capella integration — model-heavy systems engineering
- speculative multi-repo gate queue — integration scale

---

## 5. Important standards/interoperability choices

Prefer:
- in-toto/SLSA-compatible attestations
- Sigstore/cosign or Notation/KMS for signing as appropriate
- OCI Distribution/ORAS for selected distributable artifacts
- CycloneDX/SPDX BOM artifacts
- SCIP for portable semantic code index where available
- OpenTelemetry official GenAI/MCP semantic conventions
- CloudEvents + CDEvents for external CI/CD event projection
- ReqIF for future requirement interchange
- GSN/SACM export only for future high-assurance use
- OpenFeature-style vendor-neutral flag evaluation where runtime flags are used
- Buf-style breaking checks for Protobuf; OpenAPI/AsyncAPI/Pact where interface kind fits
- JSON Schema/native typed schema for structure; CEL-style bounded local constraints where useful
- ML lineage mapped into common Run/Artifact/Evidence semantics rather than a parallel authority model
- AAS/OPC UA/OpenTAP only as manufacturing interoperability/execution mechanisms where useful
- staged OTA/fleet systems modeled through per-device attempts and explicit cohort semantics
- knowledge/context catalogs treated as governed projections around KnowledgeItem lifecycle
- AI code review tools qualified through benchmark + project failure replay, not trusted by brand

Do not let any standard become the mutable engineering business authority.

---

## 6. Core architecture decisions reinforced by research

1. Control Plane owns formal engineering state.
2. Runtime/agent is a replaceable proposer/executor, not authority.
3. Artifact identity is digest; path/tag/URL is locator.
4. Evidence is immutable and issuer-bound.
5. Verification is separate from Review/Decision/Approval.
6. Approval is separate from last-moment Release Admission.
7. external side effects are reconciled against observed reality.
8. resource allocation and health are separate.
9. test attempt/result/verdict/exoneration are separate.
10. declared Verification Plan and resolved execution plan are separate.
11. test-selection optimization preserves omitted coverage/reasons.
12. semantic TraceLinks are explicit, typed and provenance-aware.
13. generated code intelligence and telemetry remain context/observability, not authority.
14. simulation evidence records fidelity and does not implicitly replace HIL/physical proof.
15. high-assurance argumentation is optional and evidence-citing, not evidence-generating.
16. progressive promotion and runtime configuration are exact-subject concerns, not mutable side notes.
17. MCP/tool discovery never implies tool authority; Formal Runs bind exact ToolProfile.
18. operational incidents feed Reproduction/FixVerification/Postmortem before curated Knowledge.
19. datasets/models/calibration reuse the same Artifact/Evidence/Verification authority model.
20. physical Device identity/provisioning/boot/attestation trust are separate from Target and Worker identity.
21. formal verification is scoped Evidence with explicit assumptions/bounds, never automatic release authority.
22. interface compatibility is multidimensional and exact-revision bound.
23. configuration authoring sources are distinct from resolved canonical configuration used for execution.
24. derived analytics, external registries and dashboards remain projections rather than hidden sources of truth.
25. manufacturing stations/recipes/results connect Release identity to each physical serial without turning the platform into an MES.
26. fleet promotion is per-device, stage-based and reconciliation-driven; abort is not assumed atomic.
27. durable Knowledge is scoped, provenance-backed, freshness-managed and revalidated.
28. AI review produces Findings under explicit reviewer identity/qualification; it does not self-approve.
29. AI usage policy and instructions are versioned, but instructions never replace capability enforcement.
30. AI productivity claims are local/time/profile scoped and paired with quality/assurance outcomes.

---

## 7. Research posture

Further research should only alter the canonical implementation profile when it:
- closes a real semantic gap;
- removes meaningful bespoke infrastructure;
- establishes a durable interoperability standard;
- identifies a failure mode not already represented.

A popular tool alone is not a reason to add a new dependency or plugin interface.

Current baseline is sufficiently mature to proceed with M0 implementation while targeted research continues in parallel with real spikes.
