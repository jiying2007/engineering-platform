# Typed embedded TargetContext and Skill contracts v1

Status: **REPOSITORY IMPLEMENTATION; REAL SKILL MATURITY STILL DEFINED**

This note is subordinate to the live authority at
[Implementation Status](../status/IMPLEMENTATION_STATUS.md).
It does not turn a CI fixture into validated field engineering expertise.

## Routing

The existing Work -> Task API and `eng work-intake` optionally accept
`target_context`:

```json
{
  "subsystem": "SSC305",
  "target_context": {
    "target_id": "ssc305",
    "platform": "linux-bsp"
  }
}
```

The `target_id` **must equal** the frozen `material.target_id`. Both the
local intake and the Core API recheck this exact identity before creating a Task.
`platform` is exactly `linux-bsp` or `mcu-rtos`; empty/unknown classes,
inconsistent Linux-vs-MCU subsystem hints, mixed family hints and malformed
target identifiers fail closed.

An opaque chip name such as SSC305 or MM32SPIN023C is **not an authority
source**. Routing follows the explicit, reviewed platform classification, not a
chip-name substring heuristic. Absent `target_context`, the existing M1
subsystem route is intentionally unchanged, so retained task subjects are not
silently rewritten. The selected Capability/Skill IDs **and exact target platform** remain bound
into the immutable TaskContract digest. `target_platform` is optional for
historical or untyped tasks and is emitted only for a declared TargetContext;
Core overrides any prefilled contract value with its verified declaration.

The target declaration does not grant Device/HIL, flashing, signing, Git push,
model or production permissions; the existing Material, Task, Action Gateway
and independent Verification constraints still apply.

## Defined Skill contracts

The existing ten embedded Skills now publish explicit version, input, required
material, method, output, blocking conditions, prohibited actions and
evaluation method alongside ownership and maturity. These fields are
**read-only guidance**, not a second permission language. `allowed_actions`
is deliberately empty at this stage; privileged operations remain authorized
only by existing Core/Action Gateway policy.

Every Skill remains `DEFINED`. A test proving the metadata exists can never
promote `EVALUATED`, `PILOTED` or `PROVEN`. Each promotion requires
subject-bound real task evidence, independent outcome verification and repeated
value; use the domain capability maturity rules.

## Qualification

Required unit tests cover:
- typed Linux/BSP and MCU/RTOS routing for opaque chip names;
- exact Material/TargetContext equality in local work intake and Core;
- rejecting malformed, contradictory and mixed-family target declarations;
- preserving existing routes when typed target context is absent;
- complete explicit catalog contracts and independent copies returned to callers.

Device firmware boot/timing/electrical suitability, complete WorkBuddy UX,
production SLOs and effective expert quality are **not** implied by this patch.
