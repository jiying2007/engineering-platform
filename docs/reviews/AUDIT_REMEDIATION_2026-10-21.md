# Comprehensive audit remediation — 2026-10-21

This records the implementation response to the comprehensive review of
`a8557646ca27eb4d7feb09144995478a20a0664c`. It is a historical change record;
[Implementation Status](../status/IMPLEMENTATION_STATUS.md) remains the single
live readiness authority. CI receipts and exact commit/run identities belong to
the associated PR and retained CI artifacts, rather than a self-referential
future commit claim in this file.

| Finding | Implementation | Regression evidence and boundary |
| --- | --- | --- |
| F01: known Go/toolchain dependency advisories | Go 1.26.10, x/text 0.41.0, x/sync 0.22.0; pinned govulncheck 1.9.0 in the Go gate | Text-mode source scan fails on reachable advisories or scanner failure and retains its version/output. The local scan returned no vulnerabilities against the 2026-10-20 database; future advisory databases may change that result. |
| F02: certificate expiry on existing Publisher connections | Revalidate leaf identity, client-auth usage and the matching verified chain's validity on every request | Real mTLS keep-alive expiry rejects new publication with zero Publish calls. It does not retroactively cancel a previously authorized in-flight request. |
| F03: insufficient PLANNED recovery projection validation | One strict row/receipt/audit projection used by readback, repeat, facts, status and completion; retain original actor separation within the same Recovery epoch | Corruption, missing/duplicate/orphan evidence, changed bindings, later Recovery epochs and a real PostgreSQL dump/restore are exercised. No historical evidence is fabricated or repaired. |
| F04: terminal Run blocks original publication evidence | Separate historical original-plan reconstruction from current dispatch admission | Completed/Aborted and later-epoch reads require exact frozen Task, sealed Runtime, receipt, Action identity and independent Publisher plan. New dispatch and mismatched evidence remain rejected. |
| F05: repeated full-history provenance work | Migration v14 partial index; combine aggregate counts and correlated audit scans | Record actual query plans, check index validity and exercise 500/2,000/5,000-operation histories within the store budget, retaining oldest-record corruption rejection. All-history validation remains mandatory. |
| F06: subprocess output can close before final frames drain | Caller-owned output pipes across qualification, live probes and engineering execution | Rapid-exit output regressions exercise final-frame delivery and cleanup. |
| F07: concurrent stderr diagnostics race | Reuse synchronized bounded diagnostic buffering for live probes | Concurrent subprocess diagnostics are exercised under the race detector. |
| F08: filesystem read-isolation claim exceeds available proof | Qualification schema v4 records `host-uid-trust` and `filesystem_read_isolation_qualified=false` | Reissued qualifications and receipt-based Profile construction/preflight reject v3/missing or unsupported claims. Exact frozen historical v3 evidence has a strict archival-only reader; no schema rewrite or new authority is granted. Real selected-host isolation remains unqualified. |
| F09: mutable workflow action identities | Full immutable action commit SHAs throughout the workflows; a checked-in format lint rejects mutable references | The lint covers the repository's direct single-line `uses` convention and rejects unsupported forms. Identity pinning is not an action-source security audit. |

## Delivery and operational limits

The canonical five jobs remain `go`, `offline-container-integration`,
`postgres-authority-restore-drill`, `codex-app-server-qualification`, and
`trusted-ci-artifact-evidence`. The restore event-stream gate explicitly requires
the PLANNED dump/restore scenario; missing and skipped execution are rejected.
Existing source/receipt/provenance checks are retained.

The runtime compatibility contract stays at version 2 and EngineeringReceipt
schema and engineering-configuration digest bytes are unchanged. Qualification
schema v4 makes the read boundary explicit without claiming a new OS isolation
mechanism. Historical v3 qualification evidence is accepted only for a completed
result's bounded archival path when it matches the frozen expected digest;
reissued qualifications and receipt-based Profile construction/preflight require
v4. Existing frozen Profile/Permit digests do not encode a schema discriminator
and remain unchanged; the archival reader does not introduce execution authority.

Internal readiness remains **BLOCKED**: **#228 and #229** remain open, alongside
production qualification **#105** and credential-dependent **#103/#106**. This
change does not supply real provider credentials, production host evidence,
original dispatcher quiescence for arbitrary unknown effects, unattended
long-run acceptance or WorkBuddy/HIL operator qualification. Local tests and
hosted CI must not be promoted into those claims.
