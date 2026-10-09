# Independent GitHub PR Publication V1

> **Historical implementation slice.** The authoritative current production
> topology is the [independent Publisher service](PRODUCTION_OPERATIONS_V1.md)
> and the [production remote boundary](#production-remote-publisher-boundary)
> below. The trusted M1 Feature/Debug pilots were closed after this slice;
> do not interpret the original prerequisites as currently open M1 work.

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

In the current production topology, the separately deployed
`publisher-service` reads its configuration from `PUBLISHER_CONFIG_FILE` and
its owner-private GitHub token file from the referenced publisher configuration.
`GITHUB_PUBLISHER_CONFIG_FILE` is the older **Control Plane** in-process
publisher input, restricted to explicit pilot deployment mode. Production
Control Plane instead receives `GITHUB_PUBLISHER_PLAN_FILE` and
`GITHUB_PUBLISHER_REMOTE_FILE` (non-secret plan and mTLS remote client);
it must never receive the local publisher configuration or token. There is
no in-process production fallback.

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
GitHub REST calls carrying the publisher token and Git's credentialed HTTPS
fetch/push both **refuse HTTP redirects**, including same-origin redirects.
The Publisher Git subprocess already uses an explicitly bounded environment;
the REST client now uses a matching **direct-only** HTTP Transport and ignores
ambient `HTTP_PROXY`, `HTTPS_PROXY` and related host proxy settings. There is
no implicit or automatic proxy failover for credential-bearing publication.
If a team requires an egress proxy, it must be separately specified, reviewed
and qualified rather than silently inherited from systemd or the shell.
A repository rename, proxy-injected redirect or unexpected API migration must
fail closed and be reviewed as an operator policy change; it must not silently
redirect a push, PR mutation or bearer credential to another endpoint. The
publisher does not automatically retry an ambiguous side effect.

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

- exact base + no branch + no historical PR for the deterministic head/base + exact local bundle => SAFE_TO_RETRY;
- exact branch + exact open PR => CONFIRMED;
- missing branch with any closed/merged/prior PR history, partial branch-only state, or conflicting ref/PR => MANUAL.

Reconciliation never replays a push or PR mutation. It derives the immutable
publication Plan from the retained Core-bound Run/Task/Codex receipts, so an
already **CONFIRMED** remote branch and exact PR can be recognized even if the
local Git bundle was lost after dispatch. A remote **ABSENT** observation is permitted only when both the exact
publication branch and its full GitHub PR history are absent. It permits
`SAFE_TO_RETRY` only if the local bundle still passes exact byte verification;
without those bytes the result remains `MANUAL`. GitHub's `state=all` query
retains closed/merged PRs even after their head ref is deleted: those are
prior external effects, not proof that nothing occurred. New Publish also
refuses to push or recreate a PR on the same deterministic branch if such a
prior PR exists, even when no PR is currently open. Upstream query failure
never authorizes a retry. Neither reconciliation observation invokes
publication again.

## Historical pilot closeout and current external gates

This publication slice alone did not prove end-to-end M1 at the time it was
introduced. Trusted self-hosted Feature #54 and Debug #55 have since completed
Requirement -> Codex -> Git/PR -> trusted CI -> Evidence -> Verification ->
independent Review -> Closure; see
[M1 closure evidence](../status/M1_RETAINED_PHASE1_CLOSURE_2026-09-29.md).
Neither is pending repetition. Managed-workspace WIF is an optional separate
unattended credential qualification (#103), not a prerequisite for the already
completed trusted self-hosted M1 proof. Optional relay (#106) and production
acceptance (#105) are also separate external gates. Repository fake app-server
tests remain protocol/filesystem evidence, not live model or production proof.


## Production remote publisher boundary

Production-v1 does not place the GitHub token in Control Plane.

Control Plane retains the existing Action Gateway responsibilities:

- frozen Task/Run/epoch/recovery authorization;
- exact Codex result and result-digest binding;
- deterministic publication Plan derivation;
- retained bundle raw-byte digest/size verification;
- Action operation/UNKNOWN/reconciliation state.

The credentialed external mutation is delegated through an mTLS
`githubpublish.Remote` client to the independent `publisher-service`.

The Publisher service:

- runs under a separate Unix identity;
- is the only production service that can read the GitHub publisher token;
- accepts only TLS 1.3 clients signed by its configured CA;
- rechecks the exact configured Control Plane URI subject;
- rechecks repository/base-ref/branch-prefix target policy;
- reads the retained shared bundle through a scoped path and copies it to
  an owner-private, short-lived snapshot while hashing the **copied bytes**;
- compares snapshot byte size and SHA-256 to the frozen Plan before any Git
  operation; Git consumes the verified private copy rather than re-opening
  the original Worker-owned path; cleanup follows the publication attempt,
  and storage/copy errors fail closed without performing a GitHub mutation;
- performs Git/GitHub mutation through the existing GitHub Remote implementation;
- never receives DATABASE_URL, model/provider credentials, Review or Closure
  authority.

The mTLS RPC is not a second Action Gateway. A transport failure remains an
UNKNOWN result at the existing Action operation and is reconciled through
`Remote.Observe`; the client never automatically retries a publication POST.
