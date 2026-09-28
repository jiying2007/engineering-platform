# Agent Engineering Platform Reference — Rounds 50–56

Date: 2026-09-28  
Status: **Research / extension reference; not Core implementation authority**

This document continues
[Agent Engineering Platform Reference — Rounds 44–49](AGENT_ENGINEERING_PLATFORM_REFERENCE_ROUNDS_44_49.md).

The search deliberately prioritizes projects that add a missing architectural
pattern or challenge an existing engineering-platform assumption. Popularity
alone is not an adoption criterion.

## 1. Round 50 — Safe agentic CI and mutation separation

### GitHub Agentic Workflows

References:
- https://github.com/github/gh-aw
- https://github.github.com/gh-aw/reference/safe-outputs/
- https://github.github.com/gh-aw/reference/permissions/
- https://docs.github.com/en/copilot/concepts/agents/about-github-agentic-workflows

Important pattern:

```text
untrusted / probabilistic reasoning
        |
 read-only agent job
        |
 structured requested effect
        |
 validate / constrain
        |
 separately permissioned write job
        |
 observable external mutation
```

Agentic Workflows compiles Markdown + frontmatter into generated GitHub Actions
workflows. Its normal security posture keeps the agent job read-only and
sandboxed. GitHub mutations are requested as validated "safe outputs" and applied
by separately scoped jobs that hold write permission and credentials.

This strongly reinforces the existing engineering-platform design:

```text
Runtime
  -> ActionRequest
  -> Action Gateway policy / ledger
  -> independently credentialed publisher/executor
  -> ActionReceipt / observed external state
```

**No new Core type is required.** A safe-output-style request is already
representable by ActionRequest. The useful result is an implementation and threat
model comparison for:

- credential non-exposure;
- structured effect requests;
- per-operation limits;
- suspicious-output validation;
- separate mutation identity;
- audit/reconciliation after the external effect.

Do not replace the Action Gateway with GitHub Agentic Workflows. The latter is a
GitHub automation mechanism; the former is engineering-platform's cross-system
authority boundary.

## 2. Round 51 — Sandbox/runtime substrate refresh

### OpenSandbox

Reference:
- https://github.com/opensandbox-group/OpenSandbox

OpenSandbox is a general-purpose sandbox platform for coding agents, GUI agents,
agent evaluation and code execution. It exposes SDK/CLI/MCP surfaces and an open
API contract while allowing multiple execution backends such as Docker and
Kubernetes.

Most relevant design idea:

> make sandbox lifecycle/execution a stable provider contract while keeping the
> concrete isolation backend replaceable.

This strengthens the proposed future `SandboxProvider` seam.

### Rivet agentOS

Reference:
- https://github.com/rivet-dev/agentos

agentOS explores a different point in the design space:

- V8 isolate + WebAssembly based lightweight guest execution;
- host functions keep host credentials outside guest code;
- default-denied outward capabilities;
- ACP-based durable coding-agent sessions;
- one transcript format across supported agents;
- optional mounting of a full external sandbox when a real OS is required.

For embedded engineering, a lightweight isolate cannot replace the real Linux
environment needed by toolchains, debuggers, Docker, USB/device access or vendor
utilities. However, the **two-tier execution idea** is useful:

```text
lightweight reasoning/tool VM
       |
       +-- request full engineering sandbox only when required
```

This is a possible future optimization, not an M1 requirement.

### Coder — revisit, not a new dependency

Reference:
- https://github.com/coder/coder

Coder has evolved from a remote development environment reference toward managed
environments for developers and their agents. Revisit it when engineering-platform
needs enterprise remote workspace templates, prebuilds, fleet policy or central
agent workspace governance.

### Daytona — historical API reference only

Reference:
- https://github.com/daytonaio/daytona

The public Daytona repository contains useful sandbox lifecycle, SDK and snapshot
patterns, but its README states that the open-source repository stopped receiving
core development in June 2026 and development moved to a private codebase.

Therefore:

- retain it as a sandbox API/history reference;
- do not select the public repository as a new platform dependency.

### NVIDIA NemoClaw

Reference:
- https://github.com/NVIDIA/NemoClaw

NemoClaw is useful evidence that OpenShell's security/runtime abstractions are
being exercised as a substrate for multiple agent families and managed inference.
It does not require a separate engineering-platform abstraction beyond the
OpenShell/SandboxProvider research already recorded.

## 3. Round 52 — Software factory decomposition and independent review

### Open SWE

Reference:
- https://github.com/langchain-ai/open-swe

Open SWE is an asynchronous software factory that accepts work through dashboard,
GitHub, Slack, Linear or schedules, executes in isolated workspaces, validates
changes and delivers pull requests. It separates implementation from read-only
review and CI-monitoring roles and fails safely rather than silently replacing an
unreachable sandbox that may contain uncommitted work.

Useful comparisons:

- durable execution/thread state;
- coding sandbox persistence;
- plan-before-code option;
- read-only reviewer;
- approval before sensitive workflow-file mutations;
- CI feedback loop.

### Vercel eve Software Factory / Foreman

Reference:
- https://github.com/vercel-labs/eve-software-factory-template
- https://github.com/vercel-labs/eve-software-factory-template/blob/main/.github/ARCHITECTURE.md

Foreman uses four stations:

```text
Classifier
  -> Analyst
  -> Implementer
  -> independent Reviewer
  -> draft PR
  -> human judgment
```

Notable details:

- Analyst derives plan and acceptance criteria from a live repository checkout;
- Implementer has its own sandbox and executes repository checks;
- Reviewer judges the pushed branch rather than Implementer chain-of-thought;
- Reviewer can use a different model vendor;
- merge is not in the agent tool surface;
- unattended mutation authority is deliberately narrow.

This is one of the strongest external confirmations of the current
engineering-platform invariant:

> Engineering != Verification != Review != irreversible authority.

However, engineering-platform should remain stricter: exact Evidence,
requirement-bound Verification and ClosureReceipts stay authoritative rather
than relying on station completion.

### AWS CLI Agent Orchestrator

Reference:
- https://github.com/awslabs/cli-agent-orchestrator

CAO coordinates full provider CLIs such as Claude Code, Codex and others through
isolated tmux sessions. It is valuable as a deliberately lightweight comparison
for supervisor/worker delegation and session orchestration.

Use it to challenge unnecessary orchestration complexity; do not use terminal
session state as formal engineering state.

## 4. Round 53 — Agent qualification and evaluation

### Harbor

References:
- https://github.com/harbor-framework/harbor
- https://github.com/harbor-framework/docs

Harbor provides a common evaluation harness across coding agents, models,
benchmarks and execution providers. It supports high-concurrency evaluation and
is the official harness for Terminal-Bench 2.0.

Potential mapping:

```text
RuntimeQualificationProfile
       |
       +-- public benchmark slice
       +-- project failure replay
       +-- exact runtime/model/tool profile
       +-- environment identity
       +-- retained result
```

Public benchmark scores must **not** become a release or engineering authority.
They qualify a runtime profile only within the scope of the benchmark.

### Terminal-Bench 2.0

Reference:
- https://github.com/harbor-framework/terminal-bench

Useful as a broad terminal/software-agent capability benchmark, especially for
shell navigation, debugging and autonomous tool use.

For engineering-platform it should be supplementary to embedded-specific replay:
cross-compilers, BSP builds, log triage, driver debugging, RTOS/MCU workflows and
future Device/HIL tasks are not represented by generic terminal benchmarks.

### SWE-smith

Reference:
- https://github.com/SWE-bench/SWE-smith

SWE-smith demonstrates how repositories can be converted into reproducible task
environments and generated software-engineering failure instances. The key idea
to absorb is **programmatically expanding a qualification/failure-replay corpus**
while retaining exact repository/environment identity.

### mini-swe-agent

Reference:
- https://github.com/SWE-agent/mini-swe-agent

mini-swe-agent is useful as a complexity baseline: modern models can achieve
substantial software-engineering capability with an intentionally small agent
loop. This is a reminder not to put capability into orchestration layers merely
because older agents needed it.

For engineering-platform:

- keep domain Skill semantics explicit;
- keep authority/evidence strict;
- keep the model/runtime loop as replaceable and as simple as practical.

## 5. Round 54 — Protocol stack: UI, agent, tool and registry boundaries

### AG-UI

Reference:
- https://github.com/ag-ui-protocol/ag-ui

AG-UI is an event-based agent/user interaction protocol with lifecycle,
streaming-text, tool-call, state-management, activity and subagent events.

Potential future seam:

```text
WorkBuddy / Web / IDE
       |
ExperienceTransportAdapter
       |
     AG-UI
       |
Control Plane / Runtime projection
```

AG-UI events are UX/session transport facts, not Work/Run/Evidence authority.

### Agent2Agent (A2A)

Reference:
- https://github.com/a2aproject/A2A

A2A standardizes communication between independent, potentially opaque agents:
capability discovery, interaction negotiation, shared tasks and exchange of
structured data.

Use A2A for **external peer-agent interoperability** if a real cross-system need
appears. It should not replace internal TaskContract, Run, coordination leases or
Evidence semantics.

### MCP Registry

Reference:
- https://github.com/modelcontextprotocol/registry

The official MCP Registry is a metadata/discovery service for public MCP
servers, including namespace ownership/authentication mechanisms.

Platform rule:

> registry discovery != tool authorization.

A discovered server may help construct a candidate ToolProfile; a Formal Run must
still bind an exact approved ToolProfile/capability/policy.

### agentgateway / Archestra

References:
- https://github.com/agentgateway/agentgateway
- https://github.com/archestra-ai/archestra

These are useful reference implementations for consolidating LLM/MCP/A2A
traffic, identity, OAuth, cost policy, tool policy, registry and observability.

Do not insert a generic gateway between Core and every tool by default. Activate
one only when provider/tool estate scale justifies the operational dependency.

## 6. Round 55 — Agent governance and deterministic control

### Microsoft Agent Governance Toolkit

Reference:
- https://github.com/microsoft/agent-governance-toolkit
- https://microsoft.github.io/agent-governance-toolkit/

The toolkit provides policy enforcement, agent identity/trust, isolation,
kill-switch/runtime controls, audit and SRE concepts. Its host-side policy model
intercepts agent actions before the external effect and supports deterministic
allow/deny/approval decisions.

Important comparisons with engineering-platform:

| Toolkit concern | engineering-platform mapping |
| --- | --- |
| policy decision | OPA / Action Gateway policy |
| agent identity/trust | authenticated principal / Worker / RuntimeProfile |
| approval | explicit controlled-action approval |
| audit evidence | audit + ActionReceipt / ExternalOperation |
| runtime termination | Session/Run control / Human Takeover |
| policy snapshot | exact policy/profile identity bound to Formal execution |

The toolkit is a **reference and conformance/threat-model source**, not a reason to
replace OPA or duplicate the existing Action Gateway.

Research items worth borrowing:

- fail-closed policy evaluation;
- immutable/pinned session policy snapshots;
- explicit conflict-resolution semantics;
- machine-readable policy decision trace;
- tamper-evident audit patterns;
- conformance tests across language bindings.

## 7. Round 56 — Current operator/parallel-agent UX landscape

### Emdash

Reference:
- https://github.com/generalaction/emdash

Local-first desktop supervision for parallel coding agents with a worktree per
task, local/SSH execution, diff/PR/CI inspection and provider lifecycle hooks.

### Paseo

Reference:
- https://github.com/getpaseo/paseo

Paseo exposes the same local coding agents across desktop, mobile, web and CLI
through a self-hosted daemon. It supports native provider adapters plus a generic
ACP provider tier.

Most useful WorkBuddy lessons:

- one durable remote operator surface over the user's actual development host;
- attach/follow-up/interrupt from another device;
- provider-neutral session presentation;
- local-first code/data ownership;
- explicit trust warning for plugins that execute on the daemon host.

### dmux / 1code / related multiplexers

References:
- https://github.com/standardagents/dmux
- https://github.com/21st-dev/1code

Treat these as UX/complexity references for parallel worktrees and coding-agent
multiplexing, not platform authority.

### Vibe Kanban

Reference:
- https://github.com/BloopAI/vibe-kanban

Useful historical UX reference for issue -> workspace -> agent -> diff -> PR
flows, but its current repository direction indicates sunsetting. Do not select
it as a new dependency.

## 8. New architecture conclusions

The continued search does **not** justify replacing the Core. It does sharpen
seven implementation rules:

1. **Reasoning and mutation should stay structurally separated.** GitHub
   Agentic Workflows' safe-output pattern independently validates the existing
   Action Gateway direction.
2. **Sandbox lifecycle needs a provider seam only when a second real backend is
   being integrated.** Do not create abstraction ahead of the need.
3. **Software-factory role labels are insufficient for assurance.** Reviewer
   independence must be enforced through identity/input/action boundaries and
   exact evidence.
4. **Agent qualification should become corpus-based.** Public benchmarks are
   useful, but retained project failure replay and embedded-specific tasks must
   dominate shipping confidence.
5. **Protocols are transport, not authority.** ACP, AG-UI, A2A and MCP each have
   useful interoperability scopes; none should redefine Work/Run/Evidence.
6. **Registry discovery never grants capability.** MCP/A2A/tool catalogs are
   projections; Formal Runs bind exact approved profiles.
7. **Generic agent governance overlaps existing controls.** Borrow conformance,
   threat models and decision traces rather than introducing a second policy
   authority.

## 9. Post-M1 bounded experiments

After the real retained Feature and Debug Closure chains complete, the best
bounded experiments are now:

1. **ACP AgentRuntimeAdapter spike** — Codex plus one second ACP-speaking agent.
2. **OpenShell vs OpenSandbox SandboxProvider spike** — compare exact isolation,
   credential, egress, filesystem, startup and recovery semantics.
3. **Harbor qualification spike** — bind one exact runtime profile to a small
   public benchmark plus an embedded-project failure replay corpus.
4. **AG-UI WorkBuddy projection spike** — map Run/Steering/Approval projections
   without exposing Core mutation authority.
5. **Safe-output threat-model comparison** — compare GitHub Agentic Workflows
   safe outputs against the existing Action Gateway publisher/reconciliation
   implementation.

These experiments are explicitly **after** M1 retained-pilot proof unless a real
pilot blocker shows that one is needed earlier.
