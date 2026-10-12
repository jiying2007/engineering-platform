# Failure report and publication readback v1

This patch extends the existing Worker and GitHub Publisher contracts without
changing the Action state machine or any external-operation schema. It neither
implements effectful terminal Recovery nor grants retry authority. Internal
#228 and #229 remain open. Preparation/workspace cleanup remains owned by the
existing maintenance implementation; no alternative cleanup service is added.

## An open-PR absence observation requires an array

`githubRemote.findOpenPull` must reject HTTP 200 with a root `null`. Only an
explicit JSON array, including `[]`, is a valid open-PR list response. The
historical-PR query already had this rule; open-PR readback now agrees.

Previously, unmarshalling `null` produced a nil slice and `len(pulls)==0` was
interpreted as no open PR. If branch/base checks matched and later history
returned an empty list, the existing Publish path could issue a create-PR POST.
A local HTTP-server regression exercises that actual decision path: unusable
open-PR readback now produces no later history request and no create/update
request. Observe returns an error rather than inventing a partial or absent
observation. Local Git stage validation is a test fixture, not a real Git push.

An explicit empty array still proves only the response to that particular read.
It does not prove the original remote dispatcher has stopped or that an earlier
publication cannot complete later. Do not infer SAFE_TO_RETRY or Recovery
completion from this change.

## Joined, one-attempt Worker failure reporting

The existing Codex and offline deferred failure reports now preserve report
failure alongside the primary execution error. `settleExecutionFailure` accepts
only the original `/api/v1/worker/codex/fail` and `/api/v1/worker/offline/fail`
endpoints, uses the exact original token, and performs one POST. Its independent
six-second Context uses `WithoutCancel` to preserve original request values
without inheriting caller cancellation. The call is joined and its timer is
released before return; no background retry worker is introduced.

Only the existing response `recorded=true` acknowledges that report. An empty,
false, null, lost or malformed acknowledgement adds
`ErrFailureReportUnobserved`. A precise underlying `ErrCommitOutcomeUnknown` or
`ErrPostOutcomeUnobserved` remains discoverable with `errors.Is`, alongside the
primary error. The wrapper's public message contains no private response body.

Failure acknowledgement is not successful execution, a rollback guarantee or
permission to rerun a model/build/publication. Operators and clients must read
back the original execution/token/epoch/audit facts before another decision.

## Compile-time and source guard

The existing caller-bound production Core interface is unchanged by this delta.
A non-test `cmd`/`internal` source-selector guard additionally rejects direct
calls and method-value references to legacy context-free Core convenience
methods, rather than guarding just a few adapter files. This is a syntactic
regression guard, not a complete alias/interprocedural analysis or proof of
bounded filesystem/kernel completion.

## Qualification boundaries

Regression tests use the real Go packages, local HTTP servers and bounded test
transports. They prove the specified response/error handling and one-attempt
behavior. They do not perform a live GitHub publication, contact a model, replace
canonical PostgreSQL/container/systemd gates, or qualify selected production
host/provider shutdown and recovery. Exact PR-head CI, protected merge,
fresh-main checks and actual CI-delivered-byte readback remain required.
