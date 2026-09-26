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

sed \
  -e "s#__PREPARATION_ROOT__#$PREPARATION_ROOT#g" \
  -e "s#__GIT_EXECUTABLE__#$GIT_EXECUTABLE#g" \
  -e "s#__CONTEXT_SOURCE__#$CONTEXT_SOURCE#g" \
  -e "s#__RUN_ID__#$RUN_ID#g" \
  -e "s#__TASK_DIGEST__#$TASK_DIGEST#g" \
  -e "s#__RUN_INPUT_DIGEST__#$RUN_INPUT_DIGEST#g" \
  -e "s#__REPOSITORY_PATH__#$REPOSITORY_PATH#g" \
  examples/pilots/worker-preparation.json.tmpl \
  > /operator/worker-preparation.json
```

The current pilot templates intentionally use no external ContextRefs, so the
operator-owned context-source directory may be empty but must still be a
canonical, non-group/world-writable absolute directory.

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
templates with deterministic placeholder values and proves the existing Core API
accepts Work -> Task -> Run before any external WIF or GitHub mutation is needed.


## After the real Codex execution FINISHES

Do not render these files before Core reports a FINISHED Codex execution and the
retained receipt/result commit/bundle values are known.

For the selected pilot directory, render the publication request:

```sh
EXECUTION_EPOCH=1
RECOVERY_EPOCH=0
CODEX_RESULT_DIGEST='<receipt.result_digest>'

sed \
  -e "s/__EXECUTION_EPOCH__/$EXECUTION_EPOCH/g" \
  -e "s/__RECOVERY_EPOCH__/$RECOVERY_EPOCH/g" \
  -e "s#__CODEX_RESULT_DIGEST__#$CODEX_RESULT_DIGEST#g" \
  "$PILOT/publish.json.tmpl" > /tmp/publish.json

eng api POST "/api/v1/runs/$RUN_ID/actions" /tmp/publish.json
```

Continue only after the corresponding Action operation is CONFIRMED and the PR
binds the exact Codex result commit.

After exact PR-head CI succeeds, retain the Codex receipt digest, result bundle,
Git changed-tree manifest digest and trusted-CI envelope ZIP digest. Then complete
the Run:

```sh
sed -e "s/__EXECUTION_EPOCH__/$EXECUTION_EPOCH/g" \
  "$PILOT/complete.json.tmpl" > /tmp/complete.json

eng api POST "/api/v1/runs/$RUN_ID/complete" /tmp/complete.json
```

Render Delivery only from retained bytes/facts:

```sh
RESULT_COMMIT='<exact Codex result commit>'
CODEX_RECEIPT_DIGEST='<eng codex-receipt-digest artifact_digest>'
BUNDLE_DIGEST='<retained Codex bundle digest>'
GIT_MANIFEST_DIGEST='<eng git-change-manifest artifact_digest>'
CI_ENVELOPE_DIGEST='<GitHub trusted-ci-evidence ZIP digest>'

sed \
  -e "s/__RESULT_COMMIT__/$RESULT_COMMIT/g" \
  -e "s#__CODEX_RECEIPT_DIGEST__#$CODEX_RECEIPT_DIGEST#g" \
  -e "s#__BUNDLE_DIGEST__#$BUNDLE_DIGEST#g" \
  -e "s#__GIT_MANIFEST_DIGEST__#$GIT_MANIFEST_DIGEST#g" \
  -e "s#__CI_ENVELOPE_DIGEST__#$CI_ENVELOPE_DIGEST#g" \
  "$PILOT/delivery.json.tmpl" > /tmp/delivery.json

eng api POST /api/v1/deliveries /tmp/delivery.json
```

Run the three existing importers under their dedicated mTLS identities. Use the
fixed requirement/evidence IDs from the pilot Task:

Feature:
- Codex: requirement `feature-codex`, evidence `feature-codex-evidence`
- Git: requirement `feature-git`, evidence `feature-git-evidence`
- CI: requirement `feature-ci`, evidence `feature-ci-evidence`

Debug:
- Codex: requirement `debug-codex`, evidence `debug-codex-evidence`
- Git: requirement `debug-git`, evidence `debug-git-evidence`
- CI: requirement `debug-ci`, evidence `debug-ci-evidence`

After all three Evidence items exist, submit the fixed
`verification.json`. Core computes PASS/FAIL; the request cannot choose the
result.

### Independent Review is intentionally not templated as PASS

The reviewer must independently inspect the exact Delivery/Verification subject.
Construct the review request only after that inspection. The minimum shape is:

```sh
jq -n \
  --arg review_id '<pilot-review-id>' \
  --arg delivery_id '<pilot-delivery-id>' \
  --arg verification_id '<pilot-verification-id>' \
  --arg reviewer 'urn:engineering-platform:reviewer:pilot' \
  --arg result "$REVIEW_RESULT" \
  --argjson findings "$REVIEW_FINDINGS_JSON" \
  --argjson known_limits "$REVIEW_KNOWN_LIMITS_JSON" \
  '{
    review_report_id:$review_id,
    delivery_receipt_id:$delivery_id,
    verification_report_id:$verification_id,
    reviewer:$reviewer,
    result:$result,
    findings:$findings,
    known_limits:$known_limits
  }' > /tmp/review.json

eng api POST /api/v1/reviews /tmp/review.json
```

Do not default `REVIEW_RESULT` to PASS. A FAIL review must be retained as FAIL.

Only after an exact PASS Verification and exact PASS independent Review may the
fixed `closure.json` be submitted.

The repository test `TestRetainedPilotPostModelTemplatesReachClosure` exercises
the post-model JSON/state-machine path with synthetic in-memory evidence. It is a
schema/lifecycle dry-run only; it is not retained model, GitHub, Evidence, Review
or M1 proof.
