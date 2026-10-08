# Runtime Skill guidance for typed Codex Tasks v1

Status: **SOURCE-BOUND PROMPT DELIVERY; NOT REAL SKILL-EFFECTIVENESS QUALIFICATION**

This is a narrow extension of
[Typed embedded TargetContext and Skill contracts](EMBEDDED_TARGET_ROUTING_V1.md)
and remains subordinate to
[Implementation Status](../status/IMPLEMENTATION_STATUS.md).

## Problem

Task intake already freezes the selected Skill IDs, methods, metadata and exact
`skill_contract_digest` into TaskContract for a declared TargetContext.
However the previous Codex prompt listed only Skill IDs, so a real engineering
turn was **not actually given the corresponding methodology, required material,
BLOCK conditions or evaluation method**. This was a genuine handoff gap between
catalog metadata and model execution.

## New typed Task behavior

Before generating the actual text delivered by `turn/start`, the Codex
Worker calls `codexexec.Prompt` with its Core-authorized frozen Assignment.
For a new Task whose **optional `skill_guidance_version` equals 1** and whose
`skill_contract_digest` is present:

1. Verify the Task/Intent/RunInput and attested preparation as before.
2. Independently resolve **only selected ordered Skills** from the host binary
   catalog and verify the full v1 selected metadata digest equals the frozen
   Task field. Missing, duplicate, stale, unsupported-version or contradictory v1 identities
   reject before the model turn; version-zero historical subjects remain
   unchanged.
3. Include the selected Skills' version, owner, declared maturity, purpose,
   input/material contracts, methods, outputs, BLOCK/prohibited conditions and
   evaluation methods as a bounded host-generated guidance section.
4. Carry the selected metadata digest **and guidance version** as *optional* fields
   inside PromptIdentity, which remains bound to the resulting Codex receipt and
   Core-bound engineering Result. The real EngineeringReceipt separately retains
   the exact prompt **bytes digest**.
5. The existing `workspace-write`, `approvalPolicy=never`, no-network
   profile and Action Gateway authority remain unchanged. Guidance is not a
   new tool/profile, permission, credential, model role or automatic optimizer.

The Skill catalog is executable host code/data, not arbitrary information from
an external requirement. Untrusted ContextBundle text remains approved material
rather than authorization. The rendered prompt is still subject to the
existing 64-KiB bound. A future catalog edit with an unchanged Skill ID can
never silently change the method of an existing typed Task; a different host
binary and stale frozen digest fail closed instead of silently swapping methods.

## Historical contracts

When `skill_guidance_version` is absent (0), **even if an existing typed Task
already has both `target_platform` and `skill_contract_digest`**, the original
name-only prompt and PromptIdentity bytes remain unchanged. New typed Tasks
receive version 1 from the Core; untyped Task requests have that field reset
to zero. A caller may not grant itself version 1. No historical Task, M1 Feature/Debug result,
Closure or Codex session is rewritten; old model turns are not replayed to add
Skill guidance.

## What is and is not proven

Go tests cover exact prompt bytes for untyped and earlier typed/digested tasks,
selected-order rendering,
known selected BSP methods/blocks, omission of unrelated MCU Skills, locator
independence, direct method-digest mismatches, and *internally self-consistent*
forged Task/Intent/Preparation identities. The last case must reject before
model execution, not just because the Task digest was obviously broken.

These tests show what the host **would send** in a Core-bound Codex turn. They
do **not** prove a live model read, obeyed, or correctly applied the methods,
nor measured reduced engineering cycle time or improved defect detection. The
ten Skills remain `DEFINED`. A separate retained, independently evaluated
real engineering pilot with source/expected outputs, negative cases, model
provenance and comparative quality evidence is still required to progress to
`EVALUATED`, `PILOTED` and `PROVEN` as appropriate.

BLOCK text is *reporting guidance*, not a new machine-verified acceptance gate
for every natural-language clause. Existing Material Readiness and Core/Action
Gateway remain the actual hard gates; never interpret a prompt statement as
device flashing, CI bypass or production readiness permission.
