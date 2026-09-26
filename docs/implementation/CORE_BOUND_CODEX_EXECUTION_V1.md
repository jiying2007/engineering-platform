# Core-bound Codex engineering execution v1

Base: `f5af0ee88bf283213424839b6a28db961a4478db` (#48).

This increment connects the already-qualified Codex 0.155 app-server to the
existing Core Run / Worker / preparation authority and produces a retained local
result commit and Git bundle. It does not push, open a PR, deploy, or auto-complete
a Run.

## Frozen execution profile

A Task must explicitly allow `worker.codex-execute`. The frozen RunInput
ToolProfile is `codex/<profile-digest>`, where the profile binds:

- exact Codex version 0.155.0;
- exact qualified native binary SHA-256;
- exact engineering config digest;
- exact model identifier;
- `sandbox=workspace-write`;
- `approvalPolicy=never`;
- tool network disabled.

The server accepts one Core-bound Codex reservation per Run. Offline and Codex
execution reservations are mutually exclusive for one Worker authority.

## WIF credential boundary

The Worker accepts only Workload Identity Federation for this lane. Long-lived
`OPENAI_API_KEY` and endpoint overrides are rejected.

The exact native Codex process starts with an isolated owner-private HOME. Before
any model-reachable thread or turn, the adapter executes
`account/rateLimits/read`, forcing the WIF exchange. The host then unlinks the
upstream identity assertion and confirms the path no longer exists.

Only after that fence does it start an ephemeral workspace-write thread.

The model shell policy is fixed:

- no inherited shell environment;
- no WIF variables in tool environment;
- no web search;
- no tool network access;
- no login shell;
- no approval requests.

Any approval request fails the execution. External tool item types such as web,
MCP or dynamic tools are rejected by the observer.

The exchanged access token is owned by the Codex process session; the upstream
assertion file is not available when model tools become reachable.

## Prompt identity

The prompt is deterministic from frozen platform facts:

- Run ID;
- TaskContract digest/type;
- RunInput digest;
- repository/base commit;
- target/capabilities/skills;
- acceptance criteria and expected outputs;
- approved Context bundle digest.

The prompt instructs Codex to modify only the current workspace and forbids
commit, push, fetch, package installation and network access. Codex does not
receive repository publishing authority.

## Execution and retention

The Worker holds a renewable, epoch-bound execution reservation. Loss of lease,
pause, takeover or recovery transition prevents a successful result receipt.

The real app-server turn retains:

- qualified binary/config/model identity;
- prompt digest;
- thread/turn IDs;
- terminal status and bounded final output;
- command count / failed command count / file-change count;
- zero approval requests;
- proof that the assertion was removed before the turn.

After the turn, the Worker revalidates frozen preparation/context identities and
uses the platform-owned Git manager to finalize the changed workspace:

- exact base commit/tree/source digest must still match preparation;
- Git-visible changes are required;
- submodules are rejected;
- hooks/GPG signing are bypassed;
- a fixed local author is used;
- the result workspace must be clean after commit;
- a Git bundle containing only the result relative to the base is created and
  verified;
- result commit/tree/source digest plus bundle SHA-256/size are retained.

No git push or GitHub token exists in this lane.

## Ambiguity and recovery

A Core-bound Codex execution is one-shot. Start/result ambiguity is never
automatically replayed. Failed local execution records the reservation UNKNOWN.

Migration v8 adds the durable Codex execution ledger. UNKNOWN/AUTHORIZED Codex
executions participate in recovery reconciliation facts and block recovery
completion until independently reconciled.

## Evidence

A dedicated evidence principal is reserved:

- subject: `urn:engineering-platform:codex-evidence-importer`
- issuer: `codex-execution-importer`
- procedure: `codex.core.execution.v1`

It may hold only `core:read + evidence:register`.

Delivery must bind two exact artifacts:

1. canonical `WORKER_ATTESTED_CODEX_EXECUTION` receipt digest;
2. result Git bundle SHA-256 and size.

The importer re-reads the durable FINISHED Codex status, validates model-turn and
changed-tree facts, requires Delivery base/result commits to match the receipt,
and rehashes the local bundle bytes before registering PASS Evidence.

This evidence proves that the exact Core-bound Codex turn produced the retained
result commit/bundle. It does not prove CI success or engineering correctness;
those remain separate frozen requirements.

## External prerequisite

A real run still requires the managed ChatGPT workspace administrator to enable
and configure Codex WIF and provide the approved federation rule / short-lived
identity assertion. Repository CI uses a fake app-server for protocol and
filesystem tests and therefore is not live model evidence.

## Remaining closure work

After this slice, the remaining path to an M1 assessment is primarily retained
real-world evidence:

1. configure the external WIF rule and run one real Core-bound Codex Feature task;
2. publish the retained result through an independently authorized Git/PR adapter;
3. collect trusted CI + Codex + Git evidence;
4. pass independent Verification and Review, then Closure;
5. repeat the lifecycle for one Debug task.

No M1 or production-readiness claim is made by this implementation alone.
