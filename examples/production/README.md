# Production Ubuntu/systemd baseline

This directory is the production-v1 deployment baseline for issue #105.

It deliberately does **not** qualify a model provider and does not contain
credentials. Provider qualification remains a later live gate.

## Source-bound installation from delivered bytes

The delivered `eng` embeds these existing templates directly from this directory;
there is no second editable template catalog. After authenticating the original
CI/release artifact and its exact source commit independently, extract it to a
controlled directory and use that package's own executable:

```sh
/path/to/distribution/eng distribution-install \
  --from /path/to/distribution --into /opt/engineering-platform \
  --source-commit "$EXPECTED_SOURCE_COMMIT"
/opt/engineering-platform/bin/eng installation-verify \
  --dir /opt/engineering-platform --source-commit "$EXPECTED_SOURCE_COMMIT"
```

The destination must NOT exist, even as an empty directory. Its canonical parent
must be caller-owned, not shared-writable or setgid. An ordinary user can install
under a controlled home directory; an explicitly authorized administrator may
install below `/opt`. Installing bytes does not authorize running services as root.
The installer must itself identify the same clean source commit as every one of
the six delivered roles. A checksum or embedded VCS string alone is not source
or release authentication. The caller must already trust the package/expected SHA.

The result contains `bin/` (six roles plus existing distribution manifests),
`templates/` (these non-secret examples) and `installation-manifest.json`. Program
files are 0555, templates/manifests 0444 and directories 0755. They contain public
release bytes, NOT secret material. Before deployment, copy/configure the needed
environment/TLS policy files into separately protected 0600 configuration paths;
do not edit the installed templates in place or add credentials under this tree.

Copying uses exclusive creation, file/directory fsync and independent full
readback. An existing installation is never overwritten, including after a
failed prior attempt. Errors/cancellation can leave partial files. A manifest
being present is NOT completion; `installation-verify` checks exact inventory,
all bytes, source/roles, modes and ownership and rejects extra files/directories,
aliases, links and drift. Sources/host parents must be quiescent and controlled;
this is not protection against malicious root/same-UID changes to the host.

No dependency is downloaded or bundled, no account/configuration is provisioned,
no systemd unit is activated, and no SQL/migration/model/Git operation is run.
`services_started`, `configuration_applied`, `dependencies_included`,
`execution_authorized` and `production_qualified` remain false. Upgrades install
into another fresh directory; version switching, migration and rollback still
require their separate deployment authority and acceptance. There is no automatic
symlink switch or database downgrade.

Canonical tests use the delivered installer and remove the original extracted
package before verifying the installation. Existing native command integration
runs installed Worker/eng/guard with mTLS/PostgreSQL and a real isolated C build.
That test uses an in-process Core host; it is not systemd startup, complete
fresh-host dependency provisioning or installed Publisher qualification.

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
eng production-status
# Only check database authority, NOT service availability:
eng production-status --require-authority-clear
```

The command reads `GET /api/v1/operations/status`. Detailed readiness is never
inferred from anonymous `/healthz` alone.

Operational status v2 exposes database authority separately from service
readiness. Empty queues or recent database progress do not prove a consumer or
publisher is alive. This slice returns `service_readiness=NOT_OBSERVED` and
`ready=false`; `--require-ready` therefore fails until actual service observation
is implemented. Never replace a production readiness gate with
`--require-authority-clear` merely to make it pass. A nonzero authority check
requires reconciliation of Recovery/UNKNOWN/dead-letter/lease facts, not a restart
or model replay. Queue ages and last admission/dispatch timestamps are database
observations, not heartbeats or calibrated SLOs. Clients rederive the full response
and reject snapshots older than 30 seconds or more than 5 seconds in the future;
these transport freshness bounds are not production performance targets.

## Restart semantics

systemd may restart admission/preparation/control processes after a crash.
It must not blindly replay a model turn or an external publication. Run,
execution epoch, Action Gateway operation and Recovery authority remain the
source of truth.


## Endpoint startup ordering

Control and Publisher use `Type=notify` with `NotifyAccess=main`. Their main
process sends only `READY=1` when the actual HTTP/TLS serving loop first enters
Accept, after configuration, Control schema checks, listener binding and TLS
setup. Worker units keep their existing Requires/After dependency on Control;
that ordering now waits for Control initialization rather than process creation.
Publisher remains a Wants dependency: its outage does not remove Core read and
reconciliation access. Startup failure is not converted into a ready endpoint.

The optional manager address is consumed before serving and removed from the
child environment. Only a bounded Unix datagram is sent, once, with no retry or
plain-process fallback after failure. Without NOTIFY_SOCKET a directly invoked
process retains its explicit unmanaged behavior. An invalid or unreachable
configured notification socket fails startup. The manager authenticates the
sender; no wrapper, Worker, model or subprocess is granted notification rights.

This signal proves endpoint initialization, not authenticated client access,
upstream provider health, queue capacity, production readiness or replay
permission. Existing mTLS health checks and Core authority remain independent.
There is no watchdog, heartbeat, READY polling API or new execution authority.
Full service-graph, separate service-user, upgrade and rollback qualification
still require their own acceptance.
