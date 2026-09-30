# Production Operations v1

Date: 2026-09-30

Status: **PRODUCTIONIZATION BASELINE — PROVIDER QUALIFICATION PENDING**

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
3. Control Plane;
4. Worker admission;
5. Worker preparation;
6. independent Publisher;
7. provider-specific execution workers only when an authorized Run exists.

systemd may restart long-running admission/preparation/control processes after a
crash. It must not replay a model turn or external publication merely because a
process restarted.

SIGTERM is the normal stop path. Control Plane already performs bounded HTTP
shutdown and cancels its relay loop. Worker loops use signal-bound contexts.

## 5. Health versus readiness

`GET /healthz` proves the HTTP process is serving. It is not sufficient for
production readiness.

Production readiness must eventually combine:

- Control TLS endpoint reachable;
- PostgreSQL reachable and schema valid;
- Recovery state NORMAL;
- Worker mTLS identities usable;
- publisher service reachable with no credential exposed to Control/Worker;
- selected provider qualified and not administratively disabled.

The observability slice will turn these facts into a retained machine-readable
status.

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

Emergency stops must include:

- disable selected provider credential/rule/gateway;
- revoke publisher credential;
- stop Worker execution;
- retain Control Plane read/recovery access where safe.

## 9. Production preflight

`eng production-preflight --config FILE` verifies the host-side static
production baseline independently of provider authentication.

At this checkpoint it deliberately reports two blockers:

- `publisher_service_separation`;
- `unattended_provider_live_qualification`.

This is preferable to silently marking production ready.

The next slices must clear these blockers in that order.

## 10. Terminal acceptance

Production readiness is not proven by source merge.

The terminal acceptance requires:

1. selected unattended provider qualification;
2. production preflight READY;
3. one bounded unattended maintenance fixture;
4. exact Run → model execution → Git/PR → exact-head CI → Evidence →
   Verification → independent human Review → Closure;
5. clean service shutdown/restart/recovery evidence;
6. retained operational/SLO report.

Trusted self-hosted M1 evidence remains separate and is not rerun merely to
produce production screenshots.
