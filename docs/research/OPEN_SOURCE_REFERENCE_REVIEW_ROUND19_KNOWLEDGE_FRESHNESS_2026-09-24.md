# Open Source Reference Review — Round 19: Knowledge Freshness, Certification and Revalidation

Date: 2026-09-24
Status: **Archived research / implementation input**

Parent:
- docs/architecture/AI_NATIVE_ENGINEERING_PLATFORM_V1_2_FINAL.md
- docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V17.md

## 1. Scope

Round 19 reviewed knowledge/context-governance practices from:
- DataHub
- OpenMetadata
- Backstage catalog/documentation patterns
- existing KnowledgeCandidate / Incident / TraceLink design

Focus:
- freshness;
- ownership/stewardship;
- certification/trust;
- provenance;
- review/expiry;
- automated revalidation;
- AI memory versus curated engineering knowledge.

---

## 2. Raw context, memory and durable knowledge are different

Use three distinct levels:

~~~text
Context
  transient or source material

MemoryCandidate
  extracted reusable observation

EngineeringKnowledge
  curated, scoped, reviewed and lifecycle-managed
~~~

Examples of Context:
- chat;
- logs;
- PR discussion;
- Incident timeline;
- raw documents;
- runtime output.

Examples of durable EngineeringKnowledge:
- Known Issue;
- Recovery Runbook;
- Design Rule;
- Compatibility Rule;
- Test Method;
- Diagnostic Pattern.

Raw context is never promoted automatically.

---

## 3. KnowledgeItem

Published durable knowledge becomes:

~~~text
KnowledgeItem
~~~

Fields:
- knowledge_id;
- knowledge_type;
- version;
- title/summary;
- content/snapshot Artifact;
- scope;
- applicable Target/Product/Component;
- assumptions;
- source/provenance refs;
- supporting Evidence/Decision refs;
- owner/steward;
- status;
- published_at;
- review_at;
- expires_at?;
- validation policy ref;
- supersedes/superseded_by refs.

---

## 4. Knowledge status

Suggested lifecycle:

~~~text
CANDIDATE
 -> REVIEWED
 -> PUBLISHED
 -> STALE
 -> REVALIDATING
 -> PUBLISHED
    | INVALID
    | SUPERSEDED
    | RETIRED
~~~

Important:
- STALE means "needs current confirmation";
- INVALID means known false/not applicable;
- SUPERSEDED means replaced by a newer version;
- RETIRED means intentionally no longer used.

Do not collapse all non-current states into deleted.

---

## 5. Freshness is policy, not just age

A KnowledgeItem can become stale because:
- review date elapsed;
- referenced Target Revision changed;
- Interface Contract changed;
- dependency/toolchain changed;
- Incident disproved assumption;
- Release/Artifact was revoked;
- calibration/procedure changed;
- security Finding affects it;
- source document was superseded.

Therefore freshness has both:

~~~text
time-based trigger
dependency/change-based trigger
~~~

---

## 6. KnowledgeFreshnessPolicy

Add versioned:

~~~text
KnowledgeFreshnessPolicy
~~~

Fields:
- applicable knowledge type/scope;
- review interval;
- expiry rule;
- dependency triggers;
- required revalidation checks;
- owner/escalation;
- stale-use policy.

Example:

~~~text
RecoveryRunbook
  review every 180d
  stale if referenced CLI/API contract changes
  stale if procedure/test fails
  block autonomous execution when STALE
~~~

---

## 7. KnowledgeValidationReceipt

Revalidation produces immutable:

~~~text
KnowledgeValidationReceipt
~~~

Binds:
- KnowledgeItem version;
- validation procedure;
- current dependency/input digests;
- Target/environment;
- validator/issuer;
- checks performed;
- result;
- Evidence refs;
- timestamp.

Results:
- VALID;
- STALE;
- INVALID;
- INCONCLUSIVE;
- TOOL_ERROR.

Publication/freshness projection derives from receipts + policy.

---

## 8. Source/provenance remains visible

Knowledge must cite its source.

Examples:
- closed Incident/Postmortem;
- Design Decision;
- Verification;
- Interface Contract;
- Test Procedure;
- external authoritative document snapshot.

AI summaries may improve readability but do not erase source provenance.

A user/agent should be able to answer:
- where did this rule come from?
- what evidence supported it?
- when was it last validated?
- what does it apply to?

---

## 9. Scope is first-class

KnowledgeItem scope may include:
- product;
- TargetRevision range;
- board revision;
- firmware/platform;
- interface revision;
- toolchain;
- environment;
- release channel.

Example:

~~~text
Known Issue:
  SSC305 + FlashVendorX + BSP 3.2
~~~

must not be retrieved as if it applies to:
~~~text
Rockchip + different flash/controller
~~~

without explicit generalized evidence.

---

## 10. Assumptions are first-class

Knowledge may depend on assumptions:
- hardware revision unchanged;
- API behavior stable;
- partition layout unchanged;
- calibration procedure valid;
- specific kernel/compiler range;
- network topology.

If an assumption changes:
- item becomes candidate STALE;
- ImpactAnalysis can schedule revalidation.

---

## 11. Certification/trust level

Borrowing metadata-catalog certification ideas, add optional:

~~~text
KnowledgeTrustLevel
~~~

Example:
- DRAFT;
- REVIEWED;
- VERIFIED;
- AUTHORITATIVE_WITHIN_SCOPE.

Trust level is not reputation.

It derives from:
- provenance;
- review;
- validation;
- issuer;
- freshness.

AI-generated content starts below authoritative levels.

---

## 12. Retrieval should consider validity

Context Resolver / Knowledge retrieval should rank/filter using:
- scope match;
- status;
- freshness;
- trust level;
- provenance strength;
- target/interface compatibility;
- recency only as one factor.

A newer stale/unreviewed memory is not automatically better than an older verified rule.

---

## 13. Stale knowledge handling

Policy examples:

### READ_ONLY_ADVISORY
Stale item may still be shown with warning.

### FORMAL_CONTEXT
Stale item can be included only as untrusted context.

### AUTONOMOUS_ACTION
Stale Runbook/Rule cannot authorize or trigger privileged action.

### RELEASE_GATE
Only currently valid/certified knowledge may support a gate.

This avoids silent reuse of outdated runbooks.

---

## 14. Automated revalidation

Knowledge revalidation can reuse existing engineering mechanisms:

~~~text
KnowledgeItem
 -> freshness trigger
 -> Task/Procedure
 -> Run
 -> Evidence
 -> KnowledgeValidationReceipt
 -> status update
~~~

Example:
- replay a Recovery Runbook in simulation/lab;
- rerun compatibility test;
- verify linked command/API still exists;
- rerun unit/property tests attached to Design Rule.

This is stronger than merely asking an owner to "review document".

---

## 15. Knowledge and TraceLink

TraceLink can connect:
- KnowledgeItem -> Requirement;
- KnowledgeItem -> Target;
- KnowledgeItem -> Interface;
- KnowledgeItem -> Procedure;
- KnowledgeItem -> Incident;
- KnowledgeItem -> source component.

Trace traversal can identify knowledge affected by engineering changes.

No separate knowledge graph database is required initially.

---

## 16. Knowledge and CodeIntelligence

Generated code index/context is not Knowledge.

~~~text
SCIP index
RepositoryMap
symbol summary
~~~

remain CodeIntelligenceArtifact.

A curated design/compatibility rule about those components may become KnowledgeItem.

Do not flood durable knowledge with generated source summaries.

---

## 17. Knowledge publication from incidents

Keep the Round 12 boundary:

~~~text
Incident/Postmortem
 -> KnowledgeCandidate
 -> curator/review
 -> KnowledgeItem
~~~

AI may draft the candidate.

Publication requires explicit curation according to policy.

---

## 18. Knowledge replacement

Newer item does not rewrite history.

Use:
- version;
- SUPERSEDES TraceLink;
- scope migration;
- explicit old status.

Formal Runs retain the exact KnowledgeItem/version digests used in context.

This allows reproducing historical decisions.

---

## 19. External knowledge/context platforms

DataHub/OpenMetadata/Backstage-like systems may provide:
- catalog/search;
- ownership;
- UI;
- graph projection;
- context retrieval;
- documentation.

They can consume/publish projections.

They do not become the authority for:
- Evidence;
- Verification;
- Release;
- engineering Knowledge lifecycle unless explicitly chosen as the Knowledge store behind the same contracts.

---

## 20. M0/M1 impact

### M0
Freeze:
- KnowledgeItem;
- KnowledgeFreshnessPolicy;
- KnowledgeValidationReceipt;
- KnowledgeTrustLevel/status.

### M1
No knowledge platform required.

Create one synthetic:
- valid Known Issue;
- stale Runbook;
- revalidation receipt.

### M2/M3
Integrate real Incident/Postmortem/Procedure-derived knowledge.

---

## 21. New invariants

1. Raw context/memory is not durable engineering Knowledge.
2. Published Knowledge always has provenance, scope, owner and lifecycle.
3. Freshness is driven by both time and dependency/change triggers.
4. STALE, INVALID, SUPERSEDED and RETIRED are distinct.
5. Revalidation creates immutable receipts/evidence.
6. Stale Knowledge cannot silently authorize autonomous/privileged actions.
7. AI-generated Knowledge starts as candidate/context, not authority.
8. Retrieval ranks by scope/trust/freshness, not recency alone.
9. Historical Runs retain exact Knowledge versions used.
10. Knowledge graph/search is a projection; engineering truth remains in typed records/evidence.

---

## 22. Conclusion

Round 19 turns "Knowledge" from a document bucket into a governed engineering asset:

> **Knowledge is useful only when the platform can state where it came from, what it applies to, whether it is still valid, and how that validity was last checked.**
