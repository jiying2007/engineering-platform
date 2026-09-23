# Open Source Reference Review — Round 6: Code Intelligence and Observability

Date: 2026-09-23
Status: **Archived research / implementation input**
Parent:
- docs/architecture/AI_NATIVE_ENGINEERING_PLATFORM_V1_2_FINAL.md
- docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V6.md

## 1. Scope

Round 6 focused on reducing proprietary formats around:
- repository/code context;
- Runtime/LLM/tool telemetry;
- CI/CD integration events;
- static-analysis evidence.

Projects/standards reviewed:
- SCIP
- Kythe
- Tree-sitter
- LSP
- OpenTelemetry Semantic Conventions
- OpenTelemetry GenAI Semantic Conventions
- OpenInference
- OpenLLMetry
- CDEvents
- Semgrep

---

## 2. SCIP — portable code-intelligence artifact

Repository:
- https://github.com/scip-code/scip

SCIP is a language-agnostic protocol for source-code indexing and code navigation.

Important design properties:
- Protobuf transmission format;
- language-independent;
- producer-friendly;
- suitable for parallel/file-incremental indexing;
- symbols/occurrences/relationships;
- explicit non-goal of being the query/storage database;
- explicit avoidance of one giant graph encoding.

### Optimization

Generalize the earlier RepositoryMap concept into:

~~~text
CodeIntelligenceArtifact
~~~

Possible kinds:
- SCIP_INDEX
- REPOSITORY_MAP
- SYMBOL_SUMMARY
- CALL_GRAPH_SUMMARY
- CUSTOM

Common metadata:
- source tree digest;
- generator/indexer identity;
- generator version;
- language/toolchain context;
- content digest;
- completeness/coverage;
- generated_at.

For supported languages, SCIP is the preferred portable detailed-index format.

### Boundary

CodeIntelligenceArtifact is:
- Context;
- cacheable/regenerable;
- bound to a source tree;
- non-authoritative for Requirement/Policy/Verification.

A stale tree digest makes it unusable for current Formal Run context.

---

## 3. Do not build a private code graph database in M1

Kythe and other large code-indexing systems validate the value of rich cross-reference graphs, but such systems have substantial indexing/query complexity.

Decision:
- do not introduce a graph database for code context;
- store/index portable artifacts when useful;
- let Context Resolver consume them;
- introduce a query/index service only after real scale requires it.

This mirrors SCIP's useful separation between transmission format and storage/query implementation.

---

## 4. Tree-sitter and LSP — interactive context, not durable engineering authority

Tree-sitter:
- fast incremental syntax trees;
- tolerant of syntax errors;
- useful for local parsing/editing.

LSP:
- useful for live language intelligence and editor/runtime integration.

Decision:
- Runtime/Context tooling may use Tree-sitter/LSP;
- do not treat either as a formal durable artifact standard by default;
- prefer SCIP for portable offline semantic index where an indexer exists.

---

## 5. OpenTelemetry GenAI semantic conventions

Repositories:
- https://github.com/open-telemetry/semantic-conventions
- https://github.com/open-telemetry/semantic-conventions-genai

The official GenAI repository now defines semantic conventions for:
- LLM/client inference;
- agents;
- tool execution;
- evaluation;
- MCP;
- provider-specific attributes.

### Major optimization

Do not create a proprietary LLM/agent tracing vocabulary if an official OTel semantic exists.

Use official semantic conventions where compatible:
- gen_ai.*
- mcp.*
- standard HTTP/RPC/process attributes
- normal OTel trace/span/resource semantics.

Add engineering-platform namespaced attributes only for domain correlation, e.g.:

~~~text
engineering.run.id
engineering.attempt.id
engineering.execution_epoch
engineering.worker.id
engineering.work.id
engineering.task.revision
engineering.subject.digest
engineering.command.id
engineering.correlation_id
~~~

---

## 6. Telemetry is not audit authority

OpenTelemetry spans/logs/metrics may be sampled, dropped, transformed or retained differently.

Therefore:

~~~text
OTel telemetry
!=
Domain Event / Audit Journal
~~~

Use OTel for:
- latency;
- runtime/tool activity;
- provider calls;
- worker/session health;
- performance/cost;
- troubleshooting.

Use domain events for:
- Steering;
- approvals;
- state transitions;
- Human Takeover;
- privileged actions;
- authoritative decisions.

Trace IDs may link the two.

---

## 7. Prompt/content telemetry is opt-in

GenAI traces can contain sensitive:
- source;
- prompts;
- tool arguments/results;
- user messages;
- model output.

Default policy:
- record IDs, timing, provider/model/tool metadata, token/cost metrics where allowed;
- do not record full prompt/source/tool payload in central telemetry by default;
- content capture requires explicit confidentiality/retention policy;
- redaction occurs before exporter;
- raw session transcript remains a separate retention class.

---

## 8. OpenInference / OpenLLMetry

Repositories:
- https://github.com/Arize-ai/openinference
- https://github.com/traceloop/openllmetry

Both provide useful OTel-compatible LLM/agent instrumentation.

Decision:
- treat them as instrumentation/reference libraries;
- prefer official OpenTelemetry GenAI semantic conventions as platform vocabulary;
- use third-party instrumentations if they emit compatible/convertible OTel data;
- do not make engineering-platform telemetry dependent on one observability vendor/spec extension.

---

## 9. CDEvents — CI/CD semantic projection

Repository:
- https://github.com/cdevents/spec

CDEvents extends CloudEvents with CI/CD semantics for:
- builds;
- artifacts;
- source changes;
- tests;
- deployment/operations;
- test outputs.

It has CloudEvents bindings and SDKs.

### Optimization

Profile v4 already introduced:

~~~text
Domain Event
 -> Integration Event Projection
 -> CloudEvents
~~~

Refine this to:

~~~text
Domain Event
 -> Integration Event Projector
 -> CDEvents vocabulary when a standard event exists
 -> CloudEvents transport binding
~~~

Examples:
- Build queued/started/finished;
- Artifact packaged/signed/published;
- TestCaseRun queued/started/finished/skipped;
- TestOutput published.

For engineering-specific events with no CDEvents equivalent:
- use engineering-platform namespaced event type.

---

## 10. CDEvents does not replace Test/Evidence domain

CDEvents test events provide useful interoperability but do not model:
- exact Subject Manifest authority;
- Evidence Issuer trust;
- TestVariant/Verdict/Exoneration semantics in full;
- Assurance Profile;
- Human Authority.

Therefore CDEvents is an integration projection, not Verification authority.

---

## 11. Static analysis tools such as Semgrep

Static analysis can produce:
- Engineering Check;
- Security Finding;
- Verification Evidence when run through an approved trusted Procedure/Issuer.

Do not introduce a generic StaticAnalysisProvider interface in M1.

Normalize outcomes into platform Finding/Evidence records.

Semgrep or similar tools can be plugged into CI without changing domain semantics.

---

## 12. Observability model

Recommended trace hierarchy:

~~~text
Work / Run correlation
  -> Attempt
     -> Runtime session
        -> model call
        -> tool call
        -> local command
        -> Action Gateway request
     -> build/test
     -> external integration
~~~

Use:
- standard OTel attributes;
- GenAI semantic conventions;
- platform correlation attributes.

Do not embed full domain object snapshots into spans.

---

## 13. Code context model

Recommended:

~~~text
Source Tree
  -> CodeIntelligenceArtifact
      -> SCIP index where supported
      -> lightweight RepositoryMap fallback
  -> Context Resolver
  -> selected Context Refs
  -> Run Input Manifest
~~~

Run Input Manifest records exact CodeIntelligenceArtifact digest if it materially contributed to Formal Run context.

This makes generated context reproducible without treating it as Authority.

---

## 14. M0/M1 impact

### M0
Freeze:
- CodeIntelligenceArtifact schema;
- TelemetryProfile;
- engineering.* OTel correlation attribute names;
- IntegrationEvent projection mapping;
- CDEvents mapping table for supported standard events.

### M1
Implement:
- OpenTelemetry tracing with domain correlation;
- safe/default telemetry redaction policy;
- CDEvents projection for a few Git/CI/Verification lifecycle events.

CodeIntelligenceArtifact is optional in M1.

### M4/context optimization
Add SCIP indexing where it measurably improves large-repo Runtime context.

---

## 15. New invariants

1. **Code intelligence is tree-bound context, never engineering authority.**
2. **Telemetry is observable execution data, never the authoritative audit log.**
3. **Sensitive prompt/tool content is not centrally captured by default.**
4. **Use official OTel GenAI semantics before inventing private Runtime telemetry fields.**
5. **Use CDEvents vocabulary for external CI/CD events where a standard semantic exists.**
6. **Internal Domain Event semantics remain richer and authoritative.**

---

## 16. Conclusion

Round 6 reduces two future compatibility risks:
- proprietary code-intelligence artifacts;
- proprietary AI/CI telemetry vocabularies.

Preferred direction:
- SCIP for portable code intelligence where useful;
- OpenTelemetry + official GenAI semantics for observability;
- CDEvents + CloudEvents for CI/CD integration events.

None becomes a new M1 authority or mandatory large service.
