# Local retained-pilot stack

This is an operator convenience layer for the first retained Feature/Debug pilots.
It does not add Core authority or replace WIF.

It creates only:

- a short-lived 7-day local pilot CA;
- one TLS server certificate for loopback control-plane;
- one separate client certificate per existing pilot principal;
- rendered least-privilege access policy;
- local publisher config bound to the Worker's retained `preparation-root/artifacts` view;
- local PostgreSQL 17 helper;
- shell environment files for control-plane and each client identity.

The PKI is for the retained pilot on one trusted Ubuntu host only. Do not reuse it
as production PKI.

## Bootstrap

First derive the exact Codex profile:

```sh
go run ./cmd/eng codex-profile \
  --codex /absolute/path/to/codex \
  --model gpt-5.6-sol \
  > /operator/codex-profile.json

PROFILE_DIGEST="$(jq -er .profile_digest /operator/codex-profile.json)"
```

Then bootstrap the local stack:

```sh
bash examples/pilots/local-stack/bootstrap.sh \
  /var/lib/engineering-platform/pilot \
  "$PROFILE_DIGEST" \
  worker/codex-pilot
```

Start PostgreSQL:

```sh
bash examples/pilots/local-stack/postgres.sh /var/lib/engineering-platform/pilot
```

Build the binaries:

```sh
go build -o /operator/bin/control-plane ./cmd/control-plane
go build -o /operator/bin/eng ./cmd/eng
go build -o /operator/bin/worker ./cmd/worker
```

Start Core without publisher first:

```sh
set -a
. /var/lib/engineering-platform/pilot/operator/control-plane.env
set +a
exec /operator/bin/control-plane
```

Health check:

```sh
bash examples/pilots/local-stack/status.sh /var/lib/engineering-platform/pilot
```

Use the owner identity for Work/Task/Run:

```sh
. /var/lib/engineering-platform/pilot/clients/owner.env
/operator/bin/eng api GET /api/v1/capabilities
```

The generated client identities are:

- `owner`
- `worker`
- `publisher`
- `codex-evidence`
- `git-evidence`
- `ci-evidence`
- `verifier`
- `reviewer`
- `closure`

## Publisher handoff

The bootstrap intentionally does **not** create
`secrets/github-token`. Until a real publisher credential is provisioned,
leave the control-plane running without publisher.

After provisioning an owner-private publisher token:

```sh
install -m 0600 /secure/source/token \
  /var/lib/engineering-platform/pilot/secrets/github-token
```

Stop/restart the same control-plane against the same PostgreSQL database using:

```sh
set -a
. /var/lib/engineering-platform/pilot/operator/control-plane-with-publisher.env
set +a
exec /operator/bin/control-plane
```

This preserves Task/Run/Codex state while enabling the independent publisher.
The publisher reads the exact retained Git bundle from
`preparation-root/artifacts/<codex-execution-id>.bundle`; there is no second
copy/staging directory. `eng pilot-preflight` rejects a publisher config whose
artifact root is not exactly this retained Worker artifact view.

## WIF boundary

The local stack does not mint OIDC tokens and does not alter the managed
ChatGPT workspace. Continue to treat the real federation rule and one retained
`codex-wif-live` receipt as external prerequisites.

Once those exist, render the Worker Codex config and run
`eng pilot-preflight`. Only consume the Feature model turn when internal,
model_execution and publication all report READY.
