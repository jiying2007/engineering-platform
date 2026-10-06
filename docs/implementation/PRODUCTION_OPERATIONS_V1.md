# Production Operations v1

Date: 2026-10-04

Status: **STATIC BASELINE IMPLEMENTED — LIVE OPERATING QUALIFICATION PENDING**

This document is the canonical operating contract for issue #105. It does not
create a new authority model. Production uses the existing Task / Run /
execution epoch / Action Gateway / Evidence / Verification / Review / Recovery
chain.

## 1. Selected deployment profile

Production-v1 targets a small-team Ubuntu deployment using systemd and
PostgreSQL.

Kubernetes is intentionally not required for v1. The current product needs
deterministic identities, fail-closed restart behavior, retained evidence and
recoverability more than cluster orchestration.

Canonical layout:

- binaries: `/opt/engineering-platform/bin`;
- configuration and TLS material: `/etc/engineering-platform`;
- mutable preparation/artifact state: `/var/lib/engineering-platform`;
- PostgreSQL: durable external/local service with independent backup/restore;
- service manager: systemd.

## 2. Deployment identities

Production roles must not collapse into one Unix identity.

Required identities:

- Control Plane;
- Worker admission;
- Worker preparation;
- Publisher;
- Evidence importers;
- Verifier;
- Reviewer;
- Closure authority.

The systemd baseline materializes Control, admission, preparation and Publisher
as separate non-root service identities. The Publisher service is the only
production process allowed to read the GitHub publisher token.

Model/Worker processes must never receive:

- `DATABASE_URL`;
- GitHub publisher credential;
- reviewer/closure credentials;
- unrelated host login state.

Control Plane must not carry `GITHUB_PUBLISHER_CONFIG_FILE` in the production
profile. It carries only a non-secret publication plan and an mTLS remote-client
configuration. The Publisher service independently rechecks target policy and
retained bundle digest/size before GitHub mutation.

## 3. Configuration and secrets

Environment/configuration files are owner-private regular files and are never
committed with real secret values.

Production requires:

- mTLS Control API;
- `INSECURE_DEV` absent;
- `AUTO_MIGRATE=0`;
- explicit PostgreSQL URL;
- explicit server certificate/key/client CA/access policy.

Database migration is a deployment operation, not a side effect of service
restart.

Provider credentials are separate from this baseline. The selected unattended
provider must be independently qualified before production readiness can be
claimed.

## 4. Startup and shutdown

Expected startup order:

1. PostgreSQL ready;
2. explicit migration/check step;
3. independent Publisher;
4. Control Plane;
5. Worker admission;
6. Worker preparation;
7. provider-specific execution workers only when an authorized Run exists.

systemd may restart long-running admission/preparation/control processes after a
crash. It must not replay a model turn or external publication merely because a
process restarted.

SIGTERM is the normal stop path. Control Plane already performs bounded HTTP
shutdown and cancels its relay loop. Worker loops use signal-bound contexts.

## 5. Health versus readiness

`GET /healthz` proves only that the HTTP process is serving. It is deliberately
not the detailed production readiness authority.

The authenticated mTLS endpoint
`GET /api/v1/operations/status` requires `core:read` and reads one
PostgreSQL-backed operational snapshot containing:

- Recovery epoch/mode;
- active Runs;
- pending Worker intents;
- active and expired Worker leases;
- pending/leased/dead-letter outbox counts;
- UNKNOWN/RECONCILING/MANUAL external Action counts;
- UNKNOWN Core-bound Codex execution count.

`eng production-status` reads this endpoint through the existing direct-mTLS
Control client. Snapshot v2 deliberately separates `authority_state=CLEAR` from
`service_readiness=NOT_OBSERVED`. Neither an empty database, a valid lease nor
historical progress demonstrates current worker/publisher capacity. `ready=false`
and `production_qualified=false` remain explicit. `--require-ready` therefore
fails closed without an implemented service-readiness observation. The separate
`--require-authority-clear` checks only the narrower database authority condition.

Authority blockers include Recovery/Reconciliation, expired Worker leases,
dead-letter Outbox records, UNKNOWN/RECONCILING/MANUAL external operations and
UNKNOWN Codex execution. No snapshot or diagnostic may resolve these conditions,
authorize a retry or replay a model/publication.

### Bounded queue-progress observation

A finite, read-only observation uses the same endpoint, client and `core:read`
permission. It adds no server route, heartbeat table, scheduler, permission or
second status authority:

```sh
eng production-status --observe-for 30s --interval 5s \
  --max-pending-age 2m --require-no-alert
```

The age is an explicitly selected diagnostic threshold, **not** a calibrated
production SLO. It must be supplied; the command invents no default target.
Windows are 2..300 seconds with 1..60-second intervals, 3..61 samples and exact
whole-second divisibility. Age thresholds are 1 second..24 hours. Reads are
sequential and bounded to one interval each. The total deadline is the requested
window plus one interval. Cancellation/SIGTERM, a failed read, missed slot,
stale/repeated/contradictory snapshot, changed Recovery epoch/mode or regressed
progress marker aborts without retries, catch-up bursts or a complete report.
Prior samples are validated before any next request. An aborted invocation
returns nonzero; it is not evidence that the service is stopped or unhealthy.

The report retains every bounded snapshot/request/receive time and the selected
policy. Queue facts include pending start/end/sample peak, final oldest age,
whether the oldest **timestamp** stayed equal at all samples, and whether the
last recorded admission/dispatch timestamp advanced. Equal oldest timestamps do
not prove that one identical task remained queued. Counts decreasing or an oldest
timestamp changing do not prove successful work completion; no throughput is
inferred. Sampling cannot detect every event between requests.

If an unchanged oldest timestamp is present at every sample and exceeds the
selected age at the final sample, the report alerts. It distinguishes
`AGED_BACKLOG_NO_PROGRESS_MARKER` from `AGED_BACKLOG_WITH_PROGRESS_MARKER`; other
work advancing must not hide persistent old backlog. A database authority hazard
at **any** sample keeps the window alerted even when the final sample is clear.
Empty or changed queues yield only sampled observations, never READY.

`--require-no-alert` prints the completed report and exits nonzero on a diagnostic
or sampled authority alert. Without it, exit zero means that the observation
completed, not that no alert exists. This window flag cannot be mixed with the
single-snapshot readiness/authority gates. All results keep service readiness
unobserved, capacity unobserved, ready/execution/production flags false. A local
report and adjacent checksum are not independently authenticated SLO evidence.

Actual compiled-CLI tests use the existing authenticated Core middleware and
real ephemeral mTLS identities: `core:read` succeeds, another valid identity
without that permission fails before reaching the Store. The Store's changing
snapshots in these tests are explicitly synthetic; they do not substitute for
PostgreSQL service, real consumer heartbeat or deployment acceptance.

Publisher reachability, component heartbeat/capacity and selected-provider
qualification remain separate unfinished observations. Measured operating SLOs
require their own frozen workload, provenance and acceptance.

## 6. Restart and UNKNOWN policy

Crash restart is permitted for idempotent polling infrastructure.

The following are never blindly replayed:

- a model turn whose completion is ambiguous;
- GitHub push/PR mutation;
- other Action Gateway controlled/high-risk mutations.

Ambiguous external effects remain UNKNOWN and must enter reconciliation or
manual recovery under the existing Action/Recovery authority.

## 7. Database and recovery

Production must retain PostgreSQL backup/restore evidence.

A restore does not immediately re-enable external mutation. The platform enters
Recovery/Reconciliation and only returns to NORMAL after independently retained
reconciliation facts satisfy the existing recovery proof contract.

Backup/restore operational targets are measured by the later SLO harness.

## 8. Rollout and rollback

Production changes use a canary-first profile.

Rollback means restoring the previous deployable binaries/configuration and, if
required, a database restore/reconciliation procedure. Rollback never rewrites
or deletes retained Evidence, Review, Closure or external-operation receipts.

Each immutable installation returns a source commit and installation-manifest
digest that must be retained outside the release directory. `eng
installation-readback` uses those externally retained identities to verify an
older installation byte-for-byte without comparing its templates to the current
binary's embedded templates. Recomputing a manifest digest from an arbitrary
tree during recovery is not qualification. This readback does not switch
versions, start/stop services, migrate a database or authorize execution.

Emergency stops must include:

- disable selected provider credential/rule/gateway;
- revoke publisher credential;
- stop Worker execution;
- retain Control Plane read/recovery access where safe.

## 9. Production preflight

`eng production-preflight --config FILE` verifies the host-side static
production baseline independently of provider authentication.

Preflight configuration version 2 requires `expected_source_commit`. The default
check validates the complete six-role distribution, actual distinct non-root
UIDs, configuration ownership/references, TLS pairs, listener/endpoint/DSN
syntax and publisher policy consistency. Referenced host files must be
single-link regular files and must be reachable using the target service
identity's Unix mode bits: every parent directory must be traversable and the
target must expose the required read or directory access. Owner-private secrets
remain owned by the consuming service (systemd EnvironmentFile itself may be
root-owned because PID1 reads it). ACL-only grants are intentionally not inferred
by this static check. Its highest static result is
`HOST_VALIDATED`, with `operationally_ready=false`.

`--config-only` is an explicit outer-contract inspection mode. It returns
`CONFIG_VALIDATED` and does not check or admit a deployed host. Missing accounts
and services must never be described as ready based on this mode.

After host validation the remaining gates are explicit:

- `unattended_provider_live_qualification`;
- `live_service_operational_qualification`.

The production systemd unit uses `control-plane --production`. That startup
path rejects local publisher configuration, migration-on-start and insecure
mode before opening the database. The old in-process publisher is restricted to
explicit `DEPLOYMENT_MODE=pilot` and the local pilot helper; it is not a second
supported production architecture.

## 10. Terminal acceptance

Production readiness is not proven by source merge.

The exact terminal contract is
`docs/implementation/PRODUCTION_TERMINAL_ACCEPTANCE_V1.md`.

`eng production-terminal-plan` freezes the one-time RELEASE maintenance
fixture, evidence procedures, human-review requirement and terminal gates before
provider access exists.

`eng production-slo-report` v2 never grants qualification. With only observations
it emits `UNVERIFIED_SUMMARY`. To read back exact collector records, supply
`--source-root DIR --run RUN_ID --subject SUBJECT_DIGEST --require-verified`.
A successful readback emits `SOURCE_BYTES_VERIFIED`, binding the procedure,
collector digest, time window and exact samples. Source files alone can still be
synthetic; provenance and operational acceptance belong to the existing
Evidence/Verification/Review chain. No latency target is guessed in code.

The manual `Production terminal pre-live dry run` workflow must retain one
protected-main artifact before the live terminal qualification.

Final acceptance still requires:

1. selected unattended provider qualification;
2. production preflight HOST_VALIDATED for the admitted source commit;
3. operational status READY;
4. the exact frozen RELEASE maintenance fixture;
5. Run → model execution → Git/PR → exact-head CI → Evidence → Verification →
   independent human Review → Closure;
6. clean service shutdown/restart/recovery evidence;
7. source-verified, provider-inclusive SLO evidence accepted by the existing
   Verification/independent Review chain against measured, explicitly frozen targets.

Trusted self-hosted M1 evidence remains separate and is not rerun merely to
produce production screenshots.


## Installed process lifecycle regression

The mandatory native command CI now starts the installed `control-plane
--production` and independent `publisher-service` entrypoints, not an in-process
Control replacement. It installs the source-matched six-role distribution and
removes its original copy first. Synthetic short-lived TLS identities and an
inert test-only Publisher token remain in a separate private directory.

An explicit test administrator creates/migrates only a new isolated PostgreSQL
schema. Production Control must reject an unprepared schema without creating
any tables and reject auto-migrate or the old in-process Publisher settings.
The running installed Control's real relay and installed Worker complete input
admission over mTLS. Clean SIGTERM and quiescent SIGKILL/restart preserve the
exact admitted receipt, audit chain, Outbox states/attempts and Recovery epoch,
without creating engineering Evidence or new model/external executions.
Port-collision failures do not consume work or stop the original listener.

Publisher tests check the exact authenticated control identity, reject malformed
publish/observe plans before any upstream call, reject all command-line
arguments, and check listener closure and rebind after graceful/forced stops.
Its listener announcement is a bound-address diagnostic, not authenticated
readiness: clients must verify TLS/health and the required operation separately.
When Publisher is stopped, Control's database status continues to report service
readiness NOT_OBSERVED rather than manufacturing a global-ready claim.

These tests require PostgreSQL in the mandatory native CI; a local unavailable
service is explicitly skipped, never replaced by memory. The optional
EP_TEST_INSTALLED_ROOT / EP_TEST_INSTALLED_SOURCE test-harness variables run the
same checks against an independently authenticated downloaded installation.
They are not production service options. The test records retain no private
keys, runtime credentials or customer data.

This is installed process and quiescent-state restart evidence, NOT systemd or
fresh-host dependency provisioning, in-flight publication/model crash recovery,
zero-downtime upgrade, migration rollback, component capacity, real-provider or
production qualification. No supervisor automatically replays model work. Those
remaining gates and #105 remain open.
