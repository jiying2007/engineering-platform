# Agent Engineering Platform Reference — Round 67 (Embedded Qualification Corpus)

Date: 2026-09-29  
Status: **Research / Runtime and Skill qualification reference; not release authority**

## 1. Purpose

General coding benchmarks do not exercise the implicit constraints that make embedded work difficult: ISR concurrency, DMA/cache coherency, RTOS timing, device tree/Kconfig, boot/storage/OTA, register semantics, power behavior and hardware fidelity.

Round 67 identifies public corpora that can seed engineering-platform's embedded Runtime/Skill qualification instead of building every case from scratch.

## 2. EmbedEval

Reference:
- https://github.com/Ecro/embedeval

EmbedEval covers production-oriented embedded domains including:
- Zephyr RTOS;
- ESP-IDF;
- STM32 HAL;
- FreeRTOS;
- Linux kernel drivers;
- Yocto recipes.

Useful methodology:
- prompts intentionally omit implicit safety/domain hints;
- multiple execution backends such as native simulation, QEMU and real build environments;
- generation and bug-fix cases;
- public plus held-out cases;
- repeated runs / confidence intervals;
- mutation negatives that must fail;
- category-level failure analysis rather than one overall number.

This is particularly relevant for engineering-platform because a runtime can look strong on general coding while still failing embedded-only invariants.

## 3. Closed-loop embedded agent evaluation

Reference:
- https://github.com/jgcarrasco/closed_loop_evaluation_agents_embedded

This benchmark evaluates coding agents inside constrained embedded workspaces with a hidden evaluator and deterministic plant/runtime checks.

Useful benchmark modes:
- one-shot blind;
- realistic self-verify using only visible local evidence;
- CI red/green feedback;
- oracle/full diagnostic feedback.

The most relevant mode is realistic self-verify:

> the agent must decide when the implementation is ready using the same visible evidence available in an ordinary engineering run, while hidden acceptance remains independent.

This maps well to engineering-platform's separation of Runtime claim from Verification.

## 4. HWE-bench

Reference:
- https://github.com/pku-liang/hwe-bench

HWE-bench contains real fail-to-pass hardware/RTL bug-fix cases across projects such as Ibex, CVA6, Caliptra, Rocket Chip, XiangShan and OpenTitan.

Although RTL is not the first engineering-platform product scope, the methodology is valuable:
- real historical bug-fix provenance;
- exact buggy base;
- independent simulation evaluator;
- fail-before / pass-after condition;
- per-case isolated environments;
- Harbor-compatible agent harness;
- external access restrictions for generated tasks.

Use it as a methodology reference for future BSP/driver/MCU historical-failure corpora.

## 5. Agentic Embedded Lab

Reference:
- https://github.com/eust-w/agentic-embedded-lab

AEL is an early/development project, but several principles are valuable:
- typed experiment/claim/evidence contracts;
- explicit capability gaps instead of silent fallback;
- model/simulator fidelity recorded with claims;
- simulator PASS never promoted into hardware-equivalence without real evidence;
- faulty/fixed benchmark pairs;
- multi-domain cases spanning firmware/RTOS, power, analog, thermal, network and RF/EM.

The strongest lesson is the explicit 'not proven' boundary.

## 6. Recommended qualification corpus layers

engineering-platform should not use one monolithic leaderboard.

Recommended layers:

    Layer A — general coding/runtime sanity
      Harbor / Terminal-Bench / small coding tasks

    Layer B — public embedded-domain probes
      EmbedEval-like ISR/DMA/RTOS/driver/build cases

    Layer C — closed-loop embedded engineering
      realistic self-verify + hidden independent evaluator

    Layer D — project historical failure replay
      frozen real bugs from our Linux/BSP/MCU/driver/OTA/debug history

    Layer E — real target/HIL retained proof
      exact board/device + procedure + raw Evidence + Verification

Only Layers D/E establish local engineering confidence for shipping decisions.

## 7. Corpus case contract

A reusable qualification case should bind at minimum:
- case id/version;
- source provenance/license;
- frozen source/base digest;
- visible Task/brief;
- editable scope;
- material supplied to the agent;
- hidden acceptance/oracle identity;
- expected initial failure where applicable;
- deterministic seed/environment/toolchain identity;
- network/tool permissions;
- timeout/action budget;
- raw outputs/patch/artifact identity;
- verdict plus failure classification.

## 8. Negative and mutation testing

A qualification suite is not credible if every case only demonstrates PASS.

Require cases proving that:
- buggy/faulty input fails before the fix;
- deliberately incorrect output is rejected;
- test tampering cannot make the hidden evaluator green;
- missing-material/BLOCK behavior is accepted when correct;
- infrastructure failure is distinguished from engineering failure;
- simulator-only evidence cannot satisfy a hardware-required case.

## 9. Initial corpus for engineering-platform

After M1, create a deliberately small retained corpus rather than chasing benchmark volume:

1. Linux/BSP boot or driver regression;
2. MCU ISR/concurrency bug;
3. DMA/cache or memory-alignment bug;
4. RTOS timing/deadlock case;
5. boot/storage/OTA failure;
6. log-triage/debug root-cause case;
7. one real-board flash + serial/HIL case.

For each case, preserve one known-bad snapshot and one independently verified known-good outcome.

## 10. Promotion rule

Public benchmark improvements are qualification Evidence, not release authority.

Runtime/Skill promotion should consider:

    public-domain qualification
      + project failure replay
      + exact RuntimeProfile
      + cost/latency/action discipline
      + retained real engineering outcomes

and remain scoped to the supported platform/target/tool envelope.
