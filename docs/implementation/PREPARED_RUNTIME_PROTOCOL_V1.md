# Prepared Runtime protocol v1

Base: 57eb3b9f0e018fbdb3a8f9fae1345a19604eaa24 (#31).
Independent Runtime/Context correctness slice; the prior Worker-admission
candidate is not implicitly applied. No new service, deployment or model call.

## Implemented

- Absolute configured executable, existing canonical worktree, disjoint fresh
  owner-only HOME; no parent environment, arbitrary arguments or PATH executable
  lookup. Child environment is constructed from scratch. Optional provider inputs
  are explicit OPENAI_API_KEY and HTTPS OPENAI_BASE_URL, never logged. CODEX_HOME
  and XDG locations are derived under HOME. Fixed `app-server --listen stdio`.
- Closable JSONL endpoints, <=1 MiB frames, <=128 pending requests and <=64 queued
  events; overflow terminates rather than blocking or dropping approvals.
  Out-of-order correlation is separate from notifications; invalid routing fields,
  duplicate envelope keys, bad IDs and zero-progress writes fail closed. Partial
  writes are completed. A timeout after sending closes without blind retry.
  A valid final response is not lost to the following EOF.
- Typed single-thread adapter for initialize/initialized, thread/start, turn/start,
  turn/steer and turn/interrupt. Workspace is bound at construction; model is an
  explicit host input, not hardcoded. This first profile requests readOnly with
  unlessTrusted. Supported scoped server approvals are declined; unknown requests
  receive method-not-found. There is no approval-grant API. Interrupt ACK is not
  completion; only the matching turn/completed clears the active turn. Completion
  preceding the start response is handled without reviving a completed turn.
- Concrete contextbundle.LocalSource implements local content-addressed resolution
  and an immutable operator approval snapshot binding Run ID, Task digest, full
  input digest and exact refs. No caller paths/URLs, implicit APPROVED grant or
  automatic source discovery. The existing materializer still enforces limits,
  hashes actual bytes and re-verifies the published bundle.

## Verification

An offline subprocess helper is the Go test binary, not Codex. A cross-component
integration test creates an exact-base Git worktree, resolves approved local data,
materializes and verifies it, launches an isolated process, then exercises
initialize/thread/turn/denied approval/steer/interrupt/close. It checks unchanged
HEAD and clean state and removes the worktree. Negative tests cover inherited
secrets/config, unsafe arguments/paths, backpressure, cancellation, short writes,
malformed frames, pending limits, final-response EOF, early completion and
unapproved sources. CI retains PostgreSQL/full race/vet/build and repeats Runtime
and Context tests ten times. Local Go 1.23 scoped tests are not full-repository
proof; exact PR/main CI using the repository Go baseline is authoritative.

## Limits and next gates

This adapter is not Core Run, lease, recovery or Action authority. The host must
authorize operations, drain Next during calls and retain exact provider receipts.
Endpoint Close must interrupt Read/Write (OS pipes/net.Conn), not an arbitrary
non-cooperative reader. No detached read/write goroutine is used to fake timeout.

File modes and canonical path checks are not an OS sandbox or a defense against
hostile same-UID modification of host-controlled parents. Workspace Git config,
hooks/environment and shared metadata still require hardening. Separate identity,
read-only mounts, network/resource controls and process-tree supervision remain
necessary before untrusted execution. Read-only protocol configuration is a
request, not independently measured isolation. Explicit credentials remain
visible to the provider process; a broker is still needed for narrower grants.

Local approvals are restart-loaded operator snapshots, not signed online approval
or mTLS principal authority. A changed approval requires controlled re-creation.
The offline helper does not prove real Codex compatibility/model output or that
the real model consumed Context bytes. Pin/qualify a concrete binary/schema before
real use. No Worker candidate is merged by this change, and INPUT_VALIDATED is not
execution authorization. Real Worker execution, Action/recovery-completion wiring,
Git/CI/Artifact facts, independent Review and Feature/Debug pilots remain open.
M1 and production readiness are not claimed.

References consulted:
- https://developers.openai.com/codex/app-server
- https://pkg.go.dev/os#Root
- https://pkg.go.dev/os/exec
