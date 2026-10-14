# Original confirmation and Recovery authority

Base: protected `d460fcc58c4760a64287e3cf79ef550723a45eb2` after the bounded
PostgreSQL entry change. The uploaded unfinished recovery-boundary snapshot is
not accepted as-is: its audit-count heuristic is removed, retained tests are
integrated, and the independent positive Recovery path is explicitly preserved.

## Three different facts

1. Original Provider return: the normal ledger can settle DISPATCHED to CONFIRMED
   once. Original identity and pre-existing independent settlement budget apply;
   a later Recovery epoch does not relabel or discard that original result.
2. Ordinary observation: the result type contains no original completion,
   dispatcher/reap witness, independently authenticated reconciler or Recovery
   proof. For potentially mutating Actions a valid current PR is diagnostic
   MANUAL, not CONFIRMED. A generic Provider positive verdict is rejected as
   unresolved. OBSERVE-only Actions keep their read-only reconciliation edge.
3. Recovery: only the existing authorized route obtains full evidence from the
   configured independent Publisher and performs the original serializable
   record/receipt/audit transaction. Negative, missing and partial observations
   do not acquire authority merely because an operator wants closure.

Both Memory repositories now use the actual Operation transition function under
same-reservation and original lock checks. They reject terminal reopening,
recovered-state injection and proofless effect confirmation instead of accepting
any state field replacement. PostgreSQL uses the same restricted predecessor
set inside its existing atomic identity-bound UPDATE.

## Original-success provenance without a schema change

A new direct successful mutation settlement writes the existing
`action.state_changed` event with causal marker
`action.original-dispatch-confirmation.v1`. This path can atomically update only
an original DISPATCHED row. Before hashing/writing, its result timestamp is UTC
and truncated to the microseconds stored by PostgreSQL. Event type, original
Operation identity, exact payload, cause and reconstructed event digest must
match the current row. The read-side query fetches at most two matching markers
per row (zero or duplicate rejects), streams rows under the existing caller's
budget, and never alters the journal. It is not an external completion witness,
a remote rollback certificate or protection from a privileged administrator
rewriting all retained facts.

An audit count of one planned and two changed events is insufficient. No
historical result is rehashed with a guessed timestamp or assigned a new marker.
An old ordinary confirmation without this provenance stays read-only and blocks
Recovery. This is intentionally conservative; it does not assert that every old
confirmation was incorrect. No historical migration or automatic remediation is
provided. Exact full-evidence enrichment below is the only new acceptance path.

Recovery facts and final completion run this validator inside their existing
serializable transaction. Operational status runs the same validator and counts
in one read-only repeatable snapshot under its unchanged five-second budget;
its deferred bounded rollback only releases read locks. No mutating COMMIT or
second authority is introduced.

## Historical positive enrichment

The existing positive Recovery route also accepts a bare CONFIRMED publication,
without allowing this state into the separate no-invocation/abandonment route.
It requires the same original identity, frozen plan, exact original PR and full
original completion/reap evidence plus fresh positive observation. Existing
observed receipt/ref must match that subject. In the existing row transaction it
adds a full Recovery receipt and one audit event, preserving all original audit
records, original epochs and the occupied idempotency key. It does not rerun the
publication or silently assert a new success from the historic state label.

Before enrichment, the row and original recovery audit are read together. A
recovery event whose receipt is missing or damaged is rejected, even if the
caller supplies an otherwise valid witness; it cannot be recreated as a second
success. Exact repeats read the original result; conflicting epoch/identity or
principal rejects. The operation's reconciler cannot complete the same Recovery
epoch via somebody else's aggregate proof. Historical recorded evidence remains
subject to the existing format/reap requirements, not implicit promotion.

## Evidence and remaining boundaries

Required regression includes all Memory state pairs by risk; ordinary positive
verdict and cancellation controls; original post-Provider success after Recovery
Begin; full-evidence positive recovery and historical enrichment; missing,
retargeted, stale and partial evidence; corrupted/missing/duplicate audit marker,
payload/time/row damage and an old-count lookalike; exact repeat and independent
completion. All six new database tests are added to the existing required
PostgreSQL17 repeated log gate. Previous lock/COMMIT-ACK-loss, mTLS, dump/restore,
service-stop, caller budgets and original reap/no-invocation tests remain.

A deliberate new provenance requirement can reveal old bare confirmations that
need real retained evidence. Do not close internal blockers [228,229] or promote
readiness from these fixtures. Unknown post-entry effects without original
completion evidence, selected-provider/host qualification, storage/latency/soak
and any required independent human review remain separate obligations.
