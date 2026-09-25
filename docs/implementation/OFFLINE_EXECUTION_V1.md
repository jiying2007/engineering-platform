# One-shot offline execution v1

Reviewed base: `2ff3de8d0520a84573a59d72cd460bf0492534af` (preparation #34).
This joins existing preparation, authenticated Worker and Core state to a real
bounded local-container execution. It is not a real Codex/model pilot or an
implementation of online provider approvals, arbitrary network actions or release.
No additional service, domain, legacy compatibility layer or identity authority.

## End-to-end path

```
Work/Task/Run -> existing relay/inbox -> worker --prepare-only
 -> retained prepared bytes and WORKER_ATTESTED_PREPARATION
 -> worker --execute-offline --run <id> --profile <profile> --once
 -> new Core execution reservation under current Run/recovery authority
 -> current local approval snapshot + independent source/context byte recheck
 -> inspected restricted container + independently timed PID 1 guard
 -> captured stdout/stderr + exact hashes + exit code + confirmed cleanup
 -> recheck unchanged prepared bytes -> current lease -> transactional receipt
 -> eng api GET /api/v1/runs/<id>/offline
```

A finished command does NOT complete the engineering Run or create Evidence,
Verification, Delivery or Closure. FINISHED records an observed exit, not PASS.
A nonzero exit remains a nonzero exit. Tests explicitly ensure no fabricated
engineering Evidence and no automatic Run completion.

## Explicit grants and frozen identity

The same preparation Worker identity must own the preparation receipt. It needs
worker:poll, worker:report, worker:prepare and its exact worker_profiles, plus an
existing action:execute grant tuple:

- action: worker.offline-execute
- risk_class: CONTROLLED_MUTATION
- capability: the exact SHA-256 Profile digest

Task.AllowedActions must include worker.offline-execute and frozen
RunInput.ToolProfile must equal `offline/<Profile digest>`. Changed image,
guard, argv or timeout requires a new explicitly frozen input and grant; the
worker cannot lower the risk, change a command or reuse another host's preparation.
The existing generic Action Gateway's privileged external providers are still
unconfigured. This is a narrowly bounded local offline execution reservation,
not a route around external-effect reconciliation.

Profile fields, in digest serialization order, are image_id, guard_digest, argv,
timeout_seconds. The digest is `sandbox.Profile.Digest()`: compact Go JSON of the
typed validated Profile, hashed as raw bytes with the sha256: prefix. Image is
an exact locally installed Docker image ID (`sha256:<64 lowercase hex>`), NOT a
mutable tag or a registry manifest digest. Engine never pulls an image. The
operator builds/inspects its tool image in a separate provisioning step.

## Execution containment and assumptions

Only Linux and an explicit canonical local Unix Engine socket are accepted.
Worker must be non-root. The daemon must report memory, CPU quota, PIDs controls
and default seccomp support; absent prerequisites fail closed. Engine API 1.45 is
used explicitly rather than inheriting Docker CLI configuration or environment.
The current integration uses the runner's local rootful daemon. Rootless or remote
Engine operation is not qualified by these tests.

The container runs as the Worker's numeric nonzero UID/GID, with no inherited
host/Worker environment and no Worker credentials or Engine socket mounted.
Its only host mounts are exact prepared source, Context bundle and guard binary:
all read-only and nonrecursive/private. No writable host mount is allowed. It has
network=none, private PID/IPC namespaces, read-only rootfs, all capabilities
dropped, no-new-privileges and the Engine's default seccomp. It is restricted to
256 MiB memory, no additional swap, one CPU, 64 PIDs, 8 MiB shm and a 64 MiB
noexec/nosuid/nodev /tmp. Image-declared volumes and unexpected image environment
are rejected. Entrypoint is always the pinned guard; image defaults cannot replace
it. Created-container inspection must match the policy before start; warnings or
missing constraints abort. Tests verify the actual process restrictions as well.

The static `sandbox-guard` runs as container PID 1 and enforces a separate 1..45
second deadline plus 128 KiB per stdout/stderr stream. Output overflow cancels the
command and cannot become an apparently complete rotated-log receipt. Guard is
built from this repository and its raw bytes are pinned in Profile. It refuses
host/non-PID1/root execution. Its independent timer bounds compute after Worker
crash; the Engine restart policy is no. Exiting PID 1 tears down remaining
namespace processes. On normal/error/cancel returns, the Worker force-removes its
exact container and refuses a success if removal cannot be confirmed.

The daemon, kernel, Worker host, operator roots, image, guard and source metadata
are TRUSTED. Engine access is itself privileged host control and must be restricted
to the Worker service, never given to the child. This is container isolation, not
a VM/kernel-escape guarantee, multi-tenant service or isolation against a malicious
same-UID host actor. Images must not embed secrets; snapshot reads must already be
authorized. Storage quotas and daemon hardening remain operator responsibilities.

## State, cancellation and uncertainty

Version-5 migration adds worker_offline_executions, without rewriting preparation
or input history. Start requires the v5 schema before any reservation. One unique
reservation per Run is permanent; there is no replay/reclaim endpoint. A lost Start
response MUST NOT be retried as a new execution. Read status with an authorized
operator instead. Expired AUTHORIZED history is shown as EXPIRED_UNRECONCILED;
a failed operation can record UNKNOWN, never reset into a launchable state.

Fresh Start checks preparation identity, exact frozen profile, current Run/Session
ownership/epoch/pause and NORMAL recovery. Preparation itself is not a live grant.
Start serializes the Worker identity and the Run inbox. Renew and first finish
recheck current authority; the same completed report is a historical read even
during later recovery. Changed report bytes/identity are rejected. Audit and first
receipt settle in one transaction; the final database wall-time expiry check is
after journal waits. Migration replay preserves history.

Execution leases last 20 seconds and renew every 3 seconds (each network attempt
has a 6-second bound). Maximum reservation age is 120 seconds; Worker operation
has a 90-second bound. Failed renewal cancels the local operation and tears down
the container. This is bounded/cooperative revocation, NOT a linearizable promise
that no instruction executes after a concurrent pause or recovery transition.
Only restricted offline effects are permitted. External irreversible actions
must not use this lease model. Hard host failure can leave a stopped container;
the guard bounds compute but does not implement operator orphan reconciliation.

The Worker exclusively retains local permit/report JSON in its prepared slot
before network reporting. Only an identical finish report can be retried once on
transport/5xx ambiguity; no local command is re-executed. Cleanup uncertainty,
lease loss, missing approval or changed bytes prevent successful settlement.
Operator reconciliation, automatic crash resume/retention and online recovery
completion remain unimplemented; an UNKNOWN record cannot authorize a new launch.

## Operator setup

Provision a trusted Linux Engine and an existing tool image; keep the socket off
all Runtime mount lists. Apply additive migration 0005 through the existing
reviewed operator path; merging code does not migrate/deploy a production service.
Build the guard statically for the host/image architecture:

```
CGO_ENABLED=0 go build -o /operator/path/sandbox-guard ./cmd/sandbox-guard
```

Prepare the Run using #34's exact operator approval, mTLS variables and dedicated
profile. `WORKER_PREPARATION_CONFIG` remains required at execution to recheck current
approval and locate the owned slot, not to create a fresh workspace. In addition,
set WORKER_OFFLINE_CONFIG to a protected operator JSON file with fields:

```
version: 1
engine_socket: canonical absolute Unix socket (resolve /var/run aliases first)
guard_executable: canonical absolute static guard path
profile:
  image_id: exact local sha256 image ID
  guard_digest: raw sha256 guard bytes
  argv: absolute in-image executable and explicit argument array
  timeout_seconds: integer in 1..45
```

No keys are included in the repository. Compute and review the typed Profile digest
before creating the immutable Task/Run and its certificate policy grants. Do not
patch a frozen Run merely to make an execution profile match. Execution is explicit:

```
worker --execute-offline --profile worker/offline --run <prepared-run> --once
eng api GET /api/v1/runs/<prepared-run>/offline
```

Non-execution modes reject unused offline configuration. execute-offline cannot
be put in the generic retry/poll loop. This lane needs an actual previously
prepared Run on the same Worker and is not an alternate preparation mode.

## Evidence and validation

WORKER_ATTESTED_OFFLINE_EXECUTION retains profile/image/guard identity, preparation
fact digest, container ID, UID, stdout/stderr bytes (JSON base64) and raw hashes,
exit code, authenticated Worker and server receipt timestamp. The server verifies
supplied-byte hashes and binding; the trusted remote Worker attests containment
and process observation. It does not mean the server observed the remote Engine.
Returned output is untrusted data; it grants no policy authority. The retained
stdout/stderr have the same platform-read access boundary as existing receipts;
operators must not authorize commands that print secret material.

Existing full PostgreSQL/race/Workspace/Context/Worker suites stay enabled. The new
mandatory offline-container-integration job sets EP_SANDBOX_INTEGRATION=1: missing
Docker/resource support fails rather than skipping. It builds unique scratch
images locally with a static probe and no network pull, runs the real guard,
checks read-only paths/absent secrets/network/capabilities/seccomp, output bound,
independent timeout and cancellation cleanup. Command integration runs real
compiled Worker/eng against live mTLS Control Plane/relay and isolated PostgreSQL,
prepares real Git/Context bytes, executes the probe and verifies exact persistent
receipt readback plus duplicate-launch denial. The probe is test code, NOT Codex.
Three shuffled DB repetitions cover one-owner reservation, exact-profile drift,
stale/foreign/paused/recovery/expired authority, replay, audit failure and expiry
while waiting on audit. PR and fresh-main results are the actual acceptance proof.

Remaining M1 gates: qualified real Codex/schema with a separately designed network
boundary and interactive approval path, output/artifact publication and real
Git/CI facts, independent Review, retained reconciliation/restore and Feature/Debug
pilots. No real model execution or production readiness is claimed here.

Primary references used: Docker Engine run/security/resource-constraints docs,
https://docs.docker.com/engine/containers/run/ ,
https://docs.docker.com/engine/security/ ,
https://docs.docker.com/engine/containers/resource_constraints/ .
