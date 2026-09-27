# Trusted self-hosted retained Codex execution

This directory is the M1 phase-1 authentication path for a trusted Linux host
where Codex CLI is already logged in with ChatGPT. It changes only the Codex
authentication source. Core, publication, CI, Evidence, Verification, Review and
Closure remain the same authorities used by the WIF lane.

## Trust boundary

The Linux host/account is explicitly trusted for M1 phase 1. This is intentionally
weaker than managed-workspace WIF for unattended operation.

The model process receives:

- a fresh isolated Codex HOME, never the operator HOME;
- only the frozen exact-base worktree and approved Context bundle;
- no GitHub publisher token;
- no `OPENAI_API_KEY`, `CODEX_API_KEY`, `CODEX_ACCESS_TOKEN` or WIF input;
- no tool network and approval policy `never`.

The operator `auth.json` is copied into the isolated HOME only for app-server
authentication prewarm. The isolated copy is deleted and verified absent before
`thread/start`. The operator source file is not modified.

After the Core-bound Codex turn is FINISHED and the result bundle plus
pre-publication PostgreSQL snapshot are retained, the Codex process is stopped.
Only then may the independent Action Gateway publisher read the trusted host's
`gh` credential.

## Runner policy

This repository is public, so do **not** attach a persistent self-hosted runner
with reusable default labels. Use one ephemeral repository runner for exactly
one retained engineering job.

For each pilot:

1. generate a fresh 128-bit custom label:
   `engineering-platform-codex-<32 lowercase hex>`;
2. dispatch the protected-main workflow with that label while no matching runner
   is online, so the intended job is already queued;
3. register one repository-scoped runner with **only** that custom label by
   using both `--no-default-labels` and `--ephemeral`;
4. start the runner under the trusted Linux account that owns the existing Codex
   and `gh` logins;
5. let GitHub assign the already-queued matching job; the runner automatically
   deregisters after that one job;
6. delete the local runner work directory before the next pilot and generate a
   different random label.

Do not configure a long-lived `self-hosted` / `linux` / `x64` labelled
runner for this public repository. An unrelated workflow requiring only those
default labels would otherwise also be eligible to run on the trusted machine.

The engineer workflow is `workflow_dispatch` only, requires protected
`refs/heads/main`, and validates the one-time label format before any pilot
work starts.

## Host prerequisites

On the runner account:

```sh
CODEX_NATIVE="$(readlink -f "$(command -v codex)")"
SAVED_LOGIN_FILE="$(readlink -f "${CODEX_HOME:-$HOME/.codex}/auth.json")"

"$CODEX_NATIVE" --version
"$CODEX_NATIVE" login status
gh auth status
docker version
go version
```

Required Codex state:

```text
codex-cli 0.155.0
Logged in using ChatGPT
```

Do not print or upload `auth.json`.

The `gh` credential is the phase-1 publisher credential. It is never exported
to the model process. A least-privilege GitHub App publisher remains preferable
for longer-lived operation.

## Optional saved-login diagnostic

A separate model probe is **not required** before the retained engineering turn.
That avoids consuming/refreshing the saved login twice.

When diagnosing the host before a pilot, the optional probe is:

```sh
bash examples/pilots/self-hosted/qualify-login.sh \
  "$HOME/operator/self-hosted-login" \
  "$CODEX_NATIVE" \
  "$SAVED_LOGIN_FILE" \
  gpt-5.6-sol
```

It creates a read-only live receipt and deletes the isolated auth bootstrap
before its model turn. It is diagnostic evidence only, not the retained Feature
or Debug engineering Evidence.

## Run Feature

First generate the one-time runner label and queue the intended job:

```sh
RUNNER_LABEL="engineering-platform-codex-$(openssl rand -hex 16)"

gh workflow run retained-pilot-self-hosted-engineer.yml \
  --repo jiying2007/engineering-platform \
  --ref main \
  -f pilot=feature \
  -f model=gpt-5.6-sol \
  -f runner_label="$RUNNER_LABEL"

gh run list \
  --repo jiying2007/engineering-platform \
  --workflow=retained-pilot-self-hosted-engineer.yml \
  --limit 5
```

Then, from an already installed GitHub Actions runner directory on AiotServer01,
mint the short-lived repository registration token and configure the runner
without default labels:

```sh
REG_TOKEN="$(
  gh api --method POST \
    repos/jiying2007/engineering-platform/actions/runners/registration-token \
    --jq .token
)"

./config.sh \
  --url https://github.com/jiying2007/engineering-platform \
  --token "$REG_TOKEN" \
  --name "AiotServer01-m1-feature" \
  --labels "$RUNNER_LABEL" \
  --no-default-labels \
  --ephemeral \
  --unattended

unset REG_TOKEN
./run.sh
```

Use GitHub's **Settings → Actions → Runners → New self-hosted runner** page to
install/update the runner application itself before these commands. The
registration token is time-limited and must not be retained. After `run.sh`
finishes the single job, remove the local runner directory before the Debug
pilot.

The workflow performs:

```text
protected main
  -> frozen Work / Task / Run / approved Context
  -> isolated saved-ChatGPT-login prewarm
  -> delete isolated auth.json
  -> one Core-bound Codex engineering turn
  -> FINISHED result commit + Git bundle
  -> stop model process
  -> materialize host gh publisher token
  -> independent Action Gateway publication
  -> retained engineering artifact
```

Success is not Closure. It only advances to exact PR-head CI.

## Continue the existing chain

After the generated PR has exact PR-head CI PASS:

```sh
gh workflow run retained-pilot-verify.yml \
  --repo jiying2007/engineering-platform \
  --ref main \
  -f pilot=feature \
  -f engineering_run_id=<self-hosted-engineering-run-id>
```

Then a GitHub actor different from the engineering dispatcher performs the
existing independent Review workflow. PASS Review alone may create Closure.

Feature #54 must reach Closure before Debug #55 starts. Debug freezes the
then-current protected main and uses separate Work/Task/Run/Evidence/Review/
Closure identities.

## WIF remains the unattended path

`examples/pilots/actions/` and `examples/pilots/wif/` remain the
managed-workspace WIF path. A self-hosted Closure proves the trusted self-hosted
authentication mode only; it does not claim WIF qualification.
