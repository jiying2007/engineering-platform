# Action recovery state matrix v1

Scope: the existing Action ledger and Recovery epoch after #262 at
`bcdf350c2916e5194aa4930979bc50af7904fa86`. This document freezes acceptance
requirements; it does not implement a new effectful terminal transaction or
qualify a live provider. Internal P0 #228 remains OPEN.

| Durable state | Established fact | Current permitted path | What is still required |
| --- | --- | --- | --- |
| PLANNED | The reservation precedes durable DISPATCHED | Existing authorized schema-v13 planned-only abandonment | Exact original ledger/audit/epoch readback; no replay or success receipt |
| DISPATCHED | A provider effect may be in flight or already complete | Preserve the original reservation and block Recovery Proof | Original-dispatcher quiescence plus independently retained provider-specific observation |
| UNKNOWN | The outcome is unresolved | Existing observation-only Reconcile admission | An absent-at-one-instant result is not proof of quiescence or safe retry |
| RECONCILING | An observation was admitted but may lack durable settlement | Preserve unresolved state; no new publish/model call | Exact original observation/process/epoch readback; no generic reset to UNKNOWN |
| MANUAL | Existing observation could not settle the operation | Preserve unresolved state | Evidence-backed, separately authorized terminal disposition, not direct SQL or a generic success override |
| SAFE_TO_RETRY | Historical verdict lacks independent dispatcher proof | No redispatch; Recovery remains blocked | Never reinterpret an old verdict as current replay authority |
| CONFIRMED | The ledger retained a positive settlement | Existing immutable result/subject readback | Do not infer that a current remote ref remains unchanged from this historical fact alone |
| ABANDONED_RECONCILED | Existing planned-only recovery recorded a non-success no-replay disposition | Existing receipt/audit/Recovery proof | Never manufacture Delivery/PR confirmation or free the idempotency key |

## Required effectful disposition contract (not yet implemented)

Reuse the existing ledger and Recovery transaction. Bind exact operation ID,
original Run and execution epoch, current Recovery epoch, frozen publication
plan/subject, original dispatcher identity and stopped/quiescent observation,
independent provider readback bytes/digest, and the authorized reconciler.
An observation reference without retained bytes is not sufficient. Keep the
existing distinct recovery:reconcile and recovery:complete identities.

Positive, absent, partial, retargeted, unavailable and still-in-flight provider
observations are different cases. No bounded number of absence queries proves
that a previously admitted remote operation can no longer complete. A generic
recovery override cannot invent the missing provider/process evidence.

## Acceptance matrix

Inject death before durable DISPATCHED, after DISPATCHED before the provider,
after the remote effect before settlement, and after RECONCILING admission.
Combine these with Recovery Begin, Run pause/takeover, stale epochs, live or
unknown dispatchers, missing bundles, closed PR/deleted refs, lost COMMIT ACK,
duplicate/conflicting dispositions and unauthorized principals. Require exact
PostgreSQL+mTLS/audit/restore readback. Keep ambiguous cases blocked.

Unit transition-matrix tests cover all state pairs and preserve existing
no-retry/no-generic-abandonment rules. They are regression coverage, not proof
that #228 is closed or that a selected host/provider has quiesced.

## Original reservation identity

Every normal ledger update binds the exact stored operation ID, Run, execution
and original Recovery epochs, Action/risk/capability, idempotency key and request
digest. Even a different currently authorized Run cannot admit the original
reservation into DISPATCHED. Mismatched settlement leaves the original row/key
and audit unchanged. Original post-Provider settlement remains allowed after a
new Recovery epoch begins; it must carry the old immutable identity, never be
rebased onto current authority. This closes an identity-substitution gap, not
original-dispatcher quiescence or effectful terminal Recovery.

## Additional pre-effect evidence boundary (2026-10-13)

The existing PLANNED-only endpoint remains unchanged. A separate explicit
`abandon-uninvoked` path now accepts an unresolved publication only when the
independent Publisher seals an original STARTED-v2 before its durable invocation
barrier, under the same inode lock, boot and directory identity. It stores a typed
non-success receipt and audit in the existing row; no normal transition, idempotency
release, CONFIRMED receipt or remote-absence retry is added. Old/missing/copied/
restored/armed witnesses cannot use it. See PUBLICATION_NO_INVOCATION_RECOVERY_V1.
The earlier table's general potentially-effectful cases remain blocked without
this concrete pre-invocation proof or the existing positive completion witness.
