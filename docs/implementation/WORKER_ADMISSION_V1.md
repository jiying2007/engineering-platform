# Durable Worker admission v1

Reviewed integration base: `4c1a226b696221e5a3fa82b6a7e425fc301b21bb` (Runtime/Context #32).
This is the reviewed/rebased implementation of the earlier unmerged Worker
candidate, not a blind application of its #31-base patch. No new domain or service.

## Executable path and authority

The authenticated Control Plane now drives a local PostgreSQL transfer:

```
Work/Task -> frozen Run -> run.started outbox
 -> atomic inbox + outbox ACK + audit
 -> mTLS Worker claim by explicit WorkerProfile
 -> verify frozen Task/RunInput/Intent identities
 -> renew exact lease -> deterministic report
 -> retained INPUT_VALIDATED receipt -> eng api readback
```

The relay handles only `run.started` with CONTROLLED_MUTATION classification;
other topics are left untouched. Every transfer locks current recovery state,
validates persisted Run identity and ownership, inserts or compares the stable
outbox-key/Run identity, settles the outbox and appends audit in ONE transaction.
A failed audit cannot leave an acknowledged outbox without its inbox. This is
local database atomicity, not distributed exactly-once or a second external action
provider. No long-running callback or remote side effect is held under the relay
transaction. Invalid/revoked intents are quarantined rather than silently accepted.

Worker identity is the verified certificate URI, never a body field or hostname.
Policy adds `worker:poll`, `worker:report` and explicit `worker_profiles`; a profile
without a worker grant, a worker grant without profiles, or a requested profile
outside those grants is rejected. The worker has no database access. Heartbeats
record input-protocol ONLINE status with an empty runtime-provider list, not READY
hardware/provider capability claims. Grants retain the platform's single-trust-
domain scope; this is not tenant/Work-row isolation or automated enrollment.

## Lease and recovery behavior

A claim binds inbox ID, owner URI, monotonic generation, profile and recovery epoch.
It reloads exact Task/RunInput/Intent digests. Each identity can hold only one live
assignment. The lease is 30 seconds; explicit renewals extend at most 30 seconds
and never beyond two minutes from the current generation's initial claim. Same-
identity takeover of an expired lease increments the generation. Old/foreign/
expired tokens cannot renew or report. DB wall time is authoritative.

Claim, renewal and first report check the current Run and Session: exact execution
epoch, Runtime ownership, valid active state and no pause. A pause that commits
between candidate selection and row locking DEFERs the claim without revoking the
intent; resume can later claim it. Terminal/taken-over or identity-mismatched
intents are revoked. Current recovery must be NORMAL; completing recovery cannot
revive an old recovery-epoch lease. The protected recovery-completion API still
needs its independent verifier; this change does not weaken that gate.

Audit writes precede the final expiry check for renew/report. Waiting for the
journal lock must not extend an expired lease or commit an expired receipt. Every
first receipt is recomputed by the server from persisted inputs, not accepted as
a caller-declared success. Duplicate reports with the same exact token, owner and
validation digest return the original receipt without another audit or state
change. Historical readback does not grant a new execution/recovery authority.
The client only retries that identical report once after a transport/5xx ambiguity;
it never automatically retries a claim or an explicit 4xx rejection.

## Input validation is NOT code execution

The only outcome implemented here is `INPUT_VALIDATED`:

```json
{"context_bytes_verified": false, "execution_started": false}
```

No context bytes are fetched by this lane, no Skill is installed, no Git/Codex/
process/tool is launched by the Worker, and no Run is completed. No Evidence,
Verification, Delivery or Closure is manufactured from an input receipt. The
prepared Runtime/Context components from #32 remain separate execution
prerequisites, not an implicit fallback when input validation succeeds. An
UNTRUSTED context reference can be structurally recorded without promotion to
approved content or authority.

## Startup and clients

Migration `0003_worker_inbox.sql` is additive, version-tracked and repeatable;
prior migrations are unchanged. It preserves retained receipts and lease
generations on replay. Quiesce old consumers before upgrading; review/apply
migration using the operator-controlled path (`AUTO_MIGRATE=1` remains explicit).
Normal startup requires schema version 3, the existing PostgreSQL settings and
complete mTLS/policy configuration. Source merge does not deploy a service or
modify a production database.

Control Plane binds the listener before it starts the local relay. The same
serving lifecycle joins HTTP and relay before the database closes. Relay storage
failure stops the serving process instead of leaving a healthy-looking server
with a dead delivery loop. Cancellation is cooperative, bounded by the DB call
contexts; no live callback is abandoned. INSECURE_DEV remains memory-only with
no worker routes or relay.

Both `worker` and `eng api` use a shared client requiring:

```
CONTROL_ENDPOINT=https://<control-host>:<port>
CONTROL_CLIENT_CERT_FILE=<operator-issued client certificate>
CONTROL_CLIENT_KEY_FILE=<owner-only private key file>
CONTROL_SERVER_CA_FILE=<explicit server CA file>
```

No private keys are supplied in the repository. The client rebuilds an allowlisted
TLS 1.3 configuration, binds certificate validation to the actual endpoint hostname,
clones roots/certificate bytes, and excludes caller Time/key-log/name overrides.
It rejects insecure TLS, anonymous/ambient CA use, redirects, inherited environment
proxies, unsafe API paths and oversized requests/responses. It does not claim a
malicious same-UID process or caller-owned private-key signer is isolated.

The explicit executable interface is:

```
worker --admission-only --profile worker/ubuntu --once
worker --admission-only --profile worker/ubuntu
eng api GET /api/v1/runs/<run-id>/inbox
eng api POST /api/v1/runs run-request.json
```

The continuous input Worker defers HTTP 409 (pause/recovery/lease change) to a
later poll, exits on other persistent failures, and never advertises a Codex
provider merely because a binary can start. An operator controls restart policy.
A minimal dedicated worker policy is in `examples/worker-admission-policy.json`;
merge that principal into an intentionally reviewed policy for real operators.

## Regression acceptance

Existing Go 1.25/pgx/PostgreSQL 17 race tests, outbox repetitions, ten #32
Runtime/Context repetitions, vet and binary builds remain enabled. Added checks:

- 20 repeated input-agent/client/loop/unit tests, including real TLS and TLS snapshot
  checks; client redirects, errors, bounds and cancellation remain fail-closed.
- Three shuffled Worker database-suite repetitions: same-identity concurrent claims,
  concurrent relay, duplicate transfer, receipt replay, expiry/reclaim generations,
  recovery/takeover, migration replay and audit rollback.
- Adversarial DB row-lock tests prove pause-after-selection does not revoke work,
  and audit-lock wait cannot revive expired renewal/report authority.
- Three command-suite repetitions compile and start actual `worker` and `eng`
  binaries against the shared real Control Plane serving/relay lifecycle and an
  isolated PostgreSQL schema. Ephemeral mTLS identities submit a frozen Run, the
  live relay transfers it, Worker reports and eng reads the exact receipt. The
  test asserts Run remains RUNNING and no Evidence is created. Separate coverage
  verifies relay failure tears down serving instead of detaching a loop.
- AST checks keep new routes explicitly mapped and absent from the bare anonymous
  development handler.

Exact PR-head CI and fresh main CI are required evidence, not this document or
old candidate/local tests. Test-only schemas and temporary fixture certificates
are independently cleaned; no shared public schema is reset.

## Remaining execution gates

Bind prepared approved Context + exact workspace to a durable execution receipt,
with an OS isolation boundary and current Run/lease authority. Qualify a pinned
real Codex binary/schema, add authorized interactive approvals, gather actual
Git/CI/Artifact-byte facts and independent Review, and retain Feature/Debug plus
restore/recovery pilots. INPUT_VALIDATED must not substitute for any of these.
**M1, real model execution and production readiness are not claimed.**
