# Independent GitHub PR Publication V1

## Purpose

This slice publishes exactly one retained Core-bound Codex result without giving
GitHub authority to the Worker or model process.

The trusted flow is:

```
FINISHED Codex receipt
  + retained Git bundle
  + frozen Task repository/base
        |
        v
Action Gateway authorization
        |
        v
independent GitHub publisher
        |
        +-- re-hash retained bundle
        +-- verify target repository/base-ref policy
        +-- fetch exact frozen base
        +-- git bundle verify
        +-- prove result descends from frozen base
        +-- push deterministic non-force branch
        +-- create/update one PR
        +-- retain publication receipt in external-operation ledger
```

This is publication transport only. It does not create CI Evidence, Verification,
Review, Delivery or Closure authority.

## Authority split

The Worker remains unable to push or create pull requests. Its Core-bound Codex
lane still has no GitHub credential and produces only a local result commit plus
verified Git bundle.

The publisher is enabled only in authenticated production assembly when
`GITHUB_PUBLISHER_CONFIG_FILE` is present. The configured credential is read
from a separate owner-private token file by the publisher process. The
unauthenticated development server rejects publication configuration.

For a publication action, all three independent gates must agree:

1. the frozen TaskContract contains `github.publish-pr` in
   `allowed_actions`;
2. the requesting mTLS principal has the exact action grant
   `github.publish-pr / CONTROLLED_MUTATION / github.publish-pr`;
3. the operator-owned publisher policy contains the exact Task repository and
   allowed base ref.

The action request `parameters_digest` must equal the immutable
`WORKER_ATTESTED_CODEX_EXECUTION.result_digest`. The provider re-reads the
FINISHED Codex receipt from Core rather than trusting caller-supplied commit,
branch, repository or bundle locators.

## Operator configuration

Example:

```json
{
  "version": 1,
  "artifact_root": "/var/lib/engineering-platform/shared-codex-artifacts",
  "git_executable": "/usr/bin/git",
  "token_file": "/run/secrets/engineering-platform-github-token",
  "targets": [
    {
      "repository": "jiying2007/engineering-platform",
      "base_ref": "main",
      "branch_prefix": "engineering-platform/"
    }
  ]
}
```

The artifact root is the read-only shared view of the Worker's retained
`artifacts/` directory. The result bundle name is not caller-controlled; it is
derived as `<codex-execution-id>.bundle`.

The config, artifact root, Git executable and token file must be canonical
host-controlled paths. The token file must be owner-private. Prefer a
short-lived GitHub App installation token with only the target repository
permissions needed to read refs, write contents/branches and create/update pull
requests. Token bytes are never placed in Git URLs or command arguments.

## Publication algorithm

The provider derives a publication plan entirely from retained authorities:

- Run and current execution epoch;
- frozen TaskContract repository/base commit;
- FINISHED Core-bound Codex receipt;
- operator target policy;
- deterministic branch
  `<branch-prefix><first-24-hex-of-codex-execution-id>`.

Before any GitHub mutation it:

1. re-hashes the retained bundle and matches receipt digest/size;
2. reads the configured base ref and requires it to equal the frozen Task base;
3. fetches that exact base into an isolated temporary bare repository;
4. runs `git bundle verify`;
5. imports bundle `HEAD` and requires the exact retained result commit;
6. proves frozen base is an ancestor of result and runs strict object checks.

The branch is never force-pushed. If it already exists it must already point at
the exact result commit. The publisher then creates one open PR or updates the
title/body of the unique existing open PR for the exact branch/base pair.

## Durable receipt and ambiguity

The existing external-operation ledger remains the authority:

```
PLANNED -> DISPATCHED -> CONFIRMED
                     \-> UNKNOWN -> RECONCILING
                                     -> CONFIRMED
                                     -> SAFE_TO_RETRY
                                     -> MANUAL
```

A confirmed operation retains the PR URL as `external_ref` and a structured
GitHub publication receipt in `observed_state` containing repository, base
ref/commit, publication branch/result commit, PR number/URL/state and whether
the PR was created, updated or already exact.

A deterministic local precondition failure is retained as
`PRECONDITION_FAILED` and reconciles to MANUAL. A transport error after
dispatch becomes UNKNOWN. Reconciliation is observation-only:

- exact base + no branch => SAFE_TO_RETRY;
- exact branch + exact open PR => CONFIRMED;
- partial branch-only state or any conflicting ref/PR => MANUAL.

Reconciliation never replays a push or PR mutation.

## Remaining external gates

This implementation does not satisfy the two retained pilot gates by itself.
Before reassessing M1:

1. the ChatGPT workspace administrator must configure the real Codex WIF
   provider/rule and one real model execution must be retained;
2. one Feature and one Debug task must each complete:
   Requirement -> Codex engineering -> result commit/bundle -> Git/PR ->
   trusted CI -> Evidence -> Verification -> independent Review -> Closure.

Repository fake app-server tests remain protocol/filesystem evidence only and
must not be represented as live model evidence.
