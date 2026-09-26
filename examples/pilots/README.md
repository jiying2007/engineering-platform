# M1 retained pilot input pack

These templates front-load the Core Work/Task/Run inputs for the two retained
M1 pilots:

- Feature requirement: GitHub issue #54
- Debug requirement: GitHub issue #55

They are **pre-WIF readiness material only**. Rendering or dry-running them does
not create model Evidence and does not replace the real managed-workspace WIF
turn.

## Runtime values

Render only these deployment-time values:

- `__HUMAN_OWNER__`: exact authenticated Work owner subject.
- `__BASE_COMMIT__`: exact current 40-hex `main` commit frozen when the pilot starts.
- `__PROFILE_DIGEST__`: exact digest emitted by `eng codex-profile`.
- `__WORKER_PROFILE__`: exact WorkerProfile granted to the Core-bound Codex Worker.
- `__TASK_DIGEST__`: exact digest returned by Task creation.

Do not edit acceptance criteria, requirement IDs, procedures, issuers, action
allow-list, repository identity or pilot IDs after Task creation.

## Render and create

Example for Feature:

```sh
set -euo pipefail

PILOT=examples/pilots/feature-routing
BASE_COMMIT="$(git rev-parse main)"
PROFILE_DIGEST="$(jq -er .profile_digest codex-profile.json)"
HUMAN_OWNER='urn:engineering-platform:operator:pilot-owner'
WORKER_PROFILE='worker/codex-pilot'

sed   -e "s/__HUMAN_OWNER__/$HUMAN_OWNER/g"   "$PILOT/work.json.tmpl" > /tmp/feature-work.json

sed   -e "s/__BASE_COMMIT__/$BASE_COMMIT/g"   "$PILOT/task.json.tmpl" > /tmp/feature-task.json

eng api POST /api/v1/work-items /tmp/feature-work.json
TASK_DIGEST="$(
  eng api POST /api/v1/task-contracts /tmp/feature-task.json |
  jq -er .digest
)"

sed   -e "s/__TASK_DIGEST__/$TASK_DIGEST/g"   -e "s/__PROFILE_DIGEST__/$PROFILE_DIGEST/g"   -e "s#__WORKER_PROFILE__#$WORKER_PROFILE#g"   "$PILOT/run.json.tmpl" > /tmp/feature-run.json

eng api POST /api/v1/runs /tmp/feature-run.json
```

Repeat with `examples/pilots/debug-firmware-identity` after the Feature pilot
has closed. Re-read `main` and freeze a new base for Debug.

## Stop conditions

Stop rather than patching the templates when:

- current `main` differs from the frozen Task base;
- the installed Codex binary/model produces a different profile digest;
- the authenticated owner/Worker profile differs from the intended principal;
- WIF/publisher prerequisites are absent;
- Task material is not READY;
- the real Codex result is not FINISHED;
- publication is not CONFIRMED;
- exact PR-head trusted CI is not successful;
- Verification or independent Review is not PASS.

The repository test `TestRetainedPilotTemplatesDryRunToRunning` renders both
templates with deterministic placeholder values and proves the existing Core API
accepts Work -> Task -> Run before any external WIF or GitHub mutation is needed.
