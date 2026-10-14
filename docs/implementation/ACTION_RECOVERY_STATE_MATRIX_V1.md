# Action recovery state matrix v1

Scope: the existing Action ledger and Recovery epoch, original reservation
identity, witnessed positive publication and pre-invocation evidence v1/v2.
This is the current acceptance map, not a claim of live-provider qualification.
Internal blocker 228 remains OPEN for cases lacking the required original proof.

| Durable state | Established fact | Current permitted path | What is still required |
| --- | --- | --- | --- |
| PLANNED | The reservation precedes durable DISPATCHED | Existing authorized schema-v13 planned-only abandonment | Exact original ledger/audit/epoch readback; no replay or success receipt |
| DISPATCHED | A provider effect may be in flight or already complete | Exact positive completion or non-invocation recovery; otherwise block | Original proof and bound receipt/audit; remote absence alone is insufficient |
| UNKNOWN | The outcome is unresolved | Existing observation-only Reconcile or exact recovery evidence | An absent-at-one-instant result is not proof of quiescence or safe retry |
| RECONCILING | An observation was admitted but may lack durable settlement | Exact positive completion or non-invocation recovery; otherwise unresolved | Exact original observation/process/epoch readback; no generic reset to UNKNOWN |
| MANUAL | Existing observation could not settle the operation | Exact positive completion or non-invocation recovery; otherwise unresolved | Evidence-backed, separately authorized disposition, not direct SQL or a success override |
| SAFE_TO_RETRY | Historical verdict lacks independent dispatcher proof | No redispatch; only exact new recovery evidence may settle | Never reinterpret an old verdict as current replay authority |
| CONFIRMED | The ledger retained a positive settlement | Existing immutable result/subject readback | Do not infer that a current remote ref remains unchanged from this historical fact alone |
| ABANDONED_RECONCILED | Planned-only or proven-uninvoked recovery recorded a non-success no-replay disposition | Existing receipt/audit/Recovery proof | Never manufacture Delivery/PR confirmation or free the idempotency key |

## Remaining effectful disposition contract outside accepted evidence lanes

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
barrier, under the same inode lock, boot and directory identity; or reads a
version-2 cancellation certificate appended by that original writer after its
synced intent but before Remote.Publish entry. It stores a typed
non-success receipt and audit in the existing row; no normal transition, idempotency
release, CONFIRMED receipt or remote-absence retry is added. Old/missing/copied/
restored witnesses and armed records without that original certificate cannot use it. See PUBLICATION_NO_INVOCATION_RECOVERY_V1.
The earlier table's general potentially-effectful cases remain blocked without
this concrete pre-invocation proof or the existing positive completion witness.

## Ordinary positive observations

The ordinary RECONCILING-to-CONFIRMED edge is restricted to explicit OBSERVE risk.
For CONTROLLED_MUTATION/HIGH_RISK (and unrecognized risk), a Provider's bare
positive verdict stays MANUAL; only original returned dispatch success or the
separate full-evidence Recovery path can establish terminal confirmation. All
three ledgers enforce this boundary. Historical CONFIRMED labels without bound
original audit provenance remain unresolved; independent positive enrichment
preserves their old audit and original idempotency reservation. See
ACTION_CONFIRMATION_AUTHORITY_V1.md. This does not qualify other post-entry or
ambiguous effects, nor allow non-invocation abandonment of a confirmed row.
