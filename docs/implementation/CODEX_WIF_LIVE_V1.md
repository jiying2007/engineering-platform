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

Before any thread or model turn is created, the adapter calls the stable
`account/rateLimits/read` RPC. The selected native Codex binary is first
compatibility-qualified against the repository contract and then rebound by exact
version and binary digest. A successful authentication read proves that the
host-owned WIF assertion has been exchanged without starting model/tool
execution. The host then unlinks the assertion and requires the path to be absent
before `thread/start`. The live receipt records
`assertion_removed_before_turn=true`; missing that fence is invalid. A later
refresh that requires the upstream assertion therefore fails closed instead of
making the assertion readable to a model-reachable local tool.

This follows Codex credential precedence rather than relying on fallback: the
presence of either required WIF variable selects WIF, and incomplete WIF must be
an error rather than silently trying another credential.

## GitHub Actions OIDC lane

`.github/workflows/codex-wif-live.yml` is manual-only and grants only:

- `contents: read`
- `id-token: write`

The live job requires:

- invocation from protected `refs/heads/main`;
- repository `jiying2007/engineering-platform`;
- repository variable `OPENAI_WIF_AUDIENCE`;
- repository variable `OPENAI_CODEX_FEDERATION_RULE_ID`.

Missing repository variables fail before checkout, OIDC minting, federation
exchange or model execution and name the missing variable explicitly.

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

`.github/workflows/codex-wif-live.yml` installs the requested bounded semantic
version of Codex (default currently `0.157.1`), compatibility-qualifies that
exact native binary without a model turn, and passes its exact binary digest to
`cmd/codex-wif-live`. The live adapter launches through `NewPinnedProvider`
and rechecks the exact executable bytes/version before starting app-server. Team
members do not need one globally fixed Codex version; compatibility qualification
and retained provenance are the admission authority.

The thread remains:

- fresh process;
- stdio transport;
- ephemeral;
- `sandbox=read-only`;
- `approvalPolicy=never`.

The prompt is fixed:

`Reply with only: engineering-platform live qualification`

The fixed credential-safe Codex HOME also disables the stable `shell_tool` and
`view_image` features. This is defense in depth: assertion removal before the
turn is the credential boundary; feature flags are not treated as a filesystem
sandbox.

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
model output + digest plus the assertion-removal fence. It contains neither the
identity token nor its filesystem path.

## External prerequisite

Codex workload identity federation is currently beta for managed ChatGPT
workspaces and must be enabled/configured by an administrator. Source merge alone
cannot create the federation provider/rule, principal membership or repository
variables.

Real probe `36556354798` on protected main
`644127255f615abb753fab5c450ea23caad76b11` failed in the first prerequisite
step because both repository variables were absent:

- `OPENAI_WIF_AUDIENCE`;
- `OPENAI_CODEX_FEDERATION_RULE_ID`.

The failure occurred before checkout, OIDC assertion minting, federation exchange
or model execution. It therefore consumed no managed-WIF model turn.

Issue #103 is the single external qualification gate. The administrator should
use `examples/pilots/wif/configure-admin-api.sh` to create/reuse the exact
provider/rule and optionally write the two repository variables.

Until that setup is complete and one `Codex WIF live qualification` workflow
succeeds and retains its receipt, the repository must continue to state:

**WIF-ready, managed-workspace WIF not yet qualified.**

## After the first successful live receipt

A qualification turn proves the managed-workspace **authentication lane**, not a
new product Feature/Debug result and not production service readiness.

Trusted self-hosted M1 phase 1 is already proven by the retained Feature and
Debug Closure chains archived in
`docs/status/M1_RETAINED_PHASE1_CLOSURE_2026-09-29.md`. Do not rerun those
pilots merely to close the WIF gate.

After the first successful WIF live receipt:

1. close issue #103 with the exact run/artifact/receipt identity;
2. mark managed-workspace WIF authentication as qualified while preserving the
   separate trusted self-hosted evidence;
3. retain the existing fail-closed WIF retained-engineering path for workloads
   that specifically need GitHub-hosted unattended execution;
4. treat long-running assertion refresh/credential-broker design, deployment
   ownership, operational SLOs, rollout/rollback and production service evidence
   as productionization work rather than M1 requalification;
5. do not broaden repository/ref/workflow/audience/immutable-ID constraints to
   make an unattended run start.

References:
- OpenAI Workload identity federation guide
- OpenAI Codex workload identity guide
- OpenAI GitHub Actions WIF guide
- OpenAI Codex federation rule reference
