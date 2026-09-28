# Agent Engineering Platform Reference — Round 59 (Hardware-Gated Agent Verification)

Date: 2026-09-28  
Status: **Research / future Device-HIL extension reference; not M1 Core authority**

## 1. Agentic HIL

Reference:
- https://github.com/agentic-hil/agentic-hil

Agentic HIL is a particularly relevant reference because it treats the real board as a verification gate rather than merely exposing raw host/debugger access to the coding agent.

Notable mechanisms:
- bounded MCP tools for probing, flashing, reset, artifact validation, UART/CAN stimulus and observation;
- authoritative project configuration stored outside the repository/workspace so the agent cannot rewrite its own hardware policy;
- probe/backend abstraction rather than hard-coding one board family;
- run reports intended for reviewer consumption;
- lease release and safe-state reporting;
- explicit negative-path demonstration: a test plan should be shown to fail against a wrong expectation, because a test that cannot fail is not meaningful evidence;
- avoidance of arbitrary debugger/host shell exposure as the default hardware interface.

These patterns map closely to engineering-platform's future Device/HIL extension.

## 2. Operator-owned hardware policy

The strongest pattern to absorb is that device authority configuration should not be mutable by the same workspace/model it constrains.

Future structure:

    operator-owned DevicePolicy / BenchProfile
          |
          +-- allowed DeviceInstance
          +-- allowed probe / interface
          +-- allowed operations
          +-- physical limits / safe state
          +-- recovery procedure
          +-- test/procedure ceiling
          |
    Core-bound immutable snapshot/digest
          |
    DeviceSession / DeviceActionRequest

Agent-generated repository files may request a test plan, but they cannot expand the bench/device action ceiling.

## 3. A hardware test must be able to fail

A green-only script is not strong Verification evidence. A future Procedure qualification should demonstrate at least:
- one known-positive case passes;
- one deliberately wrong expectation or seeded failure fails;
- the failure is attributed to the intended assertion rather than infrastructure noise;
- rerun identity and raw artifacts are retained.

This is analogous to qualification of a test oracle before relying on its PASS result.

## 4. Bench instruments are privileged actuators as well as sensors

References:
- https://github.com/colingimenez/SCPI_MCP
- https://github.com/masahiro-999/oscilloscope-mcp

An oscilloscope measurement is primarily observation, but SCPI-capable benches often also expose:
- power-supply enable/voltage/current changes;
- waveform-generator output;
- trigger/acquisition state;
- instrument reset/configuration;
- raw SCPI passthrough.

The protocol itself may provide little or no authentication. Therefore network reachability or MCP discovery must never imply authorization.

Instrument tools should distinguish:

    MeasurementQuery        -> OBSERVE
    AcquisitionConfig       -> CONTROLLED_STATE_CHANGE
    Stimulus / PSU output   -> PRIVILEGED_MUTATION
    Raw passthrough         -> deny by default or tightly bounded profile

For energized DUTs, a separate hardware current/voltage/power limit remains mandatory.

## 5. Vehicle / bus stimulus

Reference:
- https://github.com/farzadnadiri/MCP-CAN

CAN/UDS/J1939 agent tools illustrate another important boundary: passive bus observation and diagnostic/service requests are materially different actions. Fault injection, ECU routines, writes and actuator commands need explicit semantic policy rather than a blanket CAN permission.

## 6. Suggested Device/HIL authority chain

    TaskContract
      -> approved Device/Bench Context
      -> DeviceSession lease
      -> operator-owned policy snapshot
      -> bounded DeviceActionRequest
      -> Action Gateway / Device executor
      -> ActionReceipt
      -> raw capture Artifact(s)
      -> Evidence importer
      -> Verification
      -> Review when required

UNKNOWN or timeout after a flash, reset, stimulus or power operation is not blindly retried. The platform first observes actual device/bench state and reconciles.

## 7. No new M1 blocker

Round 59 reinforces the planned M2 Device/HIL direction but does not change the current M1 acceptance gate. Finish retained Feature and Debug Closure chains first, then implement the narrow board -> flash -> serial assertion -> Evidence slice before higher-risk autonomous lab control.