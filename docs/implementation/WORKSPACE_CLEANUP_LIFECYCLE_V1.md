# Workspace cleanup lifecycle v1

This is the existing host-side preparation cleanup, not an artifact GC service,
new queue, second authority, or external-effect Recovery disposition.

## Ownership and cancellation

`Manager.Cleanup(ctx, workspace)` uses the exact existing random ownership
record and anchored root identity. It reads at most 64 directory entries per
batch, checks cancellation before each traversal/removal step, and stops at
100,000 entries, 128 nested directories, or the five-second cooperative budget.
Symlinks are unlinked rather than followed; special files are not opened for
reading. Directory identity is rechecked before entering/removing it.

A managed slot's bounded regular owner.json is removed last. Cancellation or
budget exhaustion leaves it available for exact-owner inspection. If the final
empty-directory removal fails, a synchronous exclusive-create attempt restores
the original owner bytes without overwriting a concurrent file. Both removal and
restoration failures remain in ErrCleanupIncomplete. Partial deletion is never
reported as completion. No daemon, asynchronous delete or background retry is
created. An explicit later cleanup may continue only with the original owner.

Create's exclusively allocated pre-Runtime slot uses the same traversal before
owner.json exists. A failed incomplete-create cleanup may leave an unqualified
partial slot. It is not admitted/reused as a prepared workspace and requires
explicit operator inspection. No unrelated source or evidence is removed.

`CleanupAfterFailure` derives a separately bounded lifecycle with WithoutCancel:
caller cancellation cannot erase mandatory cleanup, but request values remain.
Preparation errors and cleanup errors are joined. Worker lease/recheck failures
no longer discard cleanup errors. After a report acknowledgement is uncertain,
prepared.json and source remain retained; there is no cleanup or local replay.

## Limits that tests do not erase

The implementation cooperatively stops *between* filesystem calls. A kernel call
stuck in storage I/O is not interrupted by Go Context. Neither a five-second
constant nor a fixture proves a selected-host worst-case latency or storage
health. No goroutine is abandoned to manufacture a fast return. Existing process
supervision and operator recovery remain applicable to pathological storage.
This implementation closes ignored errors/unbounded recursive traversal, not
P0 #228 provider quiescence or all P1 #229 production qualification.

## Required regression

Cancel during multi-batch anchored traversal; require unchanged owner bytes and
an untouched symlink target outside the slot. Require exact-owner subsequent
cleanup, pre-cancelled denial, entry/depth budget rejection, symlink-owner denial,
exclusive restoration refusal and cancelled-parent independent cleanup. In an
actual preparation flow, inject a lease failure plus invalid ownership and require
both error causes, no success receipt and no extra claim/report/model execution.
The full native CI still exercises container, systemd, ENOSPC and PostgreSQL
recovery separately; none is a live production-provider acceptance claim.
