# Open Source Reference Review — Round 10: Simulation, Emulation and Verification Fidelity

Date: 2026-09-23
Status: **Archived research / implementation input**
Parent:
- docs/architecture/AI_NATIVE_ENGINEERING_PLATFORM_V1_2_FINAL.md
- docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V9.md

## 1. Scope

Round 10 reviewed:
- Renode
- QEMU
- Zephyr simulation practices
- Gazebo Sim
- Webots
- ROS 2 launch/testing
- syzkaller/continuous fuzzing practices

Focus:
- virtual embedded targets;
- software/emulator/simulator vs HIL evidence;
- robotics simulation;
- verification-fidelity semantics;
- reducing scarce physical-device usage without weakening release authority.

---

## 2. Renode — virtual embedded platforms

Repository:
- https://github.com/renode/renode

Renode can:
- execute unmodified embedded binaries;
- model CPUs, SoCs and peripherals;
- simulate multi-node wired/wireless systems;
- support deterministic development/test workflows;
- integrate automated tests.

### Value

For supported MCU/SoC targets, virtual execution can move:
- boot/basic integration;
- protocol behavior;
- firmware functional tests;
- fault scenarios

earlier than physical HIL.

It does not prove analog/electrical/mechanical behavior.

---

## 3. QEMU — virtual machine/OS execution

Repository:
- https://github.com/qemu/qemu

QEMU provides:
- complete-machine emulation;
- CPU/system emulation;
- user-space ABI emulation;
- stable integration interfaces.

### Value

For embedded Linux:
- boot/rootfs testing;
- process/service integration;
- package/runtime tests;
- selected kernel/userspace behavior

can execute without scarce physical boards where hardware emulation fidelity is sufficient.

---

## 4. Gazebo / Webots — robot-system simulation

Repositories:
- https://github.com/gazebosim/gz-sim
- https://github.com/cyberbotics/webots

They provide:
- physics;
- robot/environment models;
- sensors/noise;
- plugin/control interfaces;
- repeatable simulation environments.

### Value

For robotics:
- navigation;
- motion logic;
- sensor pipelines;
- fault/environment scenarios;
- multi-run regression

can gain early broad coverage.

Physical/mechanical and final sensor/actuator behavior still require appropriate real-system validation.

---

## 5. Verification Environment Class

Add a standard classification:

~~~text
VerificationEnvironmentClass

STATIC
HOST_SIMULATED
EMULATED
VIRTUAL_PLATFORM
SIL
HIL
PHYSICAL_DUT
FIELD
~~~

Exact names may be refined in M0/M2.

Each Evidence/Procedure execution records:
- environment class;
- environment/model definition digest;
- simulator/emulator/tool version;
- Target Revision mapping;
- relevant fidelity limitations.

---

## 6. Simulation Model / Environment Definition

Use existing Artifact/ModelArtifact concepts.

Add immutable:

~~~text
VerificationEnvironmentDefinition
~~~

May reference:
- simulator/emulator name/version;
- machine/board model;
- peripheral models;
- robot/world model;
- physics engine/settings;
- sensor/noise model;
- network topology;
- external stimulus;
- deterministic seed;
- configuration digest.

The environment definition is part of the Verification Subject where relevant.

---

## 7. Verification level is policy-driven

A Procedure/AC declares acceptable evidence environments.

Examples:

~~~text
unit logic
  HOST_SIMULATED acceptable

boot/rootfs behavior
  EMULATED or PHYSICAL_DUT depending on requirement

MCU protocol logic
  VIRTUAL_PLATFORM may satisfy development gate
  HIL required for release gate

motor torque/current/thermal
  PHYSICAL_DUT/HIL required

navigation algorithm regression
  SIL/simulation accepted for broad regression
  HIL/physical acceptance required for release criteria
~~~

Assurance Profile determines the minimum acceptable level.

---

## 8. Simulation PASS is not automatic hardware PASS

Introduce explicit:

~~~text
EnvironmentEquivalenceDecision
~~~

only when policy wants to reuse evidence across environment classes.

Fields:
- source environment definition;
- target environment/Target Revision;
- evidence class;
- equivalence scope;
- known limitations;
- validation evidence;
- authority/policy;
- validity window/version.

Default:
- no implicit equivalence.

This prevents a simulator regression suite from silently becoming product-release proof.

---

## 9. Verification ladder

Resolved Verification Execution Plan may stage checks:

~~~text
STATIC
 -> HOST/SIM
 -> EMULATOR/VIRTUAL PLATFORM
 -> SIL
 -> HIL
 -> PHYSICAL
~~~

Advantages:
- fail fast;
- lower HIL occupancy;
- run broad matrices cheaply;
- reserve real hardware for hardware-sensitive/final assurance.

A failure in an earlier mandatory stage can stop later expensive stages according to policy.

---

## 10. Environment-specific test variants

TestVariant includes environment/fidelity:

~~~text
test_definition
target
artifact/bundle
environment_class
environment_definition_digest
fixture/model
seed
conditions
~~~

Simulation and HIL executions of the same semantic test remain different variants.

Their attempts/verdicts are not merged unless policy explicitly defines aggregation.

---

## 11. Model fidelity is visible

For simulation evidence, retain declared limitations:
- unsupported peripheral behavior;
- approximate timing;
- simplified physics;
- missing analog effects;
- sensor model assumptions;
- non-cycle-accurate CPU;
- simplified power/thermal behavior.

These limitations can be represented as:
- ModelArtifact metadata;
- Assurance Case assumptions for high-risk work;
- Procedure limitations.

---

## 12. Continuous virtual qualification

Virtual environments are ideal for future Continuous Qualification:
- nightly simulation matrices;
- fuzzing;
- randomized scenarios;
- protocol stress;
- large variant coverage.

Only regressions needing physical confirmation consume HIL/device capacity.

This follows the same future model already identified from OSS-Fuzz/ClusterFuzz.

---

## 13. Fault injection and fuzzing

Tools such as syzkaller illustrate continuously generated adversarial inputs and automatic regression discovery.

engineering-platform may later treat:
- fuzz campaigns;
- randomized simulation;
- fault-injection runs

as Continuous Qualification Producers.

Findings attach to exact Artifact/Release and can trigger:
- Incident;
- Quarantine;
- Work;
- re-verification.

---

## 14. M1 impact

None on the critical path.

Reserve in schemas:
- VerificationEnvironmentClass;
- optional environment definition ref on TestVariant/Evidence.

M1 CI can use HOST_SIMULATED/normal CI as appropriate.

---

## 15. M2 impact

During embedded pilot:
- classify each test by environment;
- identify tests that can move off physical hardware;
- evaluate Renode/QEMU where target support exists;
- keep HIL/physical requirements explicit;
- measure device-time reduction.

For PCR02-style systems:
- MCU logic/protocol may benefit from virtual platforms where supported;
- Linux service/rootfs tests may use QEMU where realistic;
- robot motion/navigation can use simulation;
- motor electrical/thermal/charging acceptance remains physical/HIL.

---

## 16. New invariants

1. **Every authoritative test Evidence identifies its verification environment/fidelity.**
2. **Simulation/emulation evidence is not implicitly equivalent to hardware evidence.**
3. **Environment equivalence is an explicit scoped Decision.**
4. **Simulator/model version and configuration are part of the subject.**
5. **Resolved Verification Plan may use a fidelity ladder to minimize scarce HIL use.**
6. **Hardware-sensitive acceptance remains bound to appropriate physical evidence.**
7. **Virtual qualification can broaden coverage without weakening final release gates.**

---

## 17. Conclusion

Round 10 provides a practical path to scale embedded verification:

> **Use the cheapest environment that can validly prove a property, and explicitly require higher-fidelity evidence where the property depends on real hardware.**

This reduces HIL/device bottlenecks while preserving evidence semantics and release trust.
