# Agent Engineering Platform Reference — Rounds 44–49

Date: 2026-09-28  
Status: **Research / extension reference; not Core implementation authority**

## 1. Purpose

This research extends the existing open-source reference corpus with projects that
have matured rapidly around AI coding-agent operations, provider-neutral runtime
interop, secure sandboxing, multi-agent coordination and operator UX.

It does **not** change the current M1 authority model.

The current Core remains authoritative for:

- WorkItem / TaskContract / Material readiness;
- Run / RunAttempt / Session / Steering / Checkpoint;
- privileged Action Gateway and reconciled ExternalOperation;
- exact Artifact / DeliveryReceipt / Evidence;
- Verification / independent Review / Closure.

External projects described here are replaceable mechanisms or UX references,
not engineering truth sources.

## 2. Executive conclusion

The most useful new reference set is:

1. **NVIDIA OpenShell** — secure agent runtime and policy-enforced sandbox boundary.
2. **Agent Client Protocol (ACP)** — provider-neutral coding-agent session protocol.
3. **Paperclip** — agent operations / organization / budget / activity control-plane UX.
4. **Kubernetes Agent Sandbox** — scale-out sandbox allocation and warm-pool patterns.
5. **OpenHands Software Agent SDK** — agent-server / workspace / conversation runtime APIs.
6. **BoxLite** — local/embedded microVM execution with OCI and persistent agent state.
7. **Agent Commons** — coordination primitives for independently started coding agents.
8. **Open Orchestrator / Cezar / Agetor / AGX** — parallel-worktree operator UX patterns.
9. **Clade** — provider-neutral delivery/evidence/verification concepts worth comparing
   with the existing Assurance layer.

The important boundary is:

> External agent/runtime products may provide execution, isolation, session protocol,
> coordination or UX, but engineering-platform Core remains the engineering authority.

## 3. Round 44 — Agent Operations Control Plane

### Paperclip

Reference:
- https://github.com/paperclipai/paperclip
- https://github.com/paperclipai

Observed product direction:

- bring-your-own agents and runtimes;
- goals and task assignment;
- agent heartbeats and activity;
- budgets and cost tracking;
- org/team representation;
- governance and operator dashboard.

Useful abstraction:

```text
Agent identity
  -> Runtime connection
  -> Capability / grant
  -> Task assignment
  -> Heartbeat / wake
  -> Run activity
  -> Usage / cost
  -> Audit / operator intervention
```

Recommended absorption:

- agent/runtime presence projection;
- Connection/credential binding as an operations concern;
- activity timeline;
- budget/usage projection;
- pause/resume/terminate operator UX.

Do **not** map Paperclip task completion or manager approval onto Evidence,
Verification, Review or Closure.

### Factory-style execution control planes

Use lightweight coding-agent control planes as a complexity benchmark for
Run/Worker/lease/retry/scheduling semantics. Their value is mainly to detect
over-design in our Execution Control, not to replace the Core authority model.

## 4. Round 45 — Secure Agent Runtime

### NVIDIA OpenShell

Reference:
- https://github.com/NVIDIA/OpenShell
- https://github.com/NVIDIA/OpenShell/blob/main/architecture/security-policy.md
- https://github.com/NVIDIA/OpenShell/blob/main/architecture/sandbox.md

Important mechanisms:

- sandbox / supervisor separation;
- kernel-enforced filesystem policy;
- unprivileged agent process and syscall controls;
- controlled outbound network policy;
- endpoint-bound provider credentials;
- policy changes reviewed against the access they introduce;
- Docker/Podman/MicroVM/Kubernetes compute options.

Potential engineering-platform seam:

```text
SandboxProvider
  ├─ NativeLinux
  ├─ OpenShell
  └─ KubernetesAgentSandbox
```

Core invariants stay unchanged: sandbox permission is not Action Gateway authority,
and runtime output is not Verification.

### BoxLite

Reference:
- https://github.com/boxlite-ai/boxlite

Useful mechanisms:

- hardware-isolated microVM per agent;
- OCI image compatibility;
- persistent/resumable execution;
- controlled networking;
- secret placeholders;
- daemonless embedded-library mode.

Treat as a future local strong-isolation backend candidate, not an M1 dependency.

## 5. Round 46 — Coding Agent Interoperability

### Agent Client Protocol (ACP)

References:
- https://github.com/agentclientprotocol
- https://github.com/agentclientprotocol/agent-client-protocol
- https://github.com/agentclientprotocol/codex-acp

ACP standardizes client/editor to coding-agent communication.

The Codex ACP adapter currently maps Codex App Server behavior into ACP, including
session lifecycle, prompts/context, shell/file events, permission requests, MCP
tools, terminals, plans, reasoning, token usage and review-related events.

Potential seam:

```text
AgentRuntimeAdapter
  ├─ NativeCodexAdapter
  ├─ ACPAdapter
  │    ├─ codex-acp
  │    └─ other ACP agents
  └─ OpenHandsAdapter
```

Required semantic firewall:

- ACP session ID != Run identity;
- ACP completion != Verification;
- ACP permission != privileged Action Gateway authorization;
- ACP tool result != Evidence unless imported through an exact Evidence adapter.

## 6. Round 47 — Agent Sandbox Scale

### Kubernetes Agent Sandbox

Reference:
- https://github.com/kubernetes-sigs/agent-sandbox

Use as a future Worker/Sandbox pool reference for:

- sandbox resource allocation;
- claim/template models;
- warm pools;
- stronger runtimes such as gVisor/Kata where appropriate;
- suspend/resume and scale-out scheduling patterns.

Do not introduce Kubernetes into M1 solely for architectural symmetry.

### Coder

Reference:
- https://github.com/coder/coder

Continue using Coder as a remote/cloud workspace lifecycle reference. It is a
WorkspaceProvider-style mechanism, not engineering authority.

## 7. Round 48 — Multi-Agent Coordination

### Agent Commons

Reference:
- https://github.com/t54-labs/agent-commons

Useful primitives:

- human-attributed agent identity;
- peer discovery and durable messages;
- plans/tasks as coordination state;
- resource leases with TTL;
- fencing epochs;
- audit history;
- project/workspace enrollment.

This aligns well with the platform rule that coordination does not imply approval
or verified truth.

Activate only when multiple independent agent sessions start contending for
shared repositories, devices, build servers or deployment environments.

### Clade

Reference:
- https://github.com/shenxingy/clade

Clade describes a provider-neutral delivery control plane around coding agents,
immutable evidence, calibrated verification and delivery state.

Use it as an Assurance architecture comparison. Do not introduce a parallel
Evidence/Verification authority model.

## 8. Round 49 — Agent Operator UX

### Open Orchestrator

Reference:
- https://github.com/gitpcl/openorchestrator

Useful UX patterns:

- worktree-per-task;
- provider-neutral agent supervision;
- decision-focused lanes such as NEEDS YOU / READY TO SHIP / IN FLIGHT;
- early worktree conflict detection.

### Cezar

Reference:
- https://github.com/open-mercato/cezar

Useful UX/mechanism patterns:

- parallel worktrees;
- local or remote long-running execution;
- visible tool calls/token/cost;
- run the same task through multiple agents and compare diffs;
- small YAML workflows and Markdown skills;
- GitHub issue -> agent -> diff/PR flow.

### Agetor

Reference:
- https://github.com/alamops/agetor

Useful UX patterns:

- approvals and clarification lifted out of terminal/TUI into structured cards;
- persisted run/event history;
- reproducible base-SHA-bound worktrees;
- multiple accounts/harnesses.

### AGX

Reference:
- https://github.com/nashory/agx

Useful UX patterns:

- durable local sessions;
- worktree or direct checkout execution;
- Desktop + CLI/TUI surfaces;
- live follow-up / interrupt / restart lifecycle.

These projects should influence WorkBuddy/Console/eng UX, not mutate Core facts.

## 9. Recommended architecture boundary

```text
             WorkBuddy / IDE / eng CLI
                       |
             Experience / Agent Ops
                       |
================================================
           engineering-platform CORE
================================================
 Work / Task / Material / Skill / VerificationPlan
 Run / Attempt / Session / Steering / Checkpoint
 Action Gateway / ExternalOperation
 Artifact / Evidence / Verification / Review / Closure
================================================
           |                         |
  AgentRuntimeAdapter        SandboxProvider
           |                         |
    Codex / ACP / ...       Native / OpenShell /
                           K8s Agent Sandbox / ...
```

## 10. Adoption posture

For the current M1 milestone:

1. do not re-platform the retained Feature/Debug pilots;
2. keep these results research-only;
3. after real retained Feature and Debug Closure chains exist, run bounded spikes:
   - ACP AgentRuntimeAdapter;
   - OpenShell SandboxProvider;
4. use Paperclip and the operator-control projects primarily for Experience /
   Agent Operations design;
5. activate multi-agent coordination primitives only when real contention proves
   the need.

## 11. Research rule reinforced

A popular agent product is not itself a reason to add a dependency.

Promote a mechanism into Core or an activated Extension only when it:

- closes a proven semantic or operational gap;
- removes meaningful bespoke infrastructure;
- establishes useful interoperability;
- or addresses a failure mode demonstrated by real retained pilots.
