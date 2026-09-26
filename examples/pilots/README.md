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

The requirement Context digest is **not** a free runtime value. Each pilot Run
template already pins the raw-byte SHA-256 of its sibling `requirement.md`.
Recompute it before execution and stop on mismatch.

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
CONTEXT_DIGEST="sha256:$(sha256sum "$PILOT/requirement.md" | awk '{print $1}')"
EXPECTED_CONTEXT_DIGEST="$(
  jq -er '.run_input.context_refs[0].digest' "$PILOT/run.json.tmpl"
)"
test "$CONTEXT_DIGEST" = "$EXPECTED_CONTEXT_DIGEST"

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


## Worker configuration after Run creation

Capture the Run response instead of discarding it:

```sh
RUN_RESPONSE="$(eng api POST /api/v1/runs /tmp/feature-run.json)"
RUN_INPUT_DIGEST="$(printf '%s\n' "$RUN_RESPONSE" | jq -er .run.run_input_manifest_digest)"
```

Render the preparation snapshot only after both Task and Run identities are
frozen:

```sh
RUN_ID='m1-feature-routing-run'
REPOSITORY_PATH='/absolute/operator/checkout/engineering-platform'
PREPARATION_ROOT='/var/lib/engineering-platform/codex-pilot'
CONTEXT_SOURCE='/var/lib/engineering-platform/context-source'
GIT_EXECUTABLE='/usr/bin/git'

install -d -m 0700 "$CONTEXT_SOURCE"
context_hex="${CONTEXT_DIGEST#sha256:}"
install -m 0400 "$PILOT/requirement.md" "$CONTEXT_SOURCE/$context_hex.bin"

sed \
  -e "s#__PREPARATION_ROOT__#$PREPARATION_ROOT#g" \
  -e "s#__GIT_EXECUTABLE__#$GIT_EXECUTABLE#g" \
  -e "s#__CONTEXT_SOURCE__#$CONTEXT_SOURCE#g" \
  -e "s#__RUN_ID__#$RUN_ID#g" \
  -e "s#__TASK_DIGEST__#$TASK_DIGEST#g" \
  -e "s#__RUN_INPUT_DIGEST__#$RUN_INPUT_DIGEST#g" \
  -e "s#__REPOSITORY_PATH__#$REPOSITORY_PATH#g" \
  -e "s#__CONTEXT_DIGEST__#$CONTEXT_DIGEST#g" \
  examples/pilots/worker-preparation.json.tmpl \
  > /operator/worker-preparation.json
```

Each pilot has exactly one approved DOCUMENT ContextRef named
`m1-pilot-requirement`. Its bytes are the frozen GitHub requirement/reproduction
snapshot committed beside the Run template. The Worker reads those bytes from
the operator-owned content-addressed ContextSource; Codex receives only the
materialized approved context bundle path and does not need GitHub/network
access. The ContextSource directory must be canonical and non-group/world
writable; the `<digest>.bin` requirement file must be read-only.

Generate the Worker Codex configuration from the exact output of
`eng codex-profile`; do not retype the profile object:

```sh
FEDERATION_RULE_ID='<administrator-provided-rule-id>'

jq --arg rule "$FEDERATION_RULE_ID" \
  '{
    version: 1,
    codex_executable: .codex_executable,
    federation_rule_id: $rule,
    profile: .profile
  }' \
  codex-profile.json \
  > /operator/worker-codex.json
```

At execution time the only WIF secret input is the short-lived
`OPENAI_IDENTITY_TOKEN_FILE`; it is not written into either configuration
file.

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
templates with deterministic placeholder values, verifies the committed
`requirement.md` raw-byte digest against the frozen ContextRef, and proves the
existing Core API accepts Work -> Task -> Run before any external WIF or GitHub
mutation is needed. `TestRetainedPilotPreparationTemplateLoads` additionally
opens the approved read-only content-addressed ContextSource bytes.
