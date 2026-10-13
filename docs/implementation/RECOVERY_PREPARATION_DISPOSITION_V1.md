# Recovery preparation disposition and scoped Publisher hardening

Baseline: protected main `c449d7c3f1bfaab3f9f07b61d1992d7248c6d285`
(tree `7e1a5a09033eaaaf6489d3377275a633a1370487`). This is a bounded
source/qualification slice, not closure of internal #228/#229 or production #105.

## Earlier candidate: reviewed, not silently merged or deleted

The old preparation ref `feat/completed-publication-recovery-20261012` has exact
head `f9860c2271e2119279daf5a31549ff9ca7fedba7`. Its native run `38252424287`
retained artifact `11749272822`, ZIP SHA-256
`995ecbf729405f552f17e89a39bb7736cb3fd57204f1ffd9466e7f4e76ea67f2`.
The archive's 38 source objects were checked against individual SHA-256 and Git
blob identities. Its materialized target tree is
`57280fa5aee0710eb28b013f0b6ebed3ce678592`, NOT the #268 main tree.

| Earlier component | Disposition |
| --- | --- |
| Git PID-namespace launch and fast/setsid-child tests | Absorbed into the existing Git transport, with caller cancellation and staging quarantine checks |
| Separate completed-publication table, schema v14 and alternate endpoints | Not adopted; #268 continues to settle the original external_operations ledger |
| Alternative Invocation/CompletedDispatch and journal seal formats | Not adopted; no second publisher protocol or runtime compatibility adapter |
| Host journal-ownership check intent | Adapted to the actual ServiceConfiguration v2 / witness_root contract, not copied with old field names |
| Native logs and remaining alternate source | Retained as a superseded prototype under the existing exact-ref guard, not shipping evidence |

The formerly empty `feat/publication-recovery-witness-20261012` ref is reused for
this real PR instead of adding another scratch ref. Its final retirement must
use the existing protected-merge/fresh-main workflow. Neither registration of an
older ref nor this document asserts deletion before the remote readback.

## Git return must mean the local process tree is quiescent

Every production `githubRemote.gitOutput` now enters the existing processscope
launcher. A fixed stdin gate keeps the namespace init alive until its dedicated
PID namespace is recorded, including for very fast commands. The original caller
Context is preserved. After cancellation, one separately bounded stop/reap
lifecycle is used; there is no uncontained plain-exec fallback.

A successful Git command requires an observed reaped namespace init. Local Git
helpers that setsid/double-fork or close stdio cannot outlive that namespace.
The previous plain Cmd.Run implementation is retained only as a local failing
regression overlay, not another execution mode. No credentials or caller strings
are interpolated into shell syntax; Git's sealed environment and no-redirect
policy remain unchanged.

If termination cannot be confirmed, the original verified bundle and Git stage
are quarantined, the original journal slot stays occupied, and no completion
witness is created. Controlled tests inject this exceptional result; real kernel
tests cover fast commands, detached children and cancellation. A missing kernel
feature is a canonical CI failure, never an accepted fallback or passing skip.

This is LOCAL process evidence. It does not cancel a GitHub server request already
accepted remotely, repair a historical missing completion, or authorize any replay
from negative/partial observation. No new terminal Recovery state is introduced.
Existing historical witness bytes do not retrospectively acquire kernel proof.

## Production host admission uses the deployed schema

The former host reader used the old in-process Configuration v1 while
LoadRemoteService required ServiceConfiguration v2 with witness_root. A valid v2
file therefore failed host preflight. Host admission now consumes that same v2
schema, checks exact plan targets, canonical disjoint directories, Publisher
ownership of a private 0700 non-special witness root, and actual path traversal
and write permissions. Version1, aliases, wrong owners, overlapping roots and
policy drift remain rejected. No provider/token qualification follows from a
successful local config check.

## Direct Context inventory is executable governance

The current 132 direct constructor sites have an exact source digest, ordered
path/line/constructor list and six category counts. Existing CI now checks all of
these against source. Missing, duplicated, stale, misclassified or count-drifted
rows fail closed. This is still a direct-source inventory, not alias-sensitive
interprocedural analysis or a selected-host capacity/latency claim.

## Required acceptance

Use exact-head canonical CI and fresh-main checks, actual delivered ZIP and binary
readback, normal own-ref retirement, and unchanged registered evidence refs. Keep
#228 open for other orphan-effect evidence/disposition cases and #229 open for its
remaining lifecycle and selected-host qualification. No source cleanup may replace
missing evidence with a success claim.
