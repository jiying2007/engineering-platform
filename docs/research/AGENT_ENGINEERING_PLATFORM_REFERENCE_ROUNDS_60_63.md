# Agent Engineering Platform Reference — Rounds 60–63

Date: 2026-09-29  
Status: **Research / extension reference; not M1 Core implementation authority**

This document continues the 2026 agent-platform and embedded Device/HIL research.

## 1. Round 60 — Agent execution provenance, forensics and action supply chain

### AgentProvenance

Reference:
- https://github.com/ByteYellow/AgentProvenance

AgentProvenance correlates model intent, application context and runtime telemetry into a content-addressed evidence graph. Its important contribution is not generic observability, but causal binding:

    task / peer message / tool call
      -> process / child process
      -> file / network / runtime effect
      -> artifact / risk signal
      -> replay / forensics / audit manifest

Relevant mechanisms:
- command-level recording without requiring the agent application to own the evidence model;
- process/file/network correlation, including Linux eBPF where available;
- explicit distinction between kernel/runtime-observed facts and application/AI-asserted context;
- confidence/trust degradation rather than silently upgrading asserted context into observed fact;
- content-addressed, hash-verified and optionally signed evidence;
- offline import/replay of signed forensics bundles;
- taint/quarantine/risk signals kept separate from final external decisions.

Mapping to engineering-platform:

    Runtime telemetry          -> Evidence candidate / audit enrichment
    application context        -> correlation metadata
    kernel-observed effect     -> stronger observed fact
    signed replay bundle       -> Artifact / forensic evidence package
    risk/deviation signal      -> Finding, never automatic Verification verdict

The key rule to absorb is that **provenance quality is source-aware**. A model assertion, application hook, process monitor and kernel sensor do not carry the same trust semantics.

### Agent Flight Recorder / Agent SLSA-style research

References:
- https://github.com/mpi-dsg/agent-flight-recorder
- https://github.com/AUTHENSOR/AUTHENSOR/blob/main/docs/standards/agent-slsa.md

These projects/spec drafts explore tamper-evident action histories and maturity levels for proving how an agent action was authorized, executed and recorded.

Useful semantic fields include:
- intent;
- policy evaluation;
- human approval where applicable;
- execution identity;
- observed effect;
- context provenance;
- code provenance;
- delegation provenance.

engineering-platform already owns most of these concepts across TaskContract, authenticated principal, ActionRequest, ActionReceipt, Audit, Evidence and Review. Therefore no parallel Agent-SLSA authority should be created.

Potential future export:

    Core authority facts
      -> agent-action provenance statement / attestation
      -> optional external transparency checkpoint

This is analogous to SLSA-compatible export: interoperability projection, not the mutable source of truth.

## 2. Round 61 — Reversible and forkable agent execution

### Shepherd

Reference:
- https://github.com/shepherd-agents/shepherd

Shepherd treats agent work as durable, inspectable execution traces whose workspace outputs can be forked, replayed, compared, accepted or discarded.

Important patterns:
- execution outputs are retained as proposals rather than immediately mutating the user's workspace;
- explicit permission surface;
- copy-on-write execution/environment forking;
- replay/fork from prior points for supervisor/meta-agent experiments;
- retained trace is separable from final apply/release;
- OS-level sandbox grant enforcement on supported platforms.

Potential engineering-platform benefit:

    Checkpoint
      -> retained workspace state
      -> forked RunAttempt / experiment branch
      -> compare outputs/evidence
      -> select one candidate for publication

This is especially useful for:
- competing debug hypotheses;
- parameter/config alternatives;
- two-agent implementation comparisons;
- failure reproduction experiments;
- review of speculative changes before publication.

Important boundary:

> Reverting a workspace trace is not rollback of GitHub, CI, devices, power supplies, firmware flash, releases or other external effects.

External side effects stay under the existing reconciliation model. A future fork/replay feature must mark the point after which an execution is no longer purely reversible.

## 3. Round 62 — Remote HIL lab and device-farm control plane

### Jumpstarter

References:
- https://github.com/jumpstarter-dev/jumpstarter
- https://jumpstarter.dev/main/introduction/service.html
- https://github.com/jumpstarter-dev/jumpstarter-lab-config

Jumpstarter is now one of the strongest open references for the future engineering-platform Device/HIL resource plane.

It provides:
- one API model for physical and virtual DUTs;
- UART, CAN, SPI, GPIO, power, USB and other hardware drivers;
- PyTest/Python integration;
- centralized hardware management;
- authenticated clients/exporters;
- multi-tenant access;
- exclusive device leases;
- label-based device selection;
- gRPC routing/tunneling between clients and exporters;
- Kubernetes-native controller/CRD deployment;
- CI integration;
- GitOps-style lab configuration with lint/dry-run/apply.

Potential integration boundary:

    engineering-platform Core
       |
       +-- DeviceResourceProvider
                |
                +-- Jumpstarter adapter
                +-- labgrid adapter
                +-- local direct adapter

Jumpstarter may own allocation/connectivity mechanics. engineering-platform should continue to own:
- Work/Task intent;
- exact Target/Device requirement;
- allowed action ceiling;
- procedure identity;
- Evidence semantics;
- Verification;
- Review/Closure.

Do not mirror every Jumpstarter CRD into Core. Keep only canonical device/session/lease facts needed by the engineering authority.

### Important lease invariant

Jumpstarter reinforces the distinction between:

    Resource lease state
        !=
    Physical device state

Lease loss/expiry means access ownership changed; it does not prove that firmware, power, bus state or mechanical state reverted.

## 4. Round 63 — Vendor-neutral hardware capability and safety description

### Open-MHS

Reference:
- https://github.com/Abenor-Labs/Open-MHS

Open-MHS is an independent open implementation of the emerging Model Hardware Standard idea. It describes hardware capabilities and safety limits in machine-readable capability tags and intercepts commands before bytes reach the device.

Important patterns:
- device declares what it can read/write and the exact safety envelope;
- limits travel with the hardware descriptor rather than living in prompts;
- one enforcement path shared by MCP/CLI/code surfaces;
- actionable typed refusal rather than generic transport failure;
- state-desynchronization is explicit when commanded and observed state diverge;
- hash-chained audit;
- emergency stop primitives;
- driver/transport separation;
- negative corpus that proves unsafe writes are actually blocked;
- planned capability-tag compatibility/conformance suite.

Potential future engineering-platform model:

    DeviceCapabilityProfile
       - device class / revision
       - observation capabilities
       - mutation capabilities
       - units / ranges / rates
       - safety envelope
       - enforcement level
       - emergency/safe-state semantics
       - driver/tool identity
       - capability schema version/digest

Such a profile can narrow a TaskContract/DeviceSession action ceiling. It must never widen policy on its own.

### Safety hierarchy

Machine-readable software bounds are valuable but do not replace independent hardware safety for energetic systems.

Recommended hierarchy:

    physical interlock / current-limit / watchdog / safe-state hardware
                  ↓
    operator-owned bench/device policy
                  ↓
    capability descriptor / driver enforcement
                  ↓
    Action Gateway authorization
                  ↓
    agent-facing tool schema / prompt

Each lower layer may further restrict the operation. None may relax the layer above it.

## 5. Combined architecture result

Rounds 60–63 suggest four optional seams without changing M1:

    ExecutionEvidenceProvider
        -> AgentProvenance/eBPF/audit sources

    ReversibleWorkspaceProvider
        -> checkpoint/fork/replay substrate

    DeviceResourceProvider
        -> Jumpstarter/labgrid/local lab

    DeviceCapabilityProfile
        -> MHS-like hardware capability/safety declaration

These abstractions should only be introduced when a real second implementation or M2 pilot needs them.

## 6. Recommended activation order

After retained Feature and Debug Closure chains:

1. first Device/HIL slice: exact board -> controlled flash -> serial capture -> deterministic assertion -> Evidence -> Verification;
2. add Jumpstarter only when remote/shared lab scheduling becomes a real need;
3. add DeviceCapabilityProfile before exposing generic actuator/instrument mutation to agents;
4. add provenance enrichment after defining which runtime observations materially improve audit/forensics;
5. add fork/replay when debug or candidate-comparison workflows prove the value;
6. export agent-action attestations only after Core facts are stable enough to avoid creating a second truth set.
