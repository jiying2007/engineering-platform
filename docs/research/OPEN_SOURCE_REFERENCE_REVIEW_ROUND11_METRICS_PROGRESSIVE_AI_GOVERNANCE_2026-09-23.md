# Open Source Reference Review — Round 11: Engineering Metrics, Progressive Delivery and AI Runtime Governance

Date: 2026-09-23
Status: **Archived research / implementation input**
Parent:
- docs/architecture/AI_NATIVE_ENGINEERING_PLATFORM_V1_2_FINAL.md
- docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V10.md

## 1. Scope

Round 11 reviewed:
- Apache DevLake
- CHAOSS / GrimoireLab
- Google Four Keys historical implementation
- OpenSLO / Sloth / Pyrra
- OpenFeature / flagd
- Unleash / Flipt
- Argo Rollouts / Flagger
- ToolHive
- Docker MCP Gateway
- MCP Registry
- LiteLLM / Portkey / Langfuse patterns
- Promptfoo
- Purple Llama

Focus:
- engineering metrics without creating bad incentives;
- progressive delivery and runtime flags;
- rollout analysis/rollback;
- MCP/tool governance;
- runtime/provider qualification and AI security regression.

---

## 2. Engineering metrics are a derived read model

Apache DevLake and GrimoireLab both aggregate SDLC data from many systems and compute engineering metrics/dashboards.

### Decision

Add no new authority object for DORA/engineering metrics.

Create a derived analytics layer:

~~~text
Domain Events / Run / Artifact / Verification / Release / Incident
  -> Analytics Projection
  -> Metric Measurements
  -> dashboards / reports
~~~

Metrics never mutate authoritative engineering state.

### MetricDefinition

Reserve a versioned definition:

~~~text
MetricDefinition
  metric_id
  version
  name
  population/cohort
  window
  source events/objects
  formula
  exclusions
  data freshness
  interpretation notes
~~~

A reported number must identify the MetricDefinition version.

---

## 3. Do not score individuals from AI activity

Do not turn:
- lines generated;
- prompt count;
- token count;
- model usage;
- commits;
- PR count;
- agent acceptance rate

into individual performance scores.

These are operational/productivity signals at most.

For platform evaluation, prefer paired metrics:

~~~text
Speed
  cycle time
  queue/wait time
  time-to-first-evidence
  merge/release lead time

Quality
  first-pass verification
  rework rate
  stale-evidence rate
  defect escape
  incident/regression rate

Automation effectiveness
  successful resume
  steering rate
  takeover rate
  retry rate
  autonomous completion within policy

Cost/resource
  model/API cost
  CI compute
  HIL occupancy
  device wait
~~~

Interpret metrics by team/workflow/cohort and time window, not as automatic employee ranking.

---

## 4. AI-native metric principle

AI coding is valuable only when delivery quality does not degrade.

Therefore any AI acceleration metric should be paired with one or more quality/assurance metrics.

Example:

~~~text
Run cycle time down
AND
verification/rework/escape rate stable or better
~~~

is meaningful.

~~~text
tokens down
lines generated up
~~~

alone is not.

---

## 5. Operational objectives — OpenSLO practice

OpenSLO provides a vendor-neutral SLO vocabulary.

### Optimization

For platform/runtime operational reliability, reserve:

~~~text
OperationalObjective
~~~

Examples:
- Session attach availability;
- Worker dispatch latency;
- Artifact finalize success;
- Temporal workflow availability;
- Device-lab queue SLA;
- Release confirmation latency.

This is operational reliability, not product Verification.

OpenSLO-compatible export/import may be used later.

---

## 6. Feature flags — OpenFeature

OpenFeature provides a vendor-neutral feature-flag evaluation API.

### Decision

When Runtime/Product behavior is controlled by flags, model:

~~~text
RuntimeConfigurationSnapshot
~~~

with:
- flag provider/profile;
- evaluated flag set or immutable configuration snapshot;
- environment;
- content digest;
- effective window.

The exact runtime configuration used during Verification/Release observation must be traceable.

Feature flag vendor remains external and replaceable.

---

## 7. Feature flag state is not Artifact identity

A binary Artifact may remain identical while runtime behavior changes because flags/config changed.

Therefore:

~~~text
Deployment/Operational Subject
 =
 Release Bundle digest
 + Runtime Configuration digest
 + environment/cohort
~~~

Do not claim a previously verified behavior remains valid if a behavior-affecting flag changed outside its verified scope.

---

## 8. Progressive Delivery

Argo Rollouts and Flagger demonstrate:
- canary;
- blue/green;
- staged traffic;
- metric analysis;
- automated pause/rollback;
- manual judgement.

### Add:

~~~text
PromotionPlan
~~~

Immutable/versioned:
- Release Manifest digest;
- target environment;
- stages;
- cohort/blast radius;
- entry criteria;
- observation window;
- metrics/health checks;
- manual checkpoints;
- abort/rollback criteria;
- rollback target.

A production Release may be authorized before rollout, but each promotion stage is separately admitted/observed.

---

## 9. Promotion stage model

~~~text
PENDING
 -> ADMITTED
 -> DEPLOYING
 -> OBSERVING
 -> PROMOTED
    | PAUSED
    | ROLLED_BACK
    | FAILED
    | UNKNOWN
~~~

Each stage records:
- exact subject/config;
- cohort;
- observation Evidence;
- decisions;
- actual external state.

Final Release confirmation occurs only after policy-required stages complete.

---

## 10. SLO/metric-driven rollout is evidence, not automatic truth

Rollout health checks can use:
- OpenSLO-style objectives;
- device telemetry;
- crash/error rate;
- installation success;
- functional smoke checks.

Metric result can automatically trigger:
- pause;
- rollback;
- escalation

according to pre-authorized PromotionPlan.

But:
- metric source/procedure must be trusted;
- missing data is not success;
- rollout controller does not gain general Release authority.

---

## 11. Tool/MCP governance — ToolHive and Docker MCP Gateway

Repositories:
- https://github.com/stacklok/toolhive
- https://github.com/docker/mcp-gateway

Strong practices:
- isolated MCP server runtime;
- centralized gateway;
- curated catalog/registry;
- per-profile tool allowlists;
- identity/access enforcement;
- secrets kept out of local server configuration where possible;
- audit/OTel;
- OCI-distributed profiles/catalogs.

### Optimization

Add platform-owned:

~~~text
ToolProfile
~~~

bound into Run Input Manifest.

Contains:
- approved MCP/tool server identities;
- exact server/version/image/package digest where available;
- allowed tools;
- denied tools;
- network/data policy;
- credential policy;
- trust classification;
- schema/description digest;
- profile version/digest.

Runtime cannot dynamically expand beyond ToolProfile authority.

---

## 12. MCP Registry is discovery, not trust authority

Official or private registries are useful discovery sources.

But:

~~~text
registry-listed
!=
engineering-approved
~~~

A server/tool must pass:
- source/provenance review;
- version pinning;
- capability policy;
- data classification compatibility;
- security/adversarial qualification

before entering an approved ToolProfile.

---

## 13. Tool descriptions and schemas are versioned untrusted inputs

MCP/tool descriptions may influence model behavior.

Therefore ToolProfile records:
- server manifest digest;
- tool schema digest;
- description/content digest.

A tool-description/schema change can trigger:
- Runtime qualification;
- adversarial tests;
- policy review.

Dynamic tool text cannot grant new capability.

---

## 14. AI Runtime qualification

Promptfoo and Purple Llama reinforce the need for repeatable AI/agent security evaluation.

Add optional/versioned:

~~~text
RuntimeQualificationProfile
~~~

Defines:
- runtime/provider/model/profile;
- ToolProfile;
- representative task corpus;
- security/adversarial corpus;
- expected invariants;
- pass/fail thresholds;
- data/confidentiality profile.

Produces:

~~~text
RuntimeQualificationEvidence
~~~

Used to decide whether a RuntimeProfile is eligible for Formal Runs of a given assurance/data class.

---

## 15. Runtime qualification triggers

Re-run qualification when material changes occur:
- provider/model family/profile;
- Runtime Adapter;
- system instruction;
- sandbox policy;
- ToolProfile;
- MCP server/tool schema;
- Context Resolver;
- Action Gateway;
- provider data-handling policy.

This operationalizes the adversarial-regression requirement already found in Round 9 security review.

---

## 16. Model gateways and LLM observability

LiteLLM/Portkey/Langfuse-like systems show useful mechanisms:
- multi-provider gateway;
- quotas;
- cost;
- routing;
- retry/fallback;
- tracing/evals.

Decision:
- do not make one gateway an authority;
- RuntimeProvider remains the engineering abstraction;
- optional gateway may implement provider connectivity/cost/routing;
- provider selection must obey confidentiality/qualification policy.

OpenTelemetry remains the preferred platform telemetry standard.

---

## 17. M1 impact

M1 does not require:
- DevLake;
- GrimoireLab;
- OpenFeature provider;
- Argo Rollouts;
- Flagger;
- ToolHive;
- Docker MCP Gateway;
- Promptfoo;
- Langfuse.

M0 reserves:
- MetricDefinition;
- ToolProfile;
- RuntimeQualificationProfile;
- RuntimeConfigurationSnapshot;
- PromotionPlan schemas where cheap.

M1 requires only:
- explicit tool/capability set for Codex Run;
- operational metrics from existing domain/OTel data.

---

## 18. M2/M3 impact

M2/M3 may add:
- DevLake-style analytics projection;
- feature-flag/config snapshots;
- staged deployment;
- rollout observation;
- Runtime qualification pipeline;
- curated MCP catalog/gateway.

ToolHive/Docker MCP Gateway become strong candidates if MCP tool usage grows beyond a few fixed integrations.

---

## 19. New invariants

1. **Engineering metrics are derived analytics, never workflow authority.**
2. **AI usage metrics never become automatic individual performance scores.**
3. **Acceleration metrics are interpreted together with quality/assurance outcomes.**
4. **Runtime configuration/feature flags are part of observed deployment subject when behavior-affecting.**
5. **Progressive rollout uses an immutable PromotionPlan with explicit rollback.**
6. **Registry discovery does not imply tool trust.**
7. **Formal Runs use an exact approved ToolProfile.**
8. **Dynamic MCP/tool content cannot grant capabilities.**
9. **Material Runtime/tool/provider changes trigger qualification/security regression.**
10. **LLM gateway routing does not override data/assurance policy.**

---

## 20. Conclusion

Round 11 strengthens post-build operational control and AI governance without increasing M1 runtime dependencies.

The main additions are:
- derived MetricDefinition;
- PromotionPlan;
- RuntimeConfigurationSnapshot;
- ToolProfile;
- RuntimeQualificationProfile.

All remain subordinate to the existing engineering authority chain.
