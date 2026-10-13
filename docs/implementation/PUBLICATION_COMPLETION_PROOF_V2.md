# Original publication completion proof v2

## Scope

The existing successful-publication recovery path now admits only a version-2
completion containing the original per-call Git namespace-init reap proofs.
The Publisher, not the operator or later observation, supplies the proof.
There is no new schema, recovery state, queue, retry or proof-generation endpoint.
The pre-invocation non-success path is unchanged.

## Original call binding

The service creates one collector before the original synchronous Remote.Publish.
The actual runPublisherGit path appends each observed processscope.Proof; the nested
cleanup collector cannot replace it. The bounded completion permits 1–32 valid,
reaped, non-duplicated, sequential proofs inside the original start/return interval.
The list digest and full completion bytes are retained in completed.json. Missing,
uncertain, oversized or malformed proof collections withhold completion, keeping
the original slot occupied. A validated success receipt alone is insufficient.

The new collector does not retry, kill another operation, re-observe a past PID,
or manufacture proof from the current executable version. All existing Git
containment and staging quarantine behavior remains. Namespace-init reap proves
LOCAL descendant termination, not remote request cancellation. Only a successful
original call plus the existing precise positive remote observation can settle.

## New admission versus historical readback

DispatchCompletion.Validate and existing receipt digests retain structural v1
readback without rewriting bytes. ValidateForRecovery additionally requires v2
start/completion and the retained original proofs. The Publisher refuses legacy
completion BEFORE any fresh remote observation; remote client, Provider, Core
handler and the database's new-settlement path independently enforce this rule.
GET/exact repeat of an already recorded, audit-consistent historical Core receipt
remains read-only. It does not issue a new completion, change its evidence version,
or assert that the old invocation had the new process guarantee. A pending legacy
operation cannot be promoted by upgrading the current binary.

## Witness filesystem boundary

Root and slot directories require exact private, non-special mode and ownership.
Witness files require a regular 0600 file owned by the Publisher, one hard link,
consistent inode/size/mode and read-time modification timestamp. Descriptor open
uses a held directory FD, one checked leaf component, O_NOFOLLOW, O_NONBLOCK and
O_CLOEXEC. Root.OpenFile's normal in-root link resolution is deliberately NOT used
for this strict final-component policy. Symlink/FIFO substitution cannot make this
open step follow a link or block before descriptor type validation. Unsupported
platforms reject rather than using an unchecked fallback.

Successful regular-file fsync can still depend on the storage/kernel; no arbitrary
latency guarantee is inferred from O_NONBLOCK. Existing finite body limits and
request/settlement Contexts remain. This is not attestation against a privileged
administrator able to rewrite all witnesses, database and audit, or roll back a
whole live VM. No witness is automatically erased, relinked, repaired or replayed.

## Automated acceptance

Canonical CI requires all eight new Publisher top-level tests x3 and the two new
PostgreSQL tests x3 in addition to the existing publication, no-invocation, kernel
and three restore gates. Missing, failing or nested skipped targets reject. Tests
cover original actual Git scope collection, dropped/forged proof, duplicate and
budget cases, legacy admission denial with historical readback, aliases/special
modes, and new-settlement database/audit atomicity. Synthetic fixture proofs are
explicit test data, never selected-host or live Provider qualification.

Internal #228/#229 remain open for post-invocation/ambiguous/historical recovery
requirements and selected-host lifecycle, storage and long-run qualification.
