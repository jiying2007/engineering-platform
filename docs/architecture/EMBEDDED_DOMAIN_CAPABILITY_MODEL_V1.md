# Embedded Domain Capability Model v1

Date: 2026-09-24
Status: **CANONICAL DOMAIN MODEL**

## 1. Purpose

This document defines the minimum embedded-software expertise that must exist in engineering-platform from M1 onward.

The platform is not merely an agent runner. It must route work through explicit embedded engineering capabilities and reusable engineering Skills.

---

## 2. Domain

~~~text
Embedded Software Engineering
~~~

Initial canonical capabilities:

~~~text
embedded.architecture
embedded.linux-bsp
embedded.mcu-rtos
embedded.driver-component
embedded.debug-reliability
embedded.verification
~~~

This model is intentionally small.

Do not add a new capability unless repeated real work demonstrates a stable boundary and distinct methods/outputs.

---

## 3. Capability definitions

### 3.1 embedded.architecture

Responsibilities:
- system decomposition;
- software/hardware boundary analysis;
- component responsibility;
- lifecycle and failure/recovery boundary;
- cross-component compatibility;
- resource/NFR impact;
- release/upgrade impact.

Typical outputs:
- architecture impact analysis;
- interface findings;
- compatibility constraints;
- verification impact.

### 3.2 embedded.linux-bsp

Responsibilities:
- boot chain;
- kernel/BSP;
- device tree;
- MTD/UBI/filesystems;
- drivers/platform integration;
- interrupts/DMA/cache;
- system services;
- performance/startup;
- Linux OTA/runtime integration.

Typical outputs:
- boot-stage diagnosis;
- BSP findings;
- storage-layer analysis;
- kernel/driver integration evidence.

### 3.3 embedded.mcu-rtos

Responsibilities:
- startup/vector/init;
- bare-metal/RTOS execution;
- ISR/task/queue/lock concurrency;
- linker/map/ROM/RAM;
- protocol/control state machines;
- watchdog/fault handling;
- MCU OTA/bootloader.

Typical outputs:
- startup analysis;
- memory layout analysis;
- concurrency/timing findings;
- firmware integration evidence.

### 3.4 embedded.driver-component

Responsibilities:
- peripheral/component bring-up;
- protocol/API lifecycle;
- init/shutdown/error/recovery;
- supplier/alternate component integration;
- cross-platform driver/component reuse.

Typical outputs:
- integration findings;
- lifecycle/error contract;
- compatibility matrix;
- bring-up plan/evidence.

### 3.5 embedded.debug-reliability

Responsibilities:
- log/time-line analysis;
- crash/HardFault analysis;
- memory corruption;
- intermittent failure;
- performance regression;
- failure reproduction;
- root-cause evidence;
- regression prevention.

Typical outputs:
- HypothesisRegistry;
- reproduction plan;
- confirmed/rejected hypotheses;
- root-cause evidence;
- regression scope.

### 3.6 embedded.verification

Responsibilities:
- Acceptance Criteria → Verification mapping;
- layer selection;
- CI/device/HIL/test design;
- evidence sufficiency;
- regression scope;
- test result interpretation;
- independent verification.

Typical outputs:
- VerificationPlan;
- Evidence requirements;
- VerificationReport;
- regression coverage.

---

## 4. Initial Skill set

M1 canonical Skills:

### material-readiness

Owner:
- embedded domain coordination.

Input:
- WorkItem;
- source refs;
- Target context;
- logs/files/device refs;
- Acceptance Criteria.

Output:
- MaterialManifest;
- READY / BLOCKED / DEGRADED;
- explicit missing material.

BLOCK examples:
- no exact source commit;
- no authoritative log/reproduction for Debug;
- no Target identity where Target affects result.

### architecture-impact-analysis

Owner:
- embedded.architecture.

Input:
- requested change;
- current Target/architecture;
- Interface Contracts;
- NFRs.

Output:
- affected components/interfaces;
- resource/recovery/risk impact;
- proposed Verification scope.

### interface-contract-review

Owner:
- embedded.architecture.

Input:
- old/new protocol/API/lifecycle;
- producer/consumer identities.

Output:
- compatibility findings;
- breaking dimensions;
- migration/coordination requirements.

### linux-bsp-debug

Owner:
- embedded.linux-bsp.

Input:
- source;
- boot/system logs;
- Target/BSP/kernel/storage context.

Method:
- identify layer;
- identify last confirmed state;
- separate Observed/Inferred;
- propose minimum discriminating experiment;
- update HypothesisRegistry.

Output:
- layer diagnosis;
- hypotheses;
- next observation/experiment;
- evidence.

### mcu-rtos-debug

Owner:
- embedded.mcu-rtos.

Input:
- source;
- ELF/MAP;
- fault registers/logs;
- task/ISR context;
- Target/firmware identity.

Output:
- fault/concurrency/timing/memory hypotheses;
- discriminating experiment;
- evidence.

### driver-integration-review

Owner:
- embedded.driver-component.

Input:
- component datasheet/interface;
- current driver;
- Target;
- lifecycle/error requirements.

Output:
- integration gap analysis;
- lifecycle/error recovery findings;
- verification requirements.

### log-triage

Owner:
- embedded.debug-reliability.

Input:
- raw logs/traces.

Output:
- normalized timeline;
- Observed facts;
- inferred correlations;
- gaps;
- candidate hypotheses.

Rules:
- never rewrite raw logs;
- timestamp/timebase assumptions explicit.

### verification-plan-builder

Owner:
- embedded.verification.

Input:
- TaskContract;
- Acceptance Criteria;
- risk;
- Target;
- expected output.

Output:
- VerificationPlan;
- evidence class;
- procedure/environment;
- pass/fail criteria;
- reviewer requirement.

---

## 5. Skill Contract

Every Skill must define:

~~~text
skill_id
version
owner_capability
purpose
input_contract
required_material
method
output_contract
block_conditions
allowed_actions
prohibited_actions
verification/evaluation method
maturity
~~~

Optional:
- Runtime hints;
- tool preferences;
- context strategy.

Tool/runtime instructions are not the Skill itself.

---

## 6. Skill maturity

States:

~~~text
DEFINED
EVALUATED
PILOTED
PROVEN
DEPRECATED
~~~

### DEFINED
Contract/method exists.

### EVALUATED
Positive and BLOCK behavior tested on curated cases.

### PILOTED
Used in retained real engineering Run with attributable contribution.

### PROVEN
Repeated real use shows stable value and portability across relevant Runtime/provider changes.

No file-count or synthetic-CI metric may promote maturity by itself.

---

## 7. Task routing

Initial Task types:

~~~text
FEATURE
DEBUG
REFACTOR
BRINGUP
PERFORMANCE
COMPATIBILITY
DEVICE_TEST
RELEASE
~~~

Example mapping:

~~~text
DEBUG + Linux/storage symptom
 -> embedded.debug-reliability
 -> embedded.linux-bsp
 -> material-readiness
 -> log-triage
 -> linux-bsp-debug

DEBUG + MCU fault
 -> embedded.debug-reliability
 -> embedded.mcu-rtos
 -> material-readiness
 -> log-triage
 -> mcu-rtos-debug

FEATURE + cross-component protocol
 -> embedded.architecture
 -> embedded.driver-component
 -> architecture-impact-analysis
 -> interface-contract-review

DEVICE_TEST
 -> embedded.verification
 -> verification-plan-builder
~~~

Routing may be explicit in M1.
Automatic Planner arrives only after sufficient real evidence.

---

## 8. Debug Hypothesis model

Every Debug Run should preserve:

~~~text
Hypothesis
  id
  statement
  observed_support[]
  inferred_support[]
  contradicting_evidence[]
  proposed_experiment
  status
  owner/author
  updated_at
~~~

Statuses:
- PROPOSED;
- SUPPORTED;
- REJECTED;
- CONFIRMED;
- INCONCLUSIVE.

One Run maintains one shared registry across Human and Runtime participants.

---

## 9. Material readiness by task type

### FEATURE
Minimum:
- source commit;
- goal/scope;
- Target;
- Acceptance Criteria.

### DEBUG
Minimum:
- source/firmware identity;
- symptom;
- authoritative log OR reproduction;
- relevant Target/system identity.

### BRINGUP
Minimum:
- hardware/board identity;
- interface/component docs;
- source;
- observation path.

### PERFORMANCE
Minimum:
- metric definition;
- baseline;
- environment;
- reproducible measurement method.

### COMPATIBILITY
Minimum:
- producer/consumer exact revisions;
- old/new interface/config;
- required compatibility dimension.

### DEVICE_TEST
Minimum:
- device/Target;
- Artifact;
- Procedure;
- environment/fixture;
- measurement definitions.

---

## 10. Core extension rule

A new Skill/Capability is admitted only when all are true:

1. repeated real tasks need it;
2. method boundary is stable;
3. inputs/outputs are definable;
4. BLOCK behavior can be defined;
5. owner is clear;
6. value is measurable;
7. existing Skill cannot cover it cleanly.

This prevents the platform from growing a taxonomy faster than actual engineering evidence.

---

## 11. Product principle

The user-facing promise is not:

> "the platform has many agents."

It is:

> **the platform consistently applies the right embedded engineering method to the right material, preserves Human steering, produces exact engineering output, and proves the result.**
