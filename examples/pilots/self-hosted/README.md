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

Use a repository-scoped self-hosted runner dedicated to this pilot. Do not keep
a broadly reusable runner with ChatGPT/GitHub credentials permanently online.

Recommended M1 operation:

1. register the runner only for `jiying2007/engineering-platform`;
2. add the custom label `engineering-platform-codex`;
3. run it under the same trusted Linux account that owns the existing Codex and
   `gh` logins;
4. enable it only for the retained pilot window and remove/disable it after the
   pilot;
5. do not add `pull_request` or `push` triggers to the self-hosted engineer
   workflow.

The workflow itself is `workflow_dispatch` only and refuses to run unless
GitHub reports protected `refs/heads/main`.

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

After this branch is merged and the dedicated runner is online:

```sh
gh workflow run retained-pilot-self-hosted-engineer.yml \
  --repo jiying2007/engineering-platform \
  --ref main \
  -f pilot=feature \
  -f model=gpt-5.6-sol

gh run list \
  --repo jiying2007/engineering-platform \
  --workflow=retained-pilot-self-hosted-engineer.yml \
  --limit 5
```

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
