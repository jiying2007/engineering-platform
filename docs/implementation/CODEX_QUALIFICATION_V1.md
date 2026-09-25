# Codex real app-server qualification v1

Reviewed base: `df5d69d2d2f822d90571c44526b313eb9a872090` (#35).
This gate qualifies a concrete upstream Codex CLI/protocol before the platform is
allowed to claim Codex compatibility. It does not make a model turn, provision a
credential, enable external network actions, or complete an engineering Run.

## Qualified upstream

- CLI: `codex-cli 0.155.0`
- upstream release: `rust-v0.155.0`
- upstream release commit: `f0a1b8f`
- transport: a NEW `codex app-server --stdio` process for each qualified session
- stable client capability: `experimentalApi=false`
- thread profile: `ephemeral=true`, `sandbox=read-only`,
  `approvalPolicy=never`
- per-thread `config` overrides: forbidden
- managed daemon/proxy: forbidden in this qualification path

The version is intentionally not "latest". A future upgrade changes this contract
and must produce a new exact-head qualification receipt and pass all downstream
Runtime/Worker tests before becoming the repository default.

## Why a fresh stdio process

Upstream 0.155.0 added more app-server daemon lifecycle/update behavior. A reported
0.155.0 updater failure also demonstrated that a background daemon can remain on
an older binary than the CLI. The platform therefore does not use the daemon as
its execution identity. The executable selected by the host is canonicalized,
hashed before launch, and launched directly; a changed byte digest fails closed.

A current upstream report also shows that 0.154.0 per-thread `config` overrides
can wedge a subsequent turn. This platform does not send that override. Provider
configuration belongs to the host launch profile and fresh isolated HOME.

## Protocol drift fixed by this change

The former helper accepted values that no longer represent the current stable
wire contract:

- `sandbox: "readOnly"`
- `approvalPolicy: "unlessTrusted"`
- `app-server --listen stdio`

The qualified path now uses the current explicit stable forms:

- `sandbox: "read-only"`
- `approvalPolicy: "never"`
- `app-server --stdio`

The generated stable `ThreadStartParams` schema must contain `sandbox` and
`approvalPolicy`, and the generated schema set must contain the stable
`read-only/workspace-write/danger-full-access` sandbox vocabulary plus
`never/on-request` approval vocabulary. The stable `ThreadStartParams` must NOT
expose the experimental `permissions` field; the same version's
`--experimental` schema MUST expose it. This detects an accidental surface change
rather than silently opting the product into an unstable API.

## CI qualification sequence

The mandatory `codex-app-server-0.155.0-qualification` job:

1. installs exactly `@openai/codex@0.155.0` in an isolated runner directory;
2. locates the single native executable from the platform package rather than
   qualifying the Node wrapper;
3. requires exact `codex-cli 0.155.0` output and retains npm package integrity;
4. hashes the native executable bytes;
5. generates stable and experimental JSON-schema trees with fresh HOME and pre-created owner-only CODEX/XDG directories;
6. validates the stable/experimental split and records deterministic tree digests;
7. starts the exact pinned native binary with a fresh empty HOME and no provider
   credential, daemon, inherited config or proxy;
8. completes real `initialize` + `initialized` + ephemeral `thread/start`;
9. uploads a deterministic JSON qualification receipt and npm integrity value as
   a CI artifact.

No `turn/start` occurs in this gate, so no model inference or API billing is
represented by a green qualification job. The thread-start model string is a
protocol input only.

## Runtime boundary

`NewPinnedProvider` binds launch to an exact raw-byte SHA-256. It retains the
existing rules: absolute host-selected executable/workspace, owner-only fresh HOME,
no parent environment, no arbitrary CLI arguments, explicit credential/HTTPS
endpoint only, and no inherited Git/proxy/loader settings. Pinning does not defend
against a malicious host/kernel or a same-privilege replacement race outside the
host trust model.

The adapter remains deliberately non-interactive for this qualification profile:
there is no API that accepts approval requests. Stable command/file approval
requests are declined; unknown server requests receive method-not-found. A later
interactive Codex lane must bind each approval to current Core/Action authority
rather than changing this qualification adapter into a permissive client.

## Acceptance and remaining gates

This change is accepted only when both the ordinary repository CI and this real
Codex qualification job pass on the exact PR head, followed by fresh-main checks
after merge. Offline helper tests are regression coverage, not substitutes.

Still NOT established by this gate:

- a real authenticated model turn;
- provider credential brokering or egress policy;
- workspace-write Codex execution;
- interactive Action Gateway approvals;
- Git/CI/Artifact publication authority;
- independent Review, reconciliation/restore drills or retained Feature/Debug
  pilots.

The next execution increment should consume this qualified binary identity inside
the already-built preparation/execution authority chain rather than creating a
second Runtime authority.
