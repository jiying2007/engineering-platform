# Source-only CI lane — migration contract v1

Status: **candidate design and supplemental workflow only; original canonical CI, branch protection and production qualification remain unchanged.** Owner acceptance and branch-protection access are still required for any future required-check migration.

## Why this exists

The protected main workflow `.github/workflows/ci.yml` currently runs five exact-HEAD canonical jobs: `go`, `offline-container-integration`, `postgres-authority-restore-drill`, `codex-app-server-qualification`, and `trusted-ci-artifact-evidence`. The last job requires all other four and imports the real Codex compatibility artifact into the trusted evidence v2 envelope. The required workflow may start a real Codex CLI app-server even for source-only changes.

The repository's existing **Codex/ChatGPT Work shared-quota restriction remains in force until explicitly lifted**, regardless of a change in calendar month. Therefore do not unskip an existing PR, edit it to cause a canonical re-run, change refs to a no-skip commit, dispatch the full CI, or merge with pending required checks under the current restriction.

## Deliverable now: additional no-Codex source proof

`.github/workflows/source-only-qualification.yml` is a read-only, bounded, provider-free Go verification lane. It:

- uses the exact GitHub event head SHA, records its original source tree and PR base, and checks out without persistent credentials;
- reuses pinned checkout/setup-go/upload-artifact actions and the repository-defined Go toolchain;
- uses an ephemeral PostgreSQL 17 service, canonical test-only AppArmor user-namespace preparation, `gofmt`, `go mod tidy` hygiene, full-repository `go test -race`, and `go vet`;
- refuses to run when a real `codex` executable is present in `PATH`, starts no model turn, imports no secret, performs no publishing, and retains source-bound results for 30 days;
- never emits the `trusted-ci-evidence-<sha>` artifact name or a `codex-compatibility-qualification-<sha>` receipt.

The workflow is **additional** evidence and its job `source-only-go` is **not a substitute** for any of the five required checks. It does not add `go` binary packaging, container integration, restore-drill assurance, vulnerability scan, immutable trusted CI importer, or real runtime qualification. The current `TRUSTED_CI_EVIDENCE_V2` document and all five canonical job names stay authoritative.

For already-held PRs, an independent exact-HEAD supplemental run [#41210114240](https://github.com/jiying2007/engineering-platform/actions/runs/41210114240) succeeded for Control #300 and Relay #301 on Linux. Both passed targeted and full `-race`, module hygiene, and `vet`; the Control package additionally cross-compiled for Windows. The Relay `cmd/eng` Windows package probe was **not qualified** because its preexisting `internal/retention` dependency has no Windows Go sources. That portability probe was optional and must never be promoted to PASS based on overall workflow success. Neither result proves Relay endpoint credentials, upstream identity, or production use.

## Required separation before accepting source-only PRs as a merge gate

1. **Freeze and review changed authority graph.** Define explicit risk and applicable-check rules in a separately reviewed policy. Changes to Provider, Codex/app-server protocol, credential identity, live runtime, external side effects, security/CI protection, DB schema or production release require appropriate runtime/integration qualifications. Broad filename allowlists or author-selected R1 labels are insufficient.
2. **Split evidence schemas by purpose, not success flag.** `source-only` receipts must never deserialize as `Trusted CI evidence v2`, qualify a `Profile v3`, or satisfy Delivery/Evidence/Verification/Review/Closure. Introduce separately versioned source and runtime attestation contracts, with immutable SHA/tree, workflow blob, job identities, run/attempt, retained byte digests and independent readback.
3. **Keep runtime compatibility real.** Run the exact Codex version/model/sandbox app-server qualification on an explicitly admitted runtime-lane event and selected deployment identity; never substitute a mock, fake CLI receipt, cached unrelated SHA or skipped job. A valid runtime qualification is not proof of a real provider credential or live model response.
4. **Migrate only after independent review.** Ratify applicable rules and release gates; qualify the new workflow itself at exact source and then protected fresh-main; inspect current branch-protection / ruleset requirements and change them through an administrator's normal review path. This GitHub App cannot read admin-only branch protection, so do not assert the current required-check list can be rewritten here.
5. **No silent downgrade during rollout.** Until migration completes, all five existing checks remain required on PR head and protected main. A source-only green badge cannot merge #300 or #301 under the existing rule. Keep `[skip ci]` holds while the restriction persists, and never force-merge or forge status contexts.
6. **Preserve real blockers.** #228 effectful Action recovery, #229 host Context/SLO, #106 relay contract/Provider identity and #105 unattended/production evidence remain independently OPEN. A reviewed software change cannot close them.

## Acceptance and rollback

- Expected immediate acceptance: exact-source Go test/review logs, artifact identity and no Codex/Work consumption; no main or existing workflow changes.
- Future policy migration acceptance: explicit affected-component classification, all relevant test and evidence schemas, successful canonical replacement checks on exact PR heads and fresh main, independent release/admin approval, and no weakening of production readiness.
- Rollback: disable or remove the **optional** source-only workflow by a normal PR; original five-job canonical CI and trusted v2 importer remain untouched. Do not delete historical CI artifacts or rewrite past receipts.

This is a staged source-quality proposal, not approval of changed production controls or an authorization to spend the user's shared model quota.
