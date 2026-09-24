# Open Source Reference Research Index

Date: 2026-09-24
Status: **Current research index**
Architecture:
- docs/architecture/AI_NATIVE_ENGINEERING_PLATFORM_V1_2_FINAL.md
Implementation:
- docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V39.md
Execution:
- docs/roadmap/M0_REFERENCE_ADOPTION_PLAN_V39.md

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
| 22 | PLM / hardware design / engineering change | Odoo PLM, InvenTree, Part-DB, KiCad, KiBot, LibrePCB | HardwareDesignArtifact; ProductStructureSnapshot; EngineeringChangePackage; EffectivityRule |
| 23 | PSIRT / vulnerability / CRA readiness | CVE, CSAF, OpenVEX, CRA guidance | ProductSecurityCase; applicability/VEX; advisory; support policy; RegulatoryNotificationCase |
| 24 | RMA / repair / failure analysis | ERPNext Warranty, Odoo Repairs | AS_RETURNED snapshots; FailureAnalysis; RepairActionReceipt; repair requalification |
| 25 | Product family / SKU / variants | FeatureIDE, Kconfig/Kconfiglib, ERPNext/Odoo variants | FeatureModelArtifact; ProductVariantDefinition; variant resolution/coverage/equivalence |
| 26 | Supplier lifecycle / CAPA / material compliance | InvenTree, ERPNext/Odoo Quality, CycloneDX HBOM | SupplierChangeNotice; alternate qualification; QualityCase/CAPA; material compliance |
| 27 | Machine-readable compliance | OSCAL, OpenControl | ComplianceProfile; control implementation; assessment results/package |
| 28 | Digital Twin / observed device state | Eclipse Ditto, Kanto, ThingsBoard | ObservedDeviceState; freshness-bound DeviceTwinProjection |
| 29 | FOSS/license compliance | FOSSology, ORT, ScanCode, SPDX, OpenChain | LicenseFinding/Policy/Assessment; notices/source offer; distribution profile |
| 30 | Reliability/FMEA/FRACAS landscape | targeted OSS search | negative result: no new core subsystem; reuse existing Verification/Incident/CAPA semantics |
| 31 | Product certification / battery safety | EU CE guidance, UNECE 38.3, PyBaMM | ConformityAssessmentCase; certification delta; exact battery qualification scope |
| 32 | Privacy / engineering data lifecycle | NIST Privacy Framework, OpenDP, Presidio/DataHub concepts | DataAssetProfile; ProcessingPurpose; retention/disposition/privacy transforms |
| 33 | Field anomaly / prognostics | NASA ProgPy, NAB, Digital Twin practices | TelemetryWindow; HealthStateEstimate; PrognosticEstimate; maintenance decision loop |
| 34 | Long-term compatibility / support | Kubernetes version skew, Android VINTF, K8s conformance | CompatibilityEnvelope; VersionSkewPolicy; ProductSupportPolicy; upgrade order |
| 35 | Threat modeling / security-by-design | OWASP Threat Dragon, pytm, ATT&CK, ASVS | ThreatModelArtifact; ThreatScenario; mitigation trace; threat-model freshness |
| 36 | Tamper-evident audit / disaster recovery | Rekor, Trillian/Tessera, pgBackRest, restic | AuditCheckpoint; RecoveryPoint/Plan; recovery epoch; external reconciliation |
| 37 | Laboratory sample / cross-enterprise traceability | SENAITE, Tractus-X Trace-X/IRS | SampleInstance/custody; lab import receipts; SupplyChainTraceEvent; PartGenealogyProjection |
| 38 | Digital Product Passport | EU DPP Registry/standards, Tractus-X | DigitalProductPassportArtifact; DPPProfile; projection/registration receipts |
| 39 | Digital calibration / metrology | PTB DCC | quantity/unit/uncertainty-aware MeasurementResult; DCC Artifact; measurement traceability |

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
- KiCad/KiBot-style deterministic hardware design export for reproducible manufacturing artifacts
- CSAF/VEX/CVE for security advisory/applicability interchange, with regulation-specific reporting kept separate
- OSCAL-compatible compliance export where control/assessment automation is useful
- SPDX/CycloneDX plus ORT/FOSSology/ScanCode for distribution-aware FOSS compliance
- Digital Twin systems treated as freshness-bound observed-state projections, not engineering authority
- product certification modeled as exact-scope conformity evidence rather than uploaded-document checklists
- privacy handled as a purpose/retention/transformation lifecycle over engineering data
- prognostics represented as uncertainty-bearing Evidence/Recommendation, not autonomous authority
- Kubernetes/Android-inspired compatibility envelopes and upgrade-order policy for multi-generation products
- OWASP-style threat models traced to normal Requirements/Evidence and freshness-reviewed
- tamper-evident checkpoints plus tested recovery/reconciliation instead of assuming backups roll back the external world
- laboratory sample identity/preparation/custody separated from product/device identity
- cross-enterprise trace data treated as provenance-rich observations and genealogy projections
- Digital Product Passport generated as an immutable external projection over lifecycle facts
- PTB DCC-inspired measurement semantics for units, uncertainty, instrument/calibration and influence conditions

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
31. hardware design/BOM revision and production effectivity are explicit and immutable.
32. PSIRT applicability, VEX/advisory, remediation and regulatory reporting are separate lifecycle concepts.
33. RMA preserves AS_RETURNED state before repair and appends technical service receipts.
34. SKU/feature intent, resolved variant, TargetRevision and DeviceInstance are distinct identities.
35. supplier PCN/EOL and alternate-part qualification flow through explicit impact/effectivity.
36. compliance assessment is regenerated from engineering Evidence rather than maintained as a parallel truth set.
37. Digital Twin is an observed-state projection with freshness/trust metadata.
38. FOSS license obligations are release/distribution-context specific and scanner facts do not self-authorize.
39. reliability/FMEA research does not justify a new platform subsystem until real program needs prove it.
40. certification claims bind exact product configuration, sample and standards edition and are re-assessed after material change.
41. sensitive engineering data use is purpose-scoped and retention/disposition actions are auditable.
42. predictive-maintenance outputs carry model/input identity and uncertainty and require normal Decisions.
43. long-term support is defined by compatibility envelope, directional skew and verified upgrade paths.
44. threat models are revision/freshness bound and feed standard Requirements/Verification.
45. audit append-only semantics are strengthened by signed/checkpointed integrity verification where risk requires it.
46. recovery restores records first and reconciles Git/CI/devices/releases/manufacturing before irreversible work resumes.
47. physical samples have independent identity, preparation and chain-of-custody.
48. cross-enterprise supplier/part traces are observations and genealogy inputs, not internal BOM authority.
49. Digital Product Passport is a versioned projection/export, never the mutable source of product facts.
50. formal measurements preserve quantity, unit, uncertainty, instrument, calibration and influence conditions.
51. raw measurement data remains immutable beneath derived/calculated/evaluated results.

---

## 7. Research posture

Further research should only alter the canonical implementation profile when it:
- closes a real semantic gap;
- removes meaningful bespoke infrastructure;
- establishes a durable interoperability standard;
- identifies a failure mode not already represented.

A popular tool alone is not a reason to add a new dependency or plugin interface.

Current baseline is sufficiently mature to proceed with M0 implementation while targeted research continues in parallel with real spikes.
