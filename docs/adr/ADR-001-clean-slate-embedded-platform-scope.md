# ADR-001 — Clean-Slate Embedded Platform Scope

Date: 2026-09-24
Status: **ACCEPTED**

## Context

engineering-platform is intended for the R&D center's embedded-software engineering workflow.

A historical digital-worker / embedded-expert implementation exists and contains useful ideas:
- Domain / Capability / Skill;
- Material Readiness;
- Debug hypothesis discipline;
- Engineering vs Verification vs Review separation;
- evidence-driven maturity.

However, engineering-platform is not a compatibility rewrite of that system.

Separately, broad architecture research explored many product-lifecycle domains:
- manufacturing;
- PLM;
- fleet;
- PSIRT;
- RMA;
- compliance;
- DPP;
- supply-chain traceability;
- diagnostics;
- certification;
- privacy;
- and others.

If all researched semantics are inherited into M0/M1, the project becomes a product-lifecycle meta-platform before proving the embedded engineering loop.

## Decision

### 1. engineering-platform is clean-slate

There is no requirement to preserve:
- historical repository boundaries;
- legacy schemas;
- legacy Runtime profiles;
- legacy cross-repository ownership;
- old file layout;
- migration compatibility.

Useful ideas may be re-derived and implemented under new contracts.

### 2. Product boundary

engineering-platform is first:

> an AI-native embedded-software engineering platform.

Core value:
- embedded engineering routing/methods;
- interactive Human + AI execution;
- exact source/artifact identity;
- build/test/device facts;
- Evidence/Verification/Review;
- controlled privileged actions;
- recoverable execution.

It is not first:
- PLM;
- MES;
- ERP;
- QMS;
- Digital Twin;
- regulatory portal.

### 3. Expert/Capability/Skill belongs in Core

Embedded-specific engineering methods are M1 capability.

Automatic Planner and broad Skill marketplace are not.

### 4. Context belongs in Core; knowledge platform does not

M1 must read/version/contextualize:
- source;
- requirements;
- docs;
- logs;
- datasheets;
- prior verified records.

A full curated Knowledge lifecycle is an extension.

### 5. Research is preserved as Extension Catalog

Historical research is retained as option/invariant evidence.

No research concept becomes an M0 blocker unless a real M0/M1 consumer requires it.

### 6. M0 is no longer cumulative

Statements such as:

> retain all previous M0 requirements

are prohibited for the current plan.

Every Core requirement must have a direct M1 consumer.

## Consequences

Positive:
- M0/M1 become implementable;
- embedded value appears in first release;
- legacy coupling disappears;
- future product-lifecycle work remains possible;
- research investment is preserved without dominating Core.

Negative:
- some previously "canonical implementation profiles" become historical/reference only;
- future extensions may require new ADRs and migration from Core contracts;
- not every researched schema is implemented immediately.

## Current canonical documents

- `docs/architecture/EMBEDDED_AI_ENGINEERING_PLATFORM_CORE_V1.md`
- `docs/architecture/EMBEDDED_DOMAIN_CAPABILITY_MODEL_V1.md`
- `docs/roadmap/CORE_M0_M1_VERTICAL_SLICE_PLAN_V1.md`
- `docs/extensions/EXTENSION_CATALOG_V1.md`

Historical architecture/research remains available for reference.

## Rule for future change

A proposal to expand Core must answer:

1. Which real embedded engineering task requires it?
2. Why can the current Core not express the requirement?
3. What is the smallest new invariant/contract?
4. What pilot proves value?
5. What is the exit/removal condition?

Without those answers, the proposal remains an Extension/Research item.
