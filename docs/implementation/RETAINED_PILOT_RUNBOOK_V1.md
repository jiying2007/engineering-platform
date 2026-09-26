# Retained Feature / Debug Pilot Runbook v1

Base readiness: main `316bba9fab02f9cdbbf0278cf89a21a25eea10fd`.

This runbook is intentionally operational. It does not add a Runtime lane,
Evidence family or Recovery mechanism. It describes how to exercise the
already-implemented authorities with one real Feature task and one real Debug
task.

A pilot is retained only when the exact chain reaches Closure:

```
Requirement
  -> frozen Task / Run / approved Context
  -> real managed-workspace WIF Codex engineering
  -> retained result commit + Git bundle
  -> independently authorized GitHub PR publication
  -> exact PR-head trusted CI
  -> Delivery
  -> Codex + Git + trusted-CI Evidence
  -> Verification
  -> independent Review
  -> Closure
```

Repository fake app-server tests are never accepted as real model evidence.

## 1. External prerequisites

Do not create a pilot WorkItem until all external prerequisites are ready.

### Managed-workspace WIF

The ChatGPT workspace administrator must enable/configure the Codex workload
identity provider/rule that accepts the approved workload identity.

For the repository WIF qualification workflow, configure repository variables:

- `OPENAI_WIF_AUDIENCE`
- `OPENAI_CODEX_FEDERATION_RULE_ID`

Run `.github/workflows/codex-wif-live.yml` manually on `main` and retain one
successful `codex-wif-live-<main-sha>-<run-id>` artifact. This proves the
external mapping works; it is a prerequisite, not the engineering Task Evidence.

The Core Worker execution host must separately receive a short-lived
`OPENAI_IDENTITY_TOKEN_FILE` acceptable to that same configured federation
rule. The Worker does not mint this assertion and refuses long-lived
`OPENAI_API_KEY` / `OPENAI_BASE_URL` substitution.

### Publisher credential

Start the production Control Plane with `GITHUB_PUBLISHER_CONFIG_FILE` pointing
to an operator-owned config conforming to
`examples/github-publisher.json`.

Prefer a short-lived GitHub App installation token in the configured owner-private
`token_file`. The publisher credential belongs to the Control Plane provider,
never to the Worker/Codex process.

The publisher artifact root must be a read-only shared view of the Worker's
retained Codex `artifacts/` directory.

### Control identities

Provision separate mTLS identities for:

- Work/Task/Run owner/operator;
- Core-bound Codex Worker;
- publication Action requester;
- Codex Evidence importer;
- Git Evidence importer;
- trusted-CI Evidence importer;
- Verifier;
- independent Reviewer;
- Closure authority.

The Reviewer must remain certificate-separated from the Work human owner and
Verifier. Evidence importer reserved identities remain dedicated importers.

## 2. Freeze the exact Codex profile

Install/review the qualified native `codex-cli 0.155.0` binary on the Worker
host, then derive the exact Core profile from its real bytes:

```sh
eng codex-profile \
  --codex /absolute/path/to/codex \
  --model gpt-5.6-sol \
  > codex-profile.json
```

Use the returned values without editing:

- `profile` -> `WORKER_CODEX_CONFIG.profile`;
- `profile_digest` -> Worker access-policy
  `worker.codex-execute` action capability;
- `tool_profile` -> frozen RunInput `tool_profile`;
- `action_grant` -> exact Worker action grant.

A Worker principal for this lane needs the existing poll/report/preparation
capabilities, the selected worker profile, `action:execute`, and only the exact
Core-bound Codex action grant required by policy. Do not grant
`github.publish-pr` to the Worker.

The Worker Codex config also binds the exact native executable and approved
federation rule ID.

## 3. Select a small real pilot

Use a real repository and an exact current 40-character base commit.

Feature pilot characteristics:

- Task type `FEATURE`;
- a small deterministic source change;
- acceptance criteria fully testable in repository CI;
- no device dependency unless the criterion genuinely requires it.

Debug pilot characteristics:

- Task type `DEBUG`;
- include an authoritative log or an exact reproduction;
- acceptance criteria include both the fix and regression prevention.

Do not use marker-only or documentation-only fake engineering changes merely to
make the platform lifecycle pass.

Before creating each Task, verify `main` still equals the intended frozen base.
If main moved, freeze a new Task/Run rather than silently rebasing the old one.

## 4. Verification plan

Each acceptance criterion must have explicit requirement IDs. For a retained
engineering pilot, the plan should require the evidence actually needed for the
criterion. The standard procedures available in the current platform are:

- issuer `codex-execution-importer`, procedure `codex.core.execution.v1`;
- issuer `git-change-importer`, procedure `git.changed-tree.v1`;
- issuer `github-actions-importer`, procedure
  `github.actions.trusted-ci.v1`.

Use stable unique requirement IDs per criterion. Do not reuse one evidence item
under a different requirement ID.

The frozen Task `allowed_actions` must include:

- `worker.codex-execute`;
- `github.publish-pr`.

## 5. Create Work, Task and Run

Use the authenticated `eng api` front door with JSON files, for example:

```sh
eng api POST /api/v1/work-items work.json
eng api POST /api/v1/task-contracts task.json
eng api POST /api/v1/runs run.json
```

The RunInput must bind:

- the exact Task digest returned by Task creation;
- the approved ContextRefs;
- the exact `codex/<profile-digest>` tool profile;
- the exact WorkerProfile granted to the Worker;
- explicit runtime/policy profiles.

The Work/Task/Run must remain on one immutable base/result lineage.

## 6. Admit, prepare and execute the real Codex turn

First consume the normal Worker admission/preparation flow for the exact Run.
Then, with the prepared Run and short-lived WIF assertion available:

```sh
WORKER_PREPARATION_CONFIG=/operator/preparation.json \
WORKER_CODEX_CONFIG=/operator/codex.json \
OPENAI_IDENTITY_TOKEN_FILE=/run/identity/assertion.jwt \
eng-worker --profile <worker-profile> \
  --execute-codex --run <run-id> --once
```

Use the repository's built `worker` binary name/path in deployment; the example
above names it `eng-worker` only to distinguish it from the `eng` CLI.

Success requires Core `GET /api/v1/runs/<run-id>/codex` to show:

- state `FINISHED`;
- a `WORKER_ATTESTED_CODEX_EXECUTION` receipt;
- zero approval requests;
- a changed result commit/tree/source digest;
- retained Git bundle digest and size.

Any `AUTHORIZED` or `UNKNOWN` execution is not a successful pilot. Reconcile
it before continuing; never automatically replay the model turn.

## 7. Publish while the Run is still RUNNING

Publication must occur before Run completion because Action Gateway checks the
live Run execution epoch.

Read the FINISHED Codex status and use its exact
`receipt.result_digest` as the publication action `parameters_digest`.

The publication Action request is:

- action `github.publish-pr`;
- risk `CONTROLLED_MUTATION`;
- capability `github.publish-pr`;
- current execution epoch;
- current recovery epoch;
- an immutable idempotency key;
- `requested_by` equal to the authenticated publication requester identity.

Submit:

```sh
eng api POST /api/v1/runs/<run-id>/actions publish.json
```

Then read `GET /api/v1/actions/<action-id>`.

Continue only when the external operation is `CONFIRMED` and the retained
publication receipt binds the exact repository/base/result branch/commit and
open PR.

If the operation is `UNKNOWN`, call the existing reconcile endpoint. The
provider observes GitHub state only; it does not replay push/PR mutation.

## 8. Require exact PR-head CI

The PR generated by the publisher automatically runs `CI`.

Trusted pilot evidence requires the CI run to test the exact Codex result commit,
not GitHub's synthetic merge commit. Current CI:

- checks out `pull_request.head.sha` on PR events;
- names retained artifacts by that exact source SHA;
- retains source/base SHA in the CI receipt.

For PR-head Evidence import, the live importer additionally requires:

- source SHA == tested SHA == Delivery result commit;
- PR base SHA == Delivery base commit;
- head/base repository IDs are the trusted repository;
- base ref is `main`;
- live run/jobs/artifacts bind the exact result commit;
- `.github/workflows/ci.yml` has the same Git blob SHA at base and result.

Therefore a pilot PR must not modify the trusted CI workflow. A workflow change,
fork, base drift or merge-SHA test fails closed.

Do not merge the pilot PR merely to obtain CI Evidence. The PR-head run is the
evidence for the exact result commit.

## 9. Retain evidence material

After exact PR-head CI succeeds, retain locally:

- the Codex execution receipt digest;
- the exact Git bundle;
- a Git changed-tree manifest;
- trusted-CI envelope ZIP;
- engineering-binaries ZIP;
- Codex qualification ZIP.

Useful existing commands:

```sh
eng codex-receipt-digest --run <run-id>

eng git-change-manifest \
  --repository /operator/approved/repository \
  --base <base-commit> \
  --result <result-commit> \
  --output git-change.json
```

The operator-approved repository used for Git changed-tree capture must contain
both exact commits. Fetch the already-published result branch with operator Git
authority; do not give that authority to the Worker.

## 10. Complete Run and create Delivery

Only after publication and exact PR-head CI have succeeded:

```sh
eng api POST /api/v1/runs/<run-id>/complete complete.json
```

This moves Work from EXECUTING to VERIFYING.

Create one Delivery whose `result_commit` is the original Codex result commit.
Bind separate artifact IDs/digests for at least:

- canonical Codex execution receipt;
- exact result Git bundle;
- Git changed-tree manifest file;
- exact trusted-CI evidence ZIP.

Artifact IDs are part of the Delivery subject. Freeze them once.

## 11. Import requirement-bound Evidence

Run each importer under its dedicated mTLS identity.

Codex:

```sh
eng import-codex-evidence \
  --delivery <delivery-id> \
  --requirement <codex-requirement-id> \
  --evidence <codex-evidence-id> \
  --receipt-artifact <receipt-artifact-id> \
  --bundle-artifact <bundle-artifact-id> \
  --bundle <retained.bundle>
```

Git changed tree:

```sh
eng import-git-change-evidence \
  --repository /operator/approved/repository \
  --manifest git-change.json \
  --delivery <delivery-id> \
  --requirement <git-requirement-id> \
  --evidence <git-evidence-id> \
  --artifact <git-manifest-artifact-id>
```

Trusted CI:

```sh
eng import-ci-evidence \
  --delivery <delivery-id> \
  --requirement <ci-requirement-id> \
  --evidence <ci-evidence-id> \
  --artifact <ci-envelope-artifact-id> \
  --envelope-zip trusted-ci-evidence.zip \
  --binaries-zip engineering-binaries.zip \
  --codex-zip codex-qualification.zip
```

PR-head and main-push modes share the same trusted-CI Evidence procedure. The
importer derives which strict mode applies from the retained receipt and live
GitHub facts; callers do not supply a trust-mode override.

## 12. Verification, Review and Closure

Create Verification with exactly the Evidence IDs required by the frozen plan.
Proceed only on PASS.

Create Review under the independent Reviewer certificate. The reviewer must
inspect the exact Delivery/Verification subject, result diff and retained
evidence. A FAIL review is retained; do not overwrite it.

Only an exact PASS Verification plus exact PASS independent Review may create
Closure.

Retain the final set of IDs for the pilot:

- WorkItem;
- Task digest;
- Run + execution epoch;
- Codex execution receipt/result commit/bundle digest;
- publication Action operation + PR;
- exact CI run + artifact IDs/digests;
- Delivery;
- Evidence IDs;
- VerificationReport;
- ReviewReport;
- ClosureReceipt.

## 13. Debug pilot

Repeat the entire lifecycle with a new Work/Task/Run. Do not clone the Feature
evidence IDs or receipts.

The Debug MaterialManifest must contain either:

- `has_authoritative_log=true`; or
- `has_reproduction=true`.

A real Debug pilot should demonstrate that the retained reproduction/log drives
the fix and that the exact CI/Verification evidence covers the regression.

## Stop conditions

Stop the pilot rather than weakening authority when any of these occurs:

- WIF qualification has no successful retained real turn;
- Worker assertion cannot be minted by the approved external workload identity;
- publisher credential or artifact view is absent;
- main/base commit changes before publication;
- Codex status is not FINISHED;
- publication is UNKNOWN/MANUAL;
- PR tests a SHA other than the result commit;
- PR modifies the trusted CI workflow;
- any required artifact digest changes;
- Verification or independent Review is not PASS.

Two complete retained Closure chains — one Feature and one Debug — are the next
evidence needed before reassessing M1.
