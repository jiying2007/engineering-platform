# Codex workload-identity live turn v1

Base: `c817876c8c10fddebe21fc38d394fee944f66c47` (#37).

This increment prepares and constrains the first real authenticated Codex model
turn without storing a long-lived OpenAI/ChatGPT credential. It does not count as
a live qualification until the manual workflow actually succeeds and retains its
receipt.

## Authentication contract

Codex workload identity federation (WIF) is the preferred automation credential.
The process receives:

- `OPENAI_FEDERATION_RULE_ID`: non-secret exact Codex federation rule;
- `OPENAI_IDENTITY_TOKEN_FILE`: absolute path to the current upstream identity
  assertion;
- optional `OPENAI_WORKLOAD_IDENTITY_CONTEXT`: bounded JSON audit attribution.

Provider launch fails closed when only one required variable is set. WIF cannot
be mixed with `OPENAI_API_KEY` or an alternate `OPENAI_BASE_URL`. Audit context
without the two required WIF fields is rejected.

The token itself is never copied into Runtime HOME, Context, a receipt or an
artifact. Its path must resolve without symlinks to an owner-private regular file
inside an owner-private directory. The token path must be outside the worktree and
Runtime HOME. Errors never include token contents.

This follows Codex credential precedence rather than relying on fallback: the
presence of either required WIF variable selects WIF, and incomplete WIF must be
an error rather than silently trying another credential.

## GitHub Actions OIDC lane

`.github/workflows/codex-wif-live.yml` is manual-only and grants only:

- `contents: read`
- `id-token: write`

The live job requires:

- invocation from `refs/heads/main`;
- repository `jiying2007/engineering-platform`;
- repository variable `OPENAI_WIF_AUDIENCE`;
- repository variable `OPENAI_CODEX_FEDERATION_RULE_ID`.

It asks GitHub for one signed OIDC JWT using the configured audience, writes it
with mode 0600 under a dedicated 0700 directory, then locally decodes only the
JWT claims needed to reject an unexpected issuer/audience/repository/ref or
workflow_ref. It does not print the bearer assertion.

The Codex federation rule in the managed ChatGPT workspace must independently
verify the GitHub issuer and narrowly bind repository/workflow/ref or equivalent
claims to a dedicated principal. The GitHub-side checks are defense in depth, not
a replacement for the OpenAI federation rule.

## No login-status preflight

The workflow intentionally does not run `codex login status` before the live
turn. When assertion replay protection is enabled and the upstream JWT contains a
unique `jti`, an authentication check can consume that assertion for an exchange.
The one minted assertion is reserved for the actual app-server process.

Long-running production Workers must refresh the identity token file before the
upstream assertion expires. This one-turn qualification is short-lived and does
not implement a refresh agent.

## Live-turn acceptance

`cmd/codex-wif-live` launches the already qualified native `codex-cli 0.155.0`
through `NewPinnedProvider`. It rechecks exact executable bytes and exact
`codex-cli 0.155.0` before starting app-server.

The thread remains:

- fresh process;
- stdio transport;
- ephemeral;
- `sandbox=read-only`;
- `approvalPolicy=never`.

The prompt is fixed:

`Reply with only: engineering-platform live qualification`

The observer accepts a terminal result only when:

- the matching turn reaches `completed`;
- at least one completed `agentMessage` contains bounded UTF-8 output;
- no server approval request occurred;
- no effect-bearing or unknown completed item type occurred.

Known non-effect timeline items (`userMessage`, `reasoning`, `plan`) are
allowed. Command/file/tool items fail the qualification even if Codex itself would
eventually deny them.

The retained receipt includes the exact qualified binary digest, model input,
federation rule ID, prompt digest, thread/turn IDs, terminal status and bounded
model output + digest. It contains neither the identity token nor its filesystem
path.

## External prerequisite

Codex workload identity federation is currently beta for managed ChatGPT
workspaces and must be enabled/configured by an administrator. Source merge alone
cannot create the federation provider/rule, principal membership or repository
variables.

Until an administrator completes that setup and a manual
`Codex WIF live qualification` workflow succeeds, the repository must continue
to state:

**WIF-ready, no retained authenticated model-turn proof.**

## After the first successful live receipt

A qualification turn is still not a Feature/Debug pilot. The next connected
runtime increment must:

1. bind the live process to a current Core Run/execution lease rather than a
   workflow-only qualification;
2. issue fresh WIF assertions through a host credential broker;
3. allow workspace-write only under the OS execution boundary already established;
4. route every command/file/network approval through Action Gateway authority;
5. retain provider output, changed-tree and CI provenance as Run-bound Evidence;
6. run independent Review, reconciliation/restore, then real Feature and Debug
   pilots.

References:
- OpenAI Workload identity federation guide
- OpenAI Codex workload identity guide
- OpenAI GitHub Actions WIF guide
- OpenAI Codex federation rule reference
