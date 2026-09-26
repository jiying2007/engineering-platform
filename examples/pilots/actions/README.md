# GitHub-hosted retained pilot execution

The GitHub-hosted path is an operator shortcut for the same Core authorities.
It does not create a second Runtime or Evidence path.

## Engineer workflow

The retained-pilot-engineer workflow runs on protected main and:

1. installs the exact qualified Codex 0.155.0 native binary;
2. boots an ephemeral PostgreSQL + mTLS Core;
3. freezes the selected real Feature/Debug Work/Task/Run and approved requirement context;
4. performs one real read-only WIF qualification;
5. performs the one retained Core-bound Codex engineering turn;
6. snapshots the FINISHED PostgreSQL state and result bundle before publication;
7. only after the model process has exited, exposes the job-scoped GitHub token to
   the independent Action Gateway publisher;
8. creates the retained PR and snapshots resumable post-publication state.

The model-phase step never receives github.token, GITHUB_TOKEN, GH_TOKEN or
PUBLISH_TOKEN. Checkout uses persist-credentials false.

## WIF administrator rule

The managed-workspace federation rule must accept the exact protected-main
identity for this workflow in addition to the standalone qualification workflow.

Keep exact repository/ref/audience checks and allow only these workflow refs:

- jiying2007/engineering-platform/.github/workflows/codex-wif-live.yml@refs/heads/main
- jiying2007/engineering-platform/.github/workflows/retained-pilot-engineer.yml@refs/heads/main

A single rule may express the two allowed workflow refs with its exact-claim/CEL
condition, or an administrator may use a second rule mapped to the same
least-privilege managed-workspace principal.

## PR CI gate

The first pilot may use the workflow job-scoped GITHUB_TOKEN as the publisher
credential. GitHub may mark the resulting PR workflow run as requiring approval.
A repository write user must approve that run; the engineer workflow does not
wait on an idle runner.

The workflow retains a retained-pilot-engineering-<pilot>-<run-id> artifact
containing the PostgreSQL state, result bundle, WIF receipt, publication receipt
and exact frozen identities. The verification workflow resumes only after exact
PR-head CI succeeds.

Independent Review and Closure are deliberately outside the engineer workflow.

## Verification workflow

After the engineer workflow creates the retained PR, approve the generated
approval-required PR CI run when GitHub requests it. Wait for the exact PR-head
CI to pass all five gates, then dispatch:

```sh
gh workflow run retained-pilot-verify.yml \
  --repo jiying2007/engineering-platform \
  --ref main \
  -f pilot=feature \
  -f engineering_run_id=<engineer-workflow-run-id>
```

The verification workflow:

1. downloads the exact retained engineering state;
2. rebuilds `control-plane` and `eng` from the frozen engineering base;
3. restores the post-publication PostgreSQL state;
4. discovers one successful exact PR-head CI run;
5. downloads and verifies the raw GitHub artifact ZIPs;
6. completes the Run;
7. creates Delivery bound to the exact Codex receipt/bundle, Git manifest and
   GitHub CI artifact digest;
8. imports Codex/Git/CI Evidence under three dedicated mTLS identities;
9. creates Verification under a separate verifier identity;
10. retains the PASS verification state for independent Review.

It does not create a Review or Closure. A Verification PASS is only the input to
the next independent-review gate.

## Independent Review and conditional Closure

After the retained verification workflow succeeds, a different GitHub account
from the engineering workflow dispatcher performs the independent review:

```sh
gh workflow run retained-pilot-review.yml \
  --repo jiying2007/engineering-platform \
  --ref main \
  -f pilot=feature \
  -f verification_run_id=<verification-workflow-run-id> \
  -f result=PASS \
  -f findings_json='[]' \
  -f known_limits_json='[]'
```

The workflow queries the original engineering workflow actor and refuses to
submit Review when the current `GITHUB_ACTOR` is the same account. It restores
the PASS-verified Core state and submits Review under the dedicated
`urn:engineering-platform:reviewer:pilot` mTLS identity.

`result=PASS` is never inferred. It is explicit reviewer input. Core still
validates the Review report; PASS cannot contain blocking findings, and FAIL
requires at least one blocking finding.

When the independent Review is PASS, the workflow changes to the separate
closure mTLS identity and submits the frozen Closure request. When Review is
FAIL, it retains the FAIL Review and confirms the Work is not CLOSED.

The retained review artifact records the engineering GitHub actor, reviewer
GitHub actor, review decision, closure state, and final PostgreSQL snapshot.
