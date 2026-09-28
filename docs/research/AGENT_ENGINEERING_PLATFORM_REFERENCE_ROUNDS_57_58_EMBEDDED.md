# Agent Engineering Platform Reference — Rounds 57–58 (Embedded / Device)

Date: 2026-09-28  
Status: **Research / future Device-HIL extension reference; not M1 Core authority**

This document continues the general agent-platform research with a deliberately embedded-specific scan.

## 1. Round 57 — Embedded-first agent engineering loop

### Firment

Reference:
- https://github.com/MoRiv447/Firment

Firment is currently one of the closest open-source projects to the product journey engineering-platform ultimately wants to support for MCU/firmware work.

Its loop is approximately:

    requirement
      -> inspect board/chip/peripheral context
      -> edit firmware
      -> real toolchain build
      -> flash through probe
      -> serial/register/runtime observation
      -> ELF/resource analysis
      -> on-target debug when needed
      -> optional logic-analyzer / image observation
      -> repeat until observed behavior matches expectation

Relevant mechanisms include:
- real build/flash/monitor integration;
- probe-rs based target flashing/debugging;
- ELF flash/RAM/stack regression checks;
- replayable HIL suites;
- serial expectation assertions;
- logic-analyzer captures and deterministic measurements;
- photo-based physical observation;
- explicit evidence levels from code through physical behavior;
- fault-signature detection feeding debug/forensic steps;
- permission gates for write/edit/shell and read-only plan mode.

### What to absorb

Firment validates several assumptions already present in engineering-platform:
1. embedded completion cannot stop at source-code generation;
2. build success does not imply deployment success;
3. flash success does not imply runtime correctness;
4. runtime logs do not necessarily prove physical behavior;
5. physical observations should be captured as artifacts rather than summarized only in model prose;
6. HIL procedures should be repeatable and replayable.

### Where engineering-platform should remain stricter

Firment is primarily an embedded coding-agent runtime. engineering-platform must retain stronger authority separation:

    Agent/Runtime observation
            !=
    EvidenceRef
            !=
    VerificationReport
            !=
    Review / Closure

A tool saying flash succeeded, serial matched or LED observed becomes formal Evidence only after the platform binds it to exact identities and retains the underlying artifact/receipt.

Minimum future evidence binding should include, as applicable:
- DeviceInstance / board identity;
- TargetRevision / hardware revision;
- firmware Artifact digest and source commit;
- build/toolchain identity;
- probe/debugger identity and configuration;
- procedure/test revision;
- raw serial/GDB/RTT/trace/log artifact digest;
- logic-analyzer/photo/camera artifact digest;
- monotonic/UTC timing facts and timebase assumptions;
- issuer/Worker identity;
- environment and calibration identity where measurement matters.

### Adoption posture

Do not import Firment as Core. Use it as an end-to-end embedded UX reference, a source of candidate Device/HIL contracts, a corpus of embedded failure modes, and a comparison target for future embedded Skills. Its project maturity means architectural ideas should be independently validated before adoption.

## 2. Round 58 — AI-to-hardware tool adapters

### Renode MCP proposal

Reference:
- https://github.com/renode/renode/issues/975

The 2026 Renode proposal exposes local MCP tools to start/stop Renode, load platform/firmware, issue monitor commands, inspect virtual time/state, validate ELF files and open UART analysis.

Important boundary already stated by the proposal itself: simulation evidence does not replace final HIL/physical-board evidence. This fits the existing VerificationEnvironmentClass/fidelity-ladder model.

### embedded-mcp

Reference:
- https://github.com/atillab1/embedded-mcp

Representative tool surface includes serial enumeration/read/write, target commands, SVD/register decoding, flasher discovery and flash/debug-related operations.

### Serial MCP servers

References:
- https://github.com/es617/serial-mcp-server
- https://github.com/HumbertoBernal/mcp-serial

These projects show that serial access is no longer just terminal UX: agents can own persistent device sessions, reset targets, send protocol commands and automate smoke-test flows. They also expose the safety problem clearly: a serial write can trigger a bootloader, actuator or destructive device action.

### Embedded GDB MCP

Reference:
- https://github.com/ezulabs/embeddedgdbmcp

This demonstrates structured agent control of an embedded target through GDB/OpenOCD-style debug infrastructure: registers, cores, breakpoints, backtraces and live target analysis.

### PlatformIO MCP

Reference:
- https://github.com/jl-codes/platformio-mcp

Provides an example of exposing board discovery, builds, uploads and monitoring through an agent tool protocol, with hook-based guardrails such as requiring a successful build before upload.

### Hardware MCP

Reference:
- https://github.com/hardware-mcp

The project direction extends the same concept beyond MCUs to oscilloscopes, power supplies, signal generators, JTAG/SWD, CAN and Modbus.

### Underlying non-agent mechanisms remain important

References:
- https://github.com/probe-rs/probe-rs
- https://github.com/openocd-org/openocd
- https://github.com/labgrid-project/labgrid
- https://github.com/renode/renode

Do not require an MCP wrapper when a stable native API/CLI/library provides a better deterministic integration. MCP is one possible Runtime-facing adapter, not the canonical Device/HIL authority.

## 3. Future Device/HIL action classification

The main architectural result is that hardware tool is too coarse a permission category. A future Device/HIL extension should classify operations by semantic effect.

### OBSERVE

Examples: read serial/RTT, collect logs, read non-intrusive state, inspect ELF, capture camera/logic-analyzer data, read instrument measurements. Normally produces observation artifacts and may be allowed by a bounded DeviceSession/ToolProfile.

### CONTROLLED_STATE_CHANGE

Examples: halt/resume/single-step, breakpoint changes, reset device, debugger-state changes, bounded device commands, entering test mode. These mutate execution state even when firmware bytes do not change. They require explicit policy and a retained operation receipt.

### PRIVILEGED_MUTATION

Examples: erase/program flash, change option bytes/fuses/security state, unlock protection, arbitrary register/memory writes, bootloader/partition mutation, actuator/power-rail control, instrument output changes, destructive protocol commands.

These belong behind the Action Gateway or a Device Action Gateway projection, with exact target binding, action ceiling, approval where required, timeout, UNKNOWN reconciliation and a post-action observed-state snapshot.

## 4. Resource ownership and concurrency

Real hardware is exclusive/shared state, unlike a disposable filesystem. Future Device/HIL work should bind:

    DeviceInstance
      + DeviceSessionLease
      + ToolProfile
      + target firmware identity
      + procedure identity
      + execution/recovery epoch

Lease expiry alone must not imply that a physical side effect rolled back. Resource allocation/lease state and observed physical state are different facts.

## 5. Physical safety boundary

For motors, batteries, heaters, power electronics and other energetic systems, software policy must not be the only safety mechanism.

The platform should support declarations such as maximum voltage/current/speed/temperature, independent hardware/current-limit/watchdog/interlock, emergency-stop/recovery procedure, safe-state definition, prohibited autonomous actions and human-presence requirements where appropriate.

Agent approval does not override an independent physical interlock.

## 6. Extension activation recommendation

Do not make Rounds 57–58 an M1 blocker.

When M2 Device/HIL starts, the best first vertical slice is deliberately narrow:

    exact board
      -> exact firmware
      -> build
      -> controlled flash
      -> serial capture
      -> one deterministic assertion
      -> raw Artifact
      -> Evidence import
      -> independent Verification

Only after this chain is retained should the platform add live debugger writes, logic analyzers, instruments, camera observations or autonomous recovery.

That order preserves the current architecture instead of turning MCP access into an uncontrolled hardware super-user surface.
