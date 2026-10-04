# Production Ubuntu/systemd baseline

This directory is the production-v1 deployment baseline for issue #105.

It deliberately does **not** qualify a model provider and does not contain
credentials. Provider qualification remains a later live gate.

## Identities

Create distinct non-login service identities:

- `engineering-control`
- `engineering-admission`
- `engineering-preparation`
- `engineering-publisher` for the credentialed GitHub mutation service

Do not run production services as `root`, and do not reuse one Unix identity
for multiple authority roles.

## Layout

- binaries: `/opt/engineering-platform/bin`
- service configuration: `/etc/engineering-platform`
- mutable preparation data: `/var/lib/engineering-platform/preparation`
- backups: `/var/lib/engineering-platform/backups`

The Control Plane unit invokes `--production`; do not remove this startup fence.
Environment files must be owner-private mode 0600. Secret values are provisioned
out of band and must not be committed.

Production Control Plane uses `AUTO_MIGRATE=0`. Database migration is an
explicit deployment action and is never coupled to service restart.

## Preflight

After installing binaries/configuration:

```sh
/opt/engineering-platform/bin/eng production-preflight \
  --config /etc/engineering-platform/production-preflight.json
```

Render the preflight v2 template with the admitted full source commit. The
default host check rejects missing Unix identities, wrong binary roles/digests,
invalid TLS/configuration references and policy drift. Its successful result is
`HOST_VALIDATED`, not READY, and still lists provider/live-service acceptance.

`--config-only` is available for outer-contract inspection before deployment;
it cannot admit the host or satisfy terminal acceptance.

Control Plane receives only the publisher plan plus an mTLS remote-client
configuration. The `engineering-publisher` service alone can read the GitHub
token file and execute Git/GitHub mutation.

Do not add `GITHUB_PUBLISHER_CONFIG_FILE` to `control.env`; preflight rejects
the legacy in-process production configuration.

## Health

The Control API already exposes `GET /healthz`. TLS transport remains required
by the server. Operational readiness must additionally verify PostgreSQL and
recovery state; the authenticated operational status below supplies those database facts.

## Authenticated operational status

After Control Plane is reachable, load a dedicated operator/read-only mTLS
client environment and run:

```sh
eng production-status --require-ready
```

The command reads `GET /api/v1/operations/status`. Detailed readiness is never
inferred from anonymous `/healthz` alone.

A nonzero exit from `--require-ready` blocks production mutation until the
reported Recovery/UNKNOWN/dead-letter/lease condition is reconciled. Do not
paper over a degraded status by restarting services or replaying a model turn.

## Restart semantics

systemd may restart admission/preparation/control processes after a crash.
It must not blindly replay a model turn or an external publication. Run,
execution epoch, Action Gateway operation and Recovery authority remain the
source of truth.
