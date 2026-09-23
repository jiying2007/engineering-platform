# Reference-Aligned Implementation Profile v7

Date: 2026-09-23
Status: **Canonical implementation companion to architecture v1.2**
Supersedes: docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V6.md

Research basis:
- Open Source Reference Review Rounds 1–6
- Open Source Reference Synthesis Optimization
- Architecture v1.2

## 1. Goal

Profile v7 standardizes generated code context and observability/interoperability formats without adding new mandatory services.

New principles:
- portable code intelligence over private graph formats;
- official OpenTelemetry GenAI semantics over private LLM tracing vocabulary;
- CDEvents/CloudEvents for external CI/CD integration;
- telemetry remains separate from audit authority.

---

## 2. M1 stack remains unchanged

Required:
- PostgreSQL
- Temporal
- OPA
- S3/MinIO-compatible ArtifactStore
- OpenTelemetry
- Toxiproxy
- Git/CI
- Codex
- native Ubuntu execution

No SCIP server, code graph database, OpenInference backend or CDEvents broker is required.

---

## 3. CodeIntelligenceArtifact

Replace the narrow RepositoryMap concept with a generic immutable generated-context artifact:

~~~text
CodeIntelligenceArtifact
~~~

Kinds:
- SCIP_INDEX
- REPOSITORY_MAP
- SYMBOL_SUMMARY
- CALL_GRAPH_SUMMARY
- CUSTOM

Metadata:
- source_tree_digest;
- generator;
- generator_version;
- language/toolchain context;
- coverage/completeness metadata;
- content_digest;
- generated_at.

Rules:
- bound to exact tree;
- stale when tree changes unless explicitly proven reusable;
- non-authoritative context;
- regenerable/cacheable.

SCIP is the preferred portable semantic index format where an indexer exists.

---

## 4. No private code graph service in M1

Do not build:
- graph database;
- cross-reference service;
- semantic code search server

for M1.

Context Resolver can consume:
- raw repository;
- lightweight RepositoryMap;
- SCIP Artifact;
- future query services.

Storage/query implementation remains independent from the portable CodeIntelligenceArtifact.

---

## 5. Run Input Context binding

If a generated code-intelligence artifact materially contributes to a Formal Run, Run Input Manifest records:

~~~text
context_ref
kind
content_digest
source_tree_digest
trust_class=REFERENCE
generator/version
~~~

This makes context reproducible while retaining:

~~~text
Context != Authority
~~~

---

## 6. TelemetryProfile

Add a versioned/configurable:

~~~text
TelemetryProfile
~~~

Controls:
- enabled signals;
- sampling;
- content capture;
- redaction;
- retention class;
- exporter route;
- confidentiality constraints.

Default Formal profile:
- traces/metrics enabled;
- model/provider/tool metadata allowed;
- prompt/source/tool payload content disabled unless policy explicitly allows;
- secrets always redacted/forbidden.

---

## 7. OpenTelemetry semantics

Use official OpenTelemetry conventions first.

### Standard
- HTTP/RPC/process/resource attributes
- trace/span/log/metric semantics

### GenAI
Use official GenAI/MCP semantic conventions where compatible:
- gen_ai.*
- mcp.*
- provider-specific standard fields

### engineering-platform correlation namespace

Add only domain identifiers needed for correlation:

~~~text
engineering.work.id
engineering.run.id
engineering.attempt.id
engineering.execution_epoch
engineering.task.revision
engineering.worker.id
engineering.subject.digest
engineering.command.id
engineering.correlation_id
~~~

Avoid embedding large manifests or sensitive content in spans.

---

## 8. Telemetry != Audit

Authoritative:
- Domain Event;
- Decision;
- Steering;
- approval;
- privileged action receipt;
- state transition.

Operational:
- OTel span/log/metric;
- runtime/tool latency;
- token/cost;
- worker health;
- performance trace.

OTel may be sampled/dropped.
Audit cannot rely on OTel completeness.

Trace/span IDs can be stored on domain events for navigation.

---

## 9. Runtime telemetry privacy

Default:
- no full prompts;
- no source file bodies;
- no unrestricted tool result bodies;
- no secret-bearing environment;
- no credential material.

Allowed metadata can include:
- model/provider identity;
- operation/tool name;
- token counts;
- latency;
- status/error class;
- Run/Attempt correlation.

Content capture requires an explicit data-classification/retention policy.

---

## 10. Third-party AI instrumentation

OpenInference/OpenLLMetry may be used as instrumentation libraries when useful.

Requirements:
- emit/convert to OpenTelemetry;
- respect TelemetryProfile;
- do not redefine platform authority;
- do not force vendor backend;
- content capture defaults remain platform-controlled.

Official OTel GenAI vocabulary wins when overlapping semantics conflict.

---

## 11. Integration events v7

Internal Domain Event remains canonical.

External projection path:

~~~text
Domain Event
 -> Integration Event Projector
 -> CDEvents event when matching vocabulary exists
 -> CloudEvents binding
 -> WorkBuddy/webhook/event bus
~~~

Use CDEvents where supported for:
- build lifecycle;
- artifact packaged/signed/published;
- test case/suite lifecycle;
- test output publication;
- future deployment/operations events.

Use engineering-platform namespaced CloudEvent types for domain-specific events not covered by CDEvents.

---

## 12. Integration projection is lossy by design

External events carry:
- stable subject IDs;
- event type;
- source;
- timestamp;
- correlation/chain reference;
- minimal business-safe attributes.

They do not carry the complete authority graph.

Consumers needing authoritative decisions query Control Plane by ID.

This prevents an event bus from becoming a second source of truth.

---

## 13. Static analysis normalization

Static-analysis tools emit normalized:
- Finding;
- Engineering Check;
- Evidence only when run through trusted Procedure/Issuer.

No StaticAnalysisProvider plugin framework is needed.

Tools such as Semgrep can be CI integrations.

---

## 14. M1 observability requirements

Instrument:
- Control API request;
- Temporal workflow/activity;
- Worker dispatch;
- Session attach/reconnect;
- Runtime/model/tool calls when observable;
- local commands;
- Action Gateway;
- Git/CI operations;
- Artifact upload;
- Verification.

Every trace should allow navigation to:
- Run;
- Attempt;
- Worker;
- correlation ID.

No requirement for model prompt recording.

---

## 15. M4 context optimization

When repositories grow or Runtime context becomes expensive:

1. generate SCIP index where supported;
2. generate lightweight RepositoryMap fallback;
3. attach CodeIntelligenceArtifact to source tree;
4. Context Resolver selects minimal relevant context;
5. Run Input Manifest freezes selected context digests.

Kythe/large code-graph services are considered only if cross-repo query scale requires them.

---

## 16. Final rules added by v7

1. **Generated code intelligence is source-tree-bound, immutable context.**
2. **SCIP is preferred over inventing a proprietary portable semantic index.**
3. **Observability follows official OTel/GenAI conventions first.**
4. **Telemetry and audit have different completeness/retention/security guarantees.**
5. **Prompt/source/tool content is not centrally traced by default.**
6. **CDEvents vocabulary is preferred for external CI/CD event semantics.**
7. **CloudEvents/CDEvents projections never become business authority.**

Architecture v1.2 remains canonical.
Profile v7 is the current implementation companion.
