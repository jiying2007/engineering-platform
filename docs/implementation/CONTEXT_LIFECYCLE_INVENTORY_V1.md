# Caller and maintenance Context inventory — current source slice

Base: `2772d0d9c1122d004c7854993bae88ac53d3a138`, followed by removal of
75 unreferenced context-free concrete wrappers, which reduced the inventory
from 133 to 58 direct origins. The production audit below adds one independently
bounded cleanup origin; the 2026-10-22 terminal readback adds one signal-owned
command root, bringing that source slice to 60. The 2026-10-24 capacity command
adds one signal root while isolated qualification reuses the existing bounded
probe stop helper, removing one duplicate cleanup origin. The total
at that stage remained 60. The 2026-10-25 independent observer-journal check adds
one signal-owned command root, bringing the current total to 61. This closes the
*enumeration* of the current
direct constructor candidates; it does not close #229, infer selected-host
capacity, or prove all indirect/aliased Context propagation.

The 2026-10-21 comprehensive audit remediation was reviewed against
`a8557646ca27eb4d7feb09144995478a20a0664c`. The direct constructor set and lifecycle
categories remained at 58 sites; Publisher request authorization and qualification
pipe ownership move two existing source locations without changing their bounded
cleanup roles. The subsequent production audit against
`a0628df29439b4980a9c93ba2d840fa9b8d8319a` adds the five-second, cancellation-independent
Runtime probe stop context. At that stage both audits and the fresh-main
startup-gate pipe ownership correction had 59 direct origins, including 19
bounded cleanup/settlement sites. The pipe correction
changes no Context lifecycle category or deadline.

The 2026-10-22 observation/readback source slice adds one signal-owned
`production-terminal-readback` root with an inherited 30-second total budget.
The finite `production-status` monitor inherits its existing signal root and
adds an explicit total deadline without detaching work. The direct-origin
scanner enumerates Background/TODO/WithoutCancel; these downstream timeout and
deadline constructors are not new independent origins. SLI v3 adds no Context
origin. That source slice contained 60 direct origins, 11 signal-owned roots and 19
cleanup/settlement sites. The current rows below also include the subsequent
capacity, process-start and observer-consumer corrections: 61 origins,
13 signal-owned roots and 18 cleanup/settlement sites.

The preceding operational-integrity repair retained its 60 origins. Linux
pipe output inherits the monitor context, joins its deadline callback and closes
only its separately opened descriptor. Status projection and Recovery row/audit
checks reuse their existing transaction budgets; no independent origin is added.

Production `cmd` / `internal` Go source digest from the existing scanner:

`sha256:3402309972ceb015c3ebd751012ce3522ae2cdacbb133fe8dd43c7806a415ccd`

## Reviewed categories

| Category | Direct call sites |
| --- | ---: |
| Explicitly timed command root | 25 |
| Signal-owned command/service root | 13 |
| Command with downstream budget | 2 |
| Independent bounded cleanup/settlement | 18 |
| Test support helper | 3 |

Command roots either own a signal lifetime or have explicit operation budgets.
The two downstream-budget calls are `processscope.Probe` (its existing bounded
probe/cleanup) and `controlclient.Raw` (its existing independent HTTP/dial/TLS
budgets). Daemon signal lifetime is intentionally not a guessed short request
timeout. A local process signal still must not be interpreted as remote rollback.

Concrete Core and Action types now expose only the required Context variants
for these 75 operations, matching the mandatory interfaces. All 329 former test
call sites supply their Context explicitly; the existing production selector guard
and a new concrete-method-set regression reject reintroduction. There is no
fixture-only wrapper or compatibility shim. The retired wrapper category is no
longer accepted by the inventory verifier. This source inventory is not an
alias-sensitive or interprocedural call graph.

Independent cleanup/settlement includes original Action DB settlement (5 s),
PostgreSQL rollback (5 s), owned workspace cleanup (5 s plus traversal budgets),
Codex control/history/stop/close (5–6 s), failed Worker report (6 s), checkpoint
retention (30 s), sandbox cleanup (20 s) and qualification/probe stop budgets.
Publisher Git stop/reap uses the existing 5 s independent process-scope budget.
An unconfirmed reap quarantines its staging instead of deleting under a live
descendant. Publisher witness completion has its own 5 s cancellation-independent,
synchronous lifetime. It never detaches work or runs Publish a second time.

For every filesystem or RWMutex operation, cancellation is cooperative: a
blocking kernel syscall or lock acquisition is not made forcibly interruptible
by this inventory or by a numerical deadline. Tests need to preserve exact
unknown-result and cleanup errors, not merely return quickly.

The existing CI now verifies this exact source digest, ordered path/line/constructor
rows and category counts with `scripts/verify_context_inventory.py`. That gate
prevents a stale inventory from silently staying green; it is not a full call graph.

## Exact direct candidates

`_test.go` files and symlinks are excluded by the retained scanner. Test helper
packages are shown explicitly, not counted as proven production requests.

| Source location | Constructor | Classification |
| --- | --- | --- |
| `cmd/codex-qualifier/main.go:22` | `Background` | Explicitly timed command root |
| `cmd/codex-saved-login-live/main.go:25` | `Background` | Explicitly timed command root |
| `cmd/codex-wif-live/main.go:27` | `Background` | Explicitly timed command root |
| `cmd/control-plane/main.go:58` | `Background` | Signal-owned command/service root |
| `cmd/control-plane/main.go:145` | `Background` | Independent bounded cleanup/settlement |
| `cmd/eng/artifact_capture.go:63` | `Background` | Explicitly timed command root |
| `cmd/eng/artifact_offline_capture.go:47` | `Background` | Explicitly timed command root |
| `cmd/eng/artifact_retention.go:51` | `Background` | Signal-owned command/service root |
| `cmd/eng/artifact_set.go:82` | `Background` | Explicitly timed command root |
| `cmd/eng/codex_profile.go:86` | `Background` | Explicitly timed command root |
| `cmd/eng/distribution_install.go:63` | `Background` | Signal-owned command/service root |
| `cmd/eng/execution_readback.go:52` | `Background` | Explicitly timed command root |
| `cmd/eng/human_takeover.go:99` | `Background` | Explicitly timed command root |
| `cmd/eng/import_ci.go:54` | `Background` | Explicitly timed command root |
| `cmd/eng/import_engineering.go:34` | `Background` | Explicitly timed command root |
| `cmd/eng/import_engineering.go:77` | `Background` | Explicitly timed command root |
| `cmd/eng/import_engineering.go:113` | `Background` | Explicitly timed command root |
| `cmd/eng/import_engineering.go:161` | `Background` | Explicitly timed command root |
| `cmd/eng/import_engineering.go:198` | `Background` | Explicitly timed command root |
| `cmd/eng/import_engineering.go:260` | `Background` | Explicitly timed command root |
| `cmd/eng/installation_readback.go:49` | `Background` | Signal-owned command/service root |
| `cmd/eng/installation_switch.go:56` | `Background` | Signal-owned command/service root |
| `cmd/eng/pilot_preflight.go:485` | `Background` | Explicitly timed command root |
| `cmd/eng/process_scope.go:13` | `Background` | Command with downstream budget |
| `cmd/eng/production_capacity.go:78` | `Background` | Signal-owned command/service root |
| `cmd/eng/production_live_observe.go:102` | `Background` | Signal-owned command/service root |
| `cmd/eng/production_observer_check.go:121` | `Background` | Signal-owned command/service root |
| `cmd/eng/production_publisher_health.go:42` | `Background` | Signal-owned command/service root |
| `cmd/eng/production_status.go:101` | `Background` | Signal-owned command/service root |
| `cmd/eng/production_terminal_readback.go:35` | `Background` | Signal-owned command/service root |
| `cmd/eng/relay_qualification_kit.go:40` | `Background` | Explicitly timed command root |
| `cmd/eng/relay_runtime_handoff.go:34` | `Background` | Explicitly timed command root |
| `cmd/eng/remote.go:41` | `Background` | Command with downstream budget |
| `cmd/eng/run_continue.go:69` | `Background` | Explicitly timed command root |
| `cmd/eng/run_control.go:107` | `Background` | Explicitly timed command root |
| `cmd/eng/source_checkpoint.go:26` | `Background` | Explicitly timed command root |
| `cmd/eng/work_intake.go:298` | `Background` | Explicitly timed command root |
| `cmd/publisher-service/main.go:74` | `Background` | Signal-owned command/service root |
| `cmd/publisher-service/main.go:104` | `Background` | Independent bounded cleanup/settlement |
| `cmd/sandbox-guard/build_outputs.go:27` | `Background` | Explicitly timed command root |
| `cmd/sandbox-guard/main.go:33` | `Background` | Explicitly timed command root |
| `cmd/worker/main.go:86` | `Background` | Signal-owned command/service root |
| `internal/action/service.go:126` | `WithoutCancel` | Independent bounded cleanup/settlement |
| `internal/githubpublish/armed_cancellation.go:31` | `WithoutCancel` | Independent bounded cleanup/settlement |
| `internal/githubpublish/git_scope.go:71` | `WithoutCancel` | Independent bounded cleanup/settlement |
| `internal/githubpublish/service.go:211` | `WithoutCancel` | Independent bounded cleanup/settlement |
| `internal/processscope/probe.go:32` | `Background` | Independent bounded cleanup/settlement |
| `internal/runtime/codexapp/controls.go:126` | `WithoutCancel` | Independent bounded cleanup/settlement |
| `internal/runtime/codexapp/engineering.go:196` | `WithoutCancel` | Independent bounded cleanup/settlement |
| `internal/runtime/codexapp/engineering_exit.go:22` | `WithoutCancel` | Independent bounded cleanup/settlement |
| `internal/runtime/codexapp/engineering_exit.go:34` | `WithoutCancel` | Independent bounded cleanup/settlement |
| `internal/runtime/codexapp/probe_process.go:101` | `WithoutCancel` | Independent bounded cleanup/settlement |
| `internal/sandbox/engine.go:160` | `Background` | Independent bounded cleanup/settlement |
| `internal/sandbox/testutil/image.go:33` | `Background` | Test support helper |
| `internal/sandbox/testutil/image.go:82` | `Background` | Test support helper |
| `internal/store/postgres/outbox_store.go:185` | `Background` | Independent bounded cleanup/settlement |
| `internal/testsupport/processscope.go:39` | `Background` | Test support helper |
| `internal/workeragent/codex_checkpoint.go:37` | `Background` | Independent bounded cleanup/settlement |
| `internal/workeragent/failure_settlement.go:27` | `WithoutCancel` | Independent bounded cleanup/settlement |
| `internal/workspace/cleanup.go:23` | `WithoutCancel` | Independent bounded cleanup/settlement |
| `internal/workspace/cleanup.go:151` | `WithoutCancel` | Independent bounded cleanup/settlement |

## Caller-bound bundle source reads

Both the Core-side Provider digest check and independent Publisher private-copy
check now pass the initiating Context through pre-open validation and every
bounded source read. A cancelled request cannot yield a valid source/snapshot
or reserve a Publisher invocation; no detached reader or second publication is
started. Reading is limited to the frozen expected size plus one byte and at
most 64 KiB per underlying read, including ReaderFrom optimizations.

The opened artifact directory must match its captured startup identity. Bundle
leaf opens reuse the held-directory no-follow/nonblocking primitive before
checking regular-file, immutable inode, expected size and nonspecial mode.
Worker-readable 0640 files remain valid; Publisher-only witness ownership is not
imposed on the independent Worker artifact. Existing digest and private snapshot
checks remain authoritative. This does not forcibly interrupt a blocked regular-
file syscall/fsync or qualify selected-host latency, storage or #229 overall.

## Import-bound origin references

The existing scanner now parses the exact source bytes it hashes with the Go AST
frontend in `scripts/context_inventory_ast.go`, rather than searching source text.
Explicit context import aliases, multiline selectors and constructor function values
remain reviewable at their origin. Local alias shadowing and source strings/comments
are not mistaken for Context constructors. Physical file/line positions ignore
Go `//line` remapping. Dot imports of context fail the inventory rather than being
guessed across package files. All existing direct entries and category counts remain
unchanged; no new background lifecycle is authorized by this scanner change.

Origin enumeration is still not dynamic function-value points-to analysis, a complete
interprocedural request-flow proof or selected-host kernel/storage qualification.

## Original armed-cancellation certificate

The original Publisher uses one five-second cancellation-independent context to
retain a certificate only after its own synced invocation marker and before it
enters Remote.Publish. The certificate writer is synchronous under the original
slot lock. Failure preserves unresolved state and occupied bytes. This is a
cooperative durability budget, not forced fsync interruption or a new retry lane.

## Caller-bound witness I/O

Witness reads now require the initiating Context through metadata validation,
no-follow descriptor open, bounded source reads, decoding and durability checks.
Cancelled readers return no accepted evidence and retain the immutable bytes for
later independent readback. Descriptor closure errors are preserved. The existing
read path still requires exact canonical bytes, original inode and private modes.

Witness writes accept either the original caller Context or the existing bounded
independent settlement Context. Cancellation is checked before marshalling and
again before the exclusive filename reservation. Once a <=64KiB write starts, its
file sync and directory sync finish synchronously exactly once. Successful sync
is a durability fact: the original arm records it before its next cancellation
check, preserving the original armed-cancellation v2 certificate. No asynchronous
writer, new timeout, retry, deletion, or historical evidence promotion is added.

The original completion path passes its existing five-second settlement Context;
client cancellation does not erase a genuinely completed publication. These are
cooperative I/O checkpoints, not forced interruption of kernel reads/fsync or
selected-host latency qualification. Internal blockers [228,229] remain OPEN.

## Complete HTTP observations before absence classification

Publisher REST exchange retains the initiating Context before transport and while
reading its bounded response. A complete status verdict is distinct from body,
close, limit or cancellation failure; a failed response carrying HTTP 404 cannot
become a ref-absence fact. Successful decoding uses a fresh target committed only
after read/close/decode/cancellation checks. No independent lifecycle or automatic
retry is introduced. A complete negative observation still does not prove original
dispatcher quiescence or authorize replay; internal blockers [228,229] remain OPEN.

## Independent Publisher response lifetimes

The independent mTLS Publisher client now shares bounded response-body reading,
close/cancellation checks and atomic output commit with GitHub REST. Its original
strict internal JSON schema, media checks and 1 MiB response budget remain separate
from GitHub's foreign schema and 2 MiB budget. Health retains its 4 KiB budget and
exact HTTP 200/endpoint-only meaning. Failed read/close/transport causes remain
available through errors.Is without exposing diagnostic text. Cancelled calls do
not begin marshalling or a new exchange; failures never return partial receipts.

This covers publish, observe, original-completion and no-invocation evidence
transport. It adds no recovery state, retry, independent lifecycle or post-entry
quiescence proof. Real local mTLS truncation is controlled protocol evidence, not
selected-host/provider or production qualification. Internal blockers [228,229]
remain OPEN/BLOCKED.

## Control client response lifetime and result uncertainty

The shared CLI/Worker Control client now carries its caller Context through
bounded response reads and typed response decoding, and closes every response
before accepting bytes or a precise HTTP error classification. Failure or
cancellation during read, close or decode cannot replace an earlier output.
All status bodies are bounded: success retains 4 MiB and error responses retain
1 KiB, with at most 64 KiB per underlying read. HTTP status is still observable,
but a truncated 4xx/5xx or failed close cannot become a complete rejection or a
precise server-issued COMMIT/external-effect verdict. POST retains the existing
unobserved-outcome classification and exact identity/epoch/audit readback rule.
Complete server classifications remain distinct. Worker polling suppresses only
an exact complete 409, never an embedded status from a damaged response.
No automatic replay is added.

Cancelled calls cannot begin encoding or a new HTTP exchange. The existing
15-second client timeout remains; checkpoints do not forcibly interrupt arbitrary
kernel I/O. The original full Control package race x20 regression now produces
one complete no-skip event stream checked by the existing parser. Direct Context
origin enumeration is unchanged; this is not full interprocedural/host proof and
internal blockers [228,229] remain OPEN/BLOCKED.


## Service stop and HTTP request ownership

Both command-owned HTTP servers now bind BaseContext to the cancellable serving
Context before accepting connections. A service stop, Control relay failure or
listener failure cancels request-scoped authority work before draining handlers.
Client disconnect cancellation, inherited values and caller deadlines remain
intact. Replacement BaseContext/ConnContext hooks are rejected rather than
silently overriding or merging unrelated lifetimes. This is an in-process helper,
not a new service, queue, independent lifetime or recovery authority.

The existing Control ten-second and Publisher fifteen-second shutdown budgets
remain. Their two existing constructors were reclassified as bounded independent
cleanup rather than task-command roots without changing the then-current total
of 58. The subsequent Runtime probe cleanup brought that slice to 59. Known original
post-effect settlement still uses its existing independent bounded Context.
Cancellation alone never implies rollback, dispatcher quiescence or retry.

Publisher listener failure follows the same cancel-and-drain path instead of
returning while active handlers remain. Both commands share the all-leaf stop-error classifier; a mixed sentinel plus
real failure is retained. The Worker now uses the same conservative rule: custom
error semantics and unobserved/unknown POST or external effects survive caller
cancellation instead of becoming a successful daemon stop.
Shutdown/forced-close errors are joined. Graceful drain still waits for handlers;
these changes do not forcibly interrupt blocked kernel I/O or a misbehaving
callback. Existing supervisor deadlines remain the outer process boundary.

Regression covers live HTTP and ephemeral mTLS clients which stay connected,
pre-effect rejection, original Action settlement and no replay, late listener
failure, and a real isolated PostgreSQL Work insert observed waiting on a table
lock before service cancellation. The lock remains held until the response
completes; post-cancel Work/audit/external-operation counts must remain zero.
The database test is conditional locally and must run under the canonical
PostgreSQL CI environment; absence of that environment is not a PASS substitute.
Internal blockers [228,229] remain OPEN/BLOCKED; no selected-host qualification.

## No-deadline PostgreSQL authority callers

All 85 public PostgreSQL Context entry points now have an explicit finite
operation policy. Forty-seven previously unbounded entries use the existing
20-second database statement safety ceiling as a fallback **only when the caller
has no deadline**. Thirty-six existing caller-derived timers (including 5-second
settlement and 30-second Outbox work) are unchanged. The original finite relay
batch keeps its at-most-64 individually bounded 5-second steps, with immediate
per-step cancellation; its reviewed implementation is unchanged. The new Core
capacity read inherits the same fallback and one five-second child budget across
pool acquisition and the shared read-only authority snapshot.

This closes the gap before SQL begins: server-side statement/lock settings cannot
bound pool acquisition. Nested public calls inherit the first deadline rather
than starting another full budget. Explicit caller/maintenance deadlines are
not shortened or widened. That authority-budget correction added no independent Context origin; its
then-current 58 origins were unchanged. An expired attempted COMMIT remains unknown, and existing
independent rollback/settlement and immutable reservations are retained.

See POSTGRES_AUTHORITY_BUDGET_V1.md for the public-boundary guard and measured
held-pool test. These are finite cooperative operation policies, not a proof of
hard real-time kernel interruption or selected-host latency/soak qualification.

## Confirmation authority and shared snapshot

The confirmation correction preserved the original Action settlement Context
and all 58 origin classifications present at that stage.
Ordinary mutating reconciliation cannot mint confirmation from observation.
Operational readback validates bound original-confirmation provenance and counts
in one read-only repeatable transaction, under the same five-second deadline and
existing independently bounded rollback. Recovery uses its existing serializable
transaction and caller-derived budget. No new Context origin or timer is added.
See ACTION_CONFIRMATION_AUTHORITY_V1.md; internal blockers [228,229] remain OPEN.


## Bounded Runtime qualification and live-probe stop

Every version/features/schema subprocess now owns a bounded execution Context
and a proven process namespace. Its one new `WithoutCancel` origin gives stop
and reap an independent five-second budget even after caller cancellation.
Success requires namespace quiescence; unconfirmed cleanup remains an error.
Live probes reuse this same stop path. They do not create a detached goroutine
or another Context origin. The previous Context-aware caller and signal roots
remain unchanged. These source budgets do not qualify the selected production
host, remote Provider cancellation or filesystem read isolation.

## Core capacity and startup-failure ownership

The new capacity CLI owns one signal Context with a 15-second child budget.
Its direct-mTLS GET, complete-response validation and deadline-aware Linux pipe
output inherit that caller. The server reads original v6 authority and selected
Worker/queue facts in one read-only repeatable transaction under its existing
five-second Store budget; sampling the local pool afterward adds no Context
origin. These cooperative budgets do not force interruption of arbitrary kernel
I/O or establish current Worker/Publisher capacity.

Isolated Codex qualification now reuses the existing five-second
`settleProbeExit` cleanup origin instead of constructing another independent
Background root. A startup error after launch retains the appropriate
stop-unconfirmed category and owned inputs; the original launcher still owns
Wait. Publisher marks its original proof collection uncertain, without minting
a proof or opening a retry lane. Version-probe directory cleanup uses the
outcome of that exact probe. Removing the unused Supervisor/LocalProcessProvider
implementation removes no authorized Context origin and does not close #229.

## Independent observer-journal consumer

`production-observer-check` owns one signal Context and derives its 15-second
operation budget from that Context. The command releases both lifetimes and its
separately owned Linux pipe-output descriptor before returning. It makes no Core
request, starts no monitor, and creates no independent cleanup or reader lifetime.
The monitor's optional externally selected session changes no Context origin.
The existing 60 constructors retain their lifecycle classifications; the one new
command brings the inventory to 61 origins and 13 signal-owned roots. All 85
public PostgreSQL Context entry points and their operation policies are unchanged.

The initiating Context reaches the Linux no-follow path traversal, metadata
checks and bounded JSONL reader. Path traversal is limited to 4096 bytes and 128
components. The consumer checks cancellation during replay, after final file
identity verification and closure, before output, and before granting a successful
strict-mode exit. Freshness is evaluated after readback and again after output
for that strict exit; a slow successful write cannot preserve an expired clear
result. The emitted snapshot keeps its original check time.

Regular-file reads, metadata operations and descriptor closure remain synchronous.
Cancellation checkpoints and a numerical Context budget do not forcibly interrupt
an arbitrary blocked kernel syscall. Linux pipe output reuses the existing bounded
descriptor mechanism; ordinary file output retains its OS I/O semantics. The
shared JSON printer now propagates encoding and write failures without adding a
Context origin or an automatic retry. These bounds establish neither producer
authentication nor selected-host latency, notification delivery or production
qualification. Internal blockers [228,229] remain OPEN/BLOCKED.
