# Frozen Requirement Context

Source: https://github.com/jiying2007/engineering-platform/issues/54
Issue: #54 M1 retained Feature pilot: subsystem-aware embedded routing
Snapshot: 2026-09-26

## Pilot role

Candidate retained **Feature** pilot for M1. This is a real embedded-domain behavior change, not a marker/documentation-only task.

## Problem

`internal/routing.Resolve` currently uses the subsystem for DEBUG routing, but FEATURE / BRINGUP / COMPATIBILITY always return only:

- `embedded.architecture`
- `embedded.driver-component`

That means a Feature explicitly targeting Linux/BSP/storage or MCU/RTOS/motor loses the domain-specific capability in the frozen route.

## Required change

Keep the existing architecture + driver/component route, and add subsystem-aware integration routing for the existing embedded domains:

- Linux/BSP/storage/UBI -> include `embedded.linux-bsp` and an explicit Linux/BSP integration skill.
- MCU/RTOS/motor -> include `embedded.mcu-rtos` and an explicit MCU/RTOS integration skill.
- generic/unknown subsystem -> preserve the current generic route.

Do not add a new extension domain. Keep capability/skill ownership inside the existing embedded catalog.

## Acceptance criteria

1. FEATURE with subsystem `Linux UBI storage` retains architecture + driver/component and also routes through the Linux/BSP capability with an integration-appropriate skill.
2. FEATURE with subsystem `MCU motor control` retains architecture + driver/component and also routes through the MCU/RTOS capability with an integration-appropriate skill.
3. BRINGUP and COMPATIBILITY use the same subsystem-aware rule.
4. Generic FEATURE routing remains backward-compatible with the existing generic route.
5. Existing DEBUG routing behavior remains unchanged.
6. `go test -race ./...`, repeated regressions, vet/build and repository CI all pass.

## Retained-pilot constraints

- The frozen Task must allow only `worker.codex-execute` and `github.publish-pr` in addition to ordinary read/control permissions required by the existing platform path.
- The Codex Worker must not receive GitHub write credentials.
- The result PR must not modify `.github/workflows/ci.yml` so exact PR-head trusted CI remains admissible.
- Delivery.result_commit must remain the original Codex result commit.
- Require Codex execution, Git changed-tree and trusted-CI evidence before Verification.
- Independent Review must PASS before Closure.

## Base

Do not freeze the Task until execution begins. At that point use the exact current `main` SHA and stop if `main` moves before publication.
