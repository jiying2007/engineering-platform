# Trusted self-hosted retained Codex execution

This directory is the M1 phase-1 authentication path for a trusted Linux host
where Codex CLI is already logged in with ChatGPT.

It changes only the Codex authentication source. It does not create a second
Core, publication, Evidence, Verification, Review or Closure model.

## Trust boundary

The Linux host/account is explicitly trusted for this phase. This is weaker than
the future managed-workspace WIF lane for unattended execution because unrelated
credentials may exist elsewhere under the same OS account.

The retained model process still receives:

- a fresh isolated Codex HOME, never the operator HOME;
- the frozen exact-base workspace and approved Context only;
- no GitHub publisher token;
- no `OPENAI_API_KEY`, `CODEX_API_KEY`, `CODEX_ACCESS_TOKEN` or WIF input;
- no tool network and no approval grant.

The existing saved ChatGPT `auth.json` is copied into the isolated HOME only
for app-server authentication prewarm. The copy must be deleted and verified
absent before `thread/start`. The operator source file is not modified.

## 1. Locate the exact native Codex binary and saved login

Use canonical absolute paths:

```sh
CODEX_NATIVE="$(readlink -f "$(command -v codex)")"
SAVED_LOGIN_FILE="$(readlink -f "$HOME/.codex/auth.json")"

"$CODEX_NATIVE" --version
"$CODEX_NATIVE" login status
```

Required:

```text
codex-cli 0.155.0
Logged in using ChatGPT
```

Do not export or print the contents of `auth.json`.

## 2. Qualify the existing login

```sh
bash examples/pilots/self-hosted/qualify-login.sh \
  "$HOME/operator/self-hosted-login" \
  "$CODEX_NATIVE" \
  "$SAVED_LOGIN_FILE" \
  gpt-5.6-sol
```

Expected:

```text
trusted self-hosted Codex login qualification: READY
credential_mode=saved_chatgpt_login
...
```

The retained receipt is
`$HOME/operator/self-hosted-login/saved-login-live-receipt.json`.

## 3. Run one retained model phase

Feature first:

```sh
bash examples/pilots/self-hosted/engineer-model.sh \
  feature \
  "$HOME/operator/retained-feature" \
  "$CODEX_NATIVE" \
  "$SAVED_LOGIN_FILE" \
  gpt-5.6-sol
```

This command refuses to start if publisher/API/WIF credentials are present in
its environment, if the checkout is dirty, if local/remote main differ, or if
GitHub no longer reports protected main.

Success means only:

```text
trusted self-hosted retained pilot model phase: FINISHED
...
next=independent publication; no model replay
```

The model result, Git bundle and pre-publication PostgreSQL snapshot are retained
under the supplied output root. FINISHED is not M1 Closure and is not permission
to replay the model turn after a later publication failure.

## 4. Continue the existing authority chain

After FINISHED, publication must happen in a separate operator phase that may
hold GitHub write authority only after the Codex process has exited:

```text
FINISHED Codex
  -> independent Action Gateway publication
  -> exact PR-head CI
  -> Delivery
  -> Codex/Git/CI Evidence
  -> Verification
  -> independent Review
  -> Closure
```

Feature #54 must reach Closure before starting Debug #55. Debug must freeze the
then-current protected main and use new Work/Task/Run/Evidence/Review identities.

## WIF remains supported

`examples/pilots/actions/` and `examples/pilots/wif/` remain the
GitHub-hosted/unattended managed-workspace WIF path. A self-hosted M1 proof does
not claim that WIF itself has been qualified.
