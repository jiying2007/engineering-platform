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
