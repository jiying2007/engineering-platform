# Implementation Status

<!-- engineering-platform-readiness
{
  "version": 1,
  "internal_readiness": "BLOCKED",
  "internal_blockers": [
    228,
    229
  ],
  "external_qualification_gates": [
    105
  ],
  "deferred_credential_gates": [
    103,
    106
  ]
}
-->

This is the single live implementation-status authority. Detailed contracts are
linked below; historical checkpoints and receipts remain in Git/PR history, not
parallel live checklists. Audit baseline: `e4ca13e254c883d336ec990e1767f8cc85b8b98b`.

## Maturity boundary

**Trusted self-hosted M1 phase 1 has historical proof. Current repository
implementation and automated tests are not production/unattended qualification.**

**Internal posture: `INTERNAL_P0_RECOVERY_OPEN_EXTERNAL_QUALIFICATION_PENDING` (historical W01-W10 accepted; new internal P0 #228 open).**
For the current small-team baseline, the approved repository-side W01-W10
implementation slices are closed behind required CI, retained byte/evidence
readback and explicit fail-closed boundaries. External qualification
issues remain #103 (managed-workspace WIF), #106 (optional relay/provider) and
#105 (production/unattended acceptance); none can be satisfied by repository
fixtures or CI alone. The independently identified internal P0 #228 covers
crash-orphaned Action PLANNED/DISPATCHED/RECONCILING/MANUAL states: visibility
and transaction fencing do not constitute a verified terminal no-replay
disposition. Migration v13 established an **explicit, audited, no-replay
PLANNED-only disposition**. Subsequent Publisher paths also handle exact positive
original-completion evidence and independently proven non-invocation, including
an original armed-cancellation certificate before Remote.Publish entry. These
specific proofs do not qualify other already-dispatched effects. Remaining
DISPATCHED/UNKNOWN/RECONCILING/MANUAL cases without the required evidence keep
Recovery Proof blocked; no general terminal override is provided.

The Action Gateway now binds frozen authorization, Run epoch and Recovery
epoch prechecks to the initiating caller Context, rather than waiting in
legacy background-context PostgreSQL reads. Action GET also propagates HTTP
cancellation to the same existing ledger without observing/dispatching.
The API ActionGateway now **requires** caller-bound `GetContext` at interface
construction, not just an optional runtime type assertion. The legacy
context-free `Get(string)` fallback was removed from HTTP Action GET;
a new gateway cannot compile against this API without the scoped method.
The Action Service Repository now also **requires** caller-bound
`CreateContext / GetContext / GetByIdempotencyKeyContext / UpdateContext`
for all admission, idempotency, pre-dispatch and post-Provider settlement
ledger calls: there is no optional legacy Repository type assertion or
context-free fallback. Both in-memory test/DEV ledgers explicitly implement
this same single-ledger contract; fault-injection probes continue to
intercept the exact scoped call phases. Original post-Provider settlement
remains independently bounded and never replays a remote effect.
The Gateway Authority also requires caller-bound Core
`GetExecutionContext / GetTaskByDigestContext / GetRecoveryContext`
at its constructor interface. Production Control Plane explicitly checks
that its Core backend satisfies this scoped contract before admitting a
Publisher Action Gateway. The optional unscoped Run/Task/Recovery
fallback was removed; the same in-memory Core ledger now offers scoped
reads and cancellation probes. This is a pre-dispatch Context fence,
**not** proof that an original external Provider completed or quiesced.
The Service rechecks caller cancellation after each pre-dispatch
phase and the production PostgreSQL Action repository now propagates the exact
request Context through idempotency reads, PLANNED/DISPATCHED transactions and
pre-observation reconciliation. A cancelled blocked SQL admission must stop
without dispatching a new external effect. Once a Provider operation may have
happened, its authoritative settlement is attempted under a separate bounded
five-second, cancellation-independent context, with **no repeat Provider
call**. Any failed/ambiguous COMMIT still requires exact persisted readback.
A retained PLANNED/DISPATCHED row is *not* automatically replayed or classified
CONFIRMED. The WorkItem/TaskContract HTTP intake additionally passes its
caller Context into PostgreSQL Work insert/read/update and atomic Task/Plan/Work
freeze/read operations, including the original audit transaction. Cancelled
Work/Task lock waits must not generate any committed Task or audit fragment.
The Run start/get HTTP path now carries caller Context through frozen Task
digest readback, Work readback, and the single Run/Attempt/Session/Work
transaction with the original audit and run.started Outbox. A cancelled
Run-start lock wait must not persist any Run, input, audit or Outbox fragment.
The Pause/Resume/Takeover routes now use the same request Context for
Run/Session readback and their optimistic-versioned atomic transaction.
Run completion also binds that caller to the combined Run/Session/Work
version transition and its original audit. Cancelled row-lock waits
cannot leave a partial takeover or completion. Checkpoint and Delivery
create/get HTTP operations now also carry the caller Context into the
single original PostgreSQL record+audit transaction and readback, with
cancelled lock waits leaving no record or audit residue. Evidence
registration and Verification create/get now also carry the same caller
Context into the frozen Delivery/Task/Plan/Evidence readback and their original
single PostgreSQL record+audit transactions. A cancelled lock wait must not
create PASS, a record or an audit fragment. Review/Closure create/get now also carry caller Context across frozen
Delivery/Verification/Work/Run readback and single audit-backed Review/Closure
mutations. Recovery get/begin/complete HTTP operations now also carry caller Context
through PostgreSQL readback and the existing audit/epoch/proof-bound
transactions, with independently bounded cleanup after cancellation.
The Core Store interface now **requires** those three caller-bound Recovery
methods at compile time, not via an optional API type assertion; the legacy
context-free HTTP fallback was removed. Both the existing PostgreSQL backend
and in-memory DEV/test Core implement the same Recovery epoch authority, with
cancelled Begin/Complete negative tests. Direct concrete-store maintenance
methods remain independently inventoried under #229 and do not authorize any
external Action replay or effectful terminal reconciliation.
The remaining **24 HTTP Core contextual adapters** (Work/Task, Run/Session,
Steering, Checkpoint/Delivery, Evidence/Verification and Review/Closure)
now also require their original `*Context` method at the compile-time
Core Store interface, rather than conditionally type-asserting an adapter
and falling back to context-free record/transaction calls. Existing
PostgreSQL methods are reused without changing atomic business/audit/Outbox
authority or state schemas; the in-memory DEV/test Core delegates to the
same single ledger after rejecting pre-cancelled requests. A source AST
regression rejects reintroduced runtime-optional `s.store.(...Context...)`
adapters in HTTP modules. This is request-to-Core caller cancellation
hardening, not a selected-host worst-case lock latency measurement,
an unattended credential qualification, or original external effect proof.
The historical Steering record/read and the Attempt/Task-revision read paths
now also offer caller-bound PostgreSQL variants; Steering HTTP readback uses the
same request Context while live Codex steering retains its existing control
protocol. Some controlled internal Core/CLI calls still use bounded server-side
PostgreSQL deadlines rather than an initiating caller Context. P1 #229 stays
open for remaining call-site inventory, ambiguous COMMIT readback, explicit
maintenance lifecycles and real long-run qualification.

The production PostgreSQL connection path additionally enforces bounded
server-side query, lock and idle-transaction waits and retains stricter
operator limits. This bounds waits but does not universally propagate HTTP/Worker
caller cancellation through the remaining context-free Core methods or
resolve COMMIT ambiguity;
P1 #229 remains open pending that end-to-end qualification. The
Core, Recovery and pre-dispatch Action PLANNED mutation paths classify failed
COMMIT acknowledgements explicitly as an unknown commit outcome. The API
requires exact persisted ID/epoch/idempotency/audit readback and never treats
a cancelled COMMIT as a known rollback; this is fail-closed error labeling,
not proof that a specific production transaction committed or rolled back.
See [COMMIT outcome/readback](../implementation/COMMIT_OUTCOME_READBACK_V1.md).
The merged #246 additionally classifies 25 existing Worker/Offline/Codex/
preparation/source-continuation mutating COMMIT sites and maps Worker HTTP
unknown outcomes to fixed 503 exact-readback semantics. Its isolated-database
lost-ACK regression simulates a real durable commit. The subsequently merged
#249 adds a **real isolated localhost TCP cutoff after backend COMMIT** and
verifies one retained Recovery epoch/audit plus stale-epoch denial. Exact PR-head
[CI 38111732156](https://github.com/jiying2007/engineering-platform/actions/runs/38111732156)
and fresh-main [38112232617](https://github.com/jiying2007/engineering-platform/actions/runs/38112232617)
both passed 5/5; this still does not establish production networking,
external provider-dispatcher quiescence or unattended operational readiness.
The independently authorized Execution Abandon HTTP route also preserves the
same fixed, non-leaking `commit_outcome_unknown` 503 if its Recovery transaction
COMMIT acknowledgement is lost: exact execution/Recovery epoch, receipt and
immutable audit readback must precede any further operator decision. A generic
503 is not rollback evidence and does not authorize a second abandonment.
Ordinary Control API Store/Action/Recovery errors are mapped to fixed
non-leaking messages while preserving their original HTTP status categories;
raw PostgreSQL/Provider diagnostics and malformed request parser fragments are
not sent to callers. A source AST guard rejects new direct `Error()` reflection
in `writeError`, and negative route tests cover backend error injection. The
existing exact `commit_outcome_unknown` 503 readback path remains distinct.
Redaction does not establish original operation outcome or grant replay.
The direct-mTLS `controlclient` additionally distinguishes a server-classified
`ErrCommitOutcomeUnknown` from `ErrPostOutcomeUnobserved`, the latter used
when a mutating POST has no usable transport or successful response body.
Both require original-ID/epoch/audit readback rather than an automatic retry;
the more general transport condition does not assert that COMMIT happened.
Regression tests terminate a real local mTLS POST connection after admission
and inject malformed 2xx bodies without introducing a new HTTP attempt.
Source/provider production quiescence remains P0 #228, not proven here.
Action Service post-Provider settlement failures and invalid Provider
verdicts also expose a separate typed `ErrExternalEffectUnresolved`
rather than an ordinary HTTP 422 validation error. API maps it to a fixed
non-leaking 503 directing original Operation/idempotency/Run/Recovery/audit
readback, and the direct-mTLS Control client recognizes the same code
without conflating it with server COMMIT-unknown or lost HTTP response.
If a nested COMMIT result was unknown, that more precise classification
still takes priority. No external Provider is retried or receipt invented.

Action reconciliation now checks initiating Context before operation readback,
after readback and after the original durable RECONCILING admission, before
starting a new Provider observation. If the caller was lost immediately after
admission, RECONCILING remains unresolved with no invented remote receipt or
automatic retry. An observation that actually began still uses the existing
independently bounded durable settlement even if the original caller then
disconnects. This closes one request-cancellation path under P1 #229, **not**
original-dispatcher quiescence or the provider-specific terminal Recovery
Proof required by internal P0 #228. The Recovery Proof's cancelled
PostgreSQL lock-wait cleanup now reuses the existing independently bounded
rollback helper, rather than attempting rollback with an already-cancelled
request Context. An isolated lock-wait regression requires cancellation
before release, no proof/audit residue and a subsequent successful authorized
proof. The same source-level COMMIT guard also detects any new direct
unqualified `Rollback(...)` outside the established bounded cleanup helper.
This is transaction hygiene, not external-effect disposition.

The default internal lane remains trusted Ubuntu with a saved ChatGPT Codex
session. This project is independent of digital-worker and inherits no old
schema/runtime compatibility obligation.

[The fixed M1 proof](M1_RETAINED_PHASE1_CLOSURE_2026-09-29.md) records real Feature
#54 and Debug #55 closure. It does not qualify later runtime changes. Do not rerun
those subjects or rewrite their receipts to manufacture current production proof.

## Publisher kernel and host-admission correction

The production Git transport now uses the existing dedicated PID-namespace
launcher, not plain exec. Successful original Git return requires namespace-init
reaping; cancellation has one bounded stop/reap lifecycle. Unconfirmed local
termination quarantines staging and cannot create a completion witness. This
local kernel fact does not cancel an already accepted GitHub server operation,
upgrade historical evidence, or authorize replay of negative/partial observations.

Production host admission now reads the same Publisher v2 ServiceConfiguration as
the service, including its Publisher-owned private witness_root. The old host
Configuration v1 reader incorrectly rejected valid deployed v2 configuration.
Canonical paths, disjoint artifact/evidence roots, role ownership and access are
checked; no v1 fallback or provider qualification is added.

The direct Context inventory is updated to 132 constructors and checked in CI
against exact source digest, ordered locations and category counts. Its scope is
still direct enumeration, not interprocedural or selected-host qualification.
The older completed-publication candidate is reviewed and registered as a seventh
retained prototype; only its scoped Git launch is absorbed, not its alternate
schema/table/endpoints. See [the disposition record](../implementation/RECOVERY_PREPARATION_DISPOSITION_V1.md).
Internal #228/#229 remain open; all current readiness flags stay BLOCKED.

## Response, Memory and readiness hardening

A positive-only publication recovery path now requires original independent
Publisher completion witnesses plus fresh exact positive PR readback. Core
collects evidence itself over mTLS and records a serializable original-Action
settlement, full evidence, audit and exact readback; no user-uploaded stop flag,
negative observation or new publish call authorizes it. Publisher-service config
v2 requires a separate private witness root and never reuses occupied slots.
Historical/missing-completion and interrupted in-flight publication still remain
unresolved. This repository implementation is not selected-host/provider or
production qualification; internal [228,229] remains BLOCKED. See
[publication recovery witness](../implementation/PUBLICATION_RECOVERY_WITNESS_V1.md).

Publication readback, exact-repeat settlement and aggregate Recovery facts now
share row/observation/time/original-audit-payload validation. Corrupt or missing
receipts/events/operations cannot clear a new proof or complete an older proof.
Database regressions include real post-COMMIT TCP acknowledgement loss, concurrent
settlement, cancellation while waiting on three authority locks, failed audit
rollback, mTLS Core roles, and native dump/restore into a new isolated database.
The upstream observation in these transaction fixtures is synthetic. The
transient systemd fixture also uses Publisher v2 and a random private witness
StateDirectory, preserving the canonical security properties and read-only
artifacts. It never maps test cleanup to production state. Canonical
CI still requires PostgreSQL 17 and its five original jobs; supplementary native
same-major tests are not a substitute. No production/host closure is asserted.


Normal Action updates now compare all immutable original reservation fields in
the same PostgreSQL UPDATE predicate; a different authorized Run cannot lend
its admission to the original operation ID. Both existing memory ledgers enforce
the same identity fence. The standalone Action MemoryRepository also checks
cancellation immediately after acquiring its original lock. Real PostgreSQL
regressions reject eight identity substitutions without row/audit changes and
preserve exact original post-Provider settlement after Recovery Begin. This is
not a new terminal disposition, a retry grant or closure of #228/#229.

The merged readback hardening rejects null open-PR observations before any
later publication decision, and joins one-attempt Codex/offline failure-report
uncertainty with the original execution error. The non-test Core selector guard
complements the scoped Store interface; no empty response authorizes replay.

Both engineering credential lanes now share their existing bounded process/control
settlement: original turn errors, failed namespace stop and failed Core close
acknowledgements remain independently discoverable. History-retention and control
RPC/report errors are joined rather than masking one another. Stop, history,
report and close retain original request values under cancellation-independent
bounded Contexts; each original operation is attempted once. Any settlement
failure clears the successful engineering receipt. Kernel-controlled tests include
an interrupted real fixture process with a simultaneous Core-close failure;
unit-only synthetic proofs are never represented as production evidence. These
changes do not supply original Publisher dispatcher quiescence or close #228/#229.

The service-facing Core Store contract now contains only caller-Context methods;
31 unscoped interface entries were removed. Historical Attempt/Task-revision/
Steering methods use the same required Context contract and Memory post-lock
cancellation checks. Concrete convenience wrappers remain for direct fixtures,
not a network fallback. Memory historical Steering also matches PostgreSQL's
Run/Session identity rejection.

Preparation cleanup now preserves primary and cleanup errors together. Managed
and incomplete-create slot deletion use anchored, batched, cooperative traversal
with a five-second deadline, 100,000-entry and 128-level budgets. Managed owner
bytes survive cancellation/budget failure, and normal completion removes only the
empty owned slot. Cleanup never follows symlinks, opens special-file contents,
detaches deletion, replays execution or removes retained artifact roots. Blocking
kernel filesystem calls are not made interruptible by a Context; pathological
storage/real-host stop latency remains explicitly unqualified under #229. See
[workspace cleanup lifecycle](../implementation/WORKSPACE_CLEANUP_LIFECYCLE_V1.md).

The already-merged #262 Control client rejects a root null/non-object,
validates explicitly tagged top-level required response members, and commits a
fresh decoded output only after validation. Explicit nullable assignment is a
valid empty claim; missing assignment is not. Complete allowlisted 503 codes
keep their precise classification; unusable 5xx bodies preserve outcome-
unobserved, HTTP status and internal cause. Intact ordinary errors are not
reclassified as proof of COMMIT. No retry is added. The remaining hardening in
this slice does not overwrite or reimplement #262. Nested domain checks remain
the consumer's responsibility.

Memory's existing scoped methods additionally recheck cancellation after the
original RWMutex is acquired, before ledger access or mutation. Context-free
concrete methods delegate to the same implementation; there is no second ledger.
RWMutex waiting itself is not cancellable. This closes the post-wait access
race, not all #229 lifecycle, pool-wait or production latency qualification.

The unreferenced context-free `internal/authority` duplicate is retired; the
single production Gateway retains its frozen-task authorization and now rejects
cross-Run or stale Session identities in its epoch precheck. Existing PostgreSQL
transactional admission remains authoritative. An isolated PostgreSQL regression
holds the only pooled connection until seven caller-bound read/mutation/admission
paths time out, then requires zero business/audit/Action residue, zero Provider
calls and a usable pool. This is pool-wait cancellation evidence, not a production
capacity measurement or permission to retry an uncertain operation.

Delivery envelope v2 separates CI/source provenance from declared internal
readiness, production acceptance and deferred credential alternatives. The one
machine-readable block in this document is the declared readiness authority.
Missing or inconsistent blocks fail closed; VerifyStatusBytes requires the exact
original status bytes. The envelope is neither independent issue closure nor
production qualification. #228 still needs an implemented and qualified
provider-specific effectful terminal disposition; the state matrix is an
acceptance contract and regression coverage, not that implementation.

## Original completion proof and witness integrity

New positive Recovery admission requires completion-v2 with the original bounded
Git namespace-init reap proof list, not merely a successful PR receipt or a current
binary version. Older audit-consistent Core receipts keep exact read-only history;
legacy pending completion records cannot be promoted. Publisher, client, Provider,
Core and new database settlement enforce this distinction. Private witness files
reject wrong ownership, special modes, hard links and final symlink/FIFO opens.
No original witness/key is erased and no replay or new schema is introduced.
See `docs/implementation/PUBLICATION_COMPLETION_PROOF_V2.md` for the acceptance and
storage/kernel limits. #228/#229 remain BLOCKED; neither the historical byte
reader nor a local reap proof is remote cancellation or production qualification.

## Pre-invocation publication recovery

The Publisher now writes STARTED-v2 while holding the original slot inode lock,
then durably arms invoked.json before Remote.Publish. A new explicit recovery
path can seal a same-boot/original-inode STARTED-v2 before arming (evidence v1),
or read an explicit original-writer cancellation certificate written after a
fully synced invocation marker but before Remote.Publish entry (evidence v2). This is independent proof of **not calling the
Publisher remote**, not a remote-absence inference. It creates a typed
non-success reconciliation receipt in the existing Action row, preserves the
original idempotency reservation and diagnostic, and never creates a PR/success
receipt or grants replay. Core obtains the proof through its configured mTLS
Publisher; request JSON cannot substitute evidence. Active holders, old v1 starts,
restored/copied directories and foreign boot/slot identities stay blocked. An
armed/partial intent without the original cancellation certificate remains
unresolved; after function entry, errors or timeouts cannot mint this certificate. The positive #268 path and #269 processscope remain intact.

The direct Context inventory remains source-bound. Database/mTLS/concurrency/
COMMIT-loss/corruption/restore checks extend existing canonical gates. The exact
scope and local-filesystem assumptions are recorded in
`docs/implementation/PUBLICATION_NO_INVOCATION_RECOVERY_V1.md`.
**#228 and #229 remain internal blockers**: historical missing witnesses,
post-intent crashes without the certificate, errors after function entry,
ambiguous external results and selected-host lifecycle/backup/long-run acceptance
are not promoted by this pre-effect path. Nine existing database fault/transport
suites and the non-invocation restore drill now cover both evidence versions.

## Typed embedded routing and Skill maturity

The post-RC domain hardening adds an **optional explicit target_context**
(`target_id` and `platform`) to existing Work/Task intake. Both CLI and
Core require the exact declared target to match MaterialManifest; contradictory
platform/subsystem inputs fail closed. The exact target platform and **selected v1 Skill metadata contract digest**
are bound into new typed TaskContracts. Core derives the digest from its
catalog, and `eng work-intake` independently verifies Task readback before
creating a Run; selected methods, version, evaluation and prohibitions cannot
silently drift with unchanged Skill IDs. Untyped/historical Task JSON and
digests remain unchanged. A digest is an identity, **not** evidence that a
Skill is EVALUATED, PILOTED or PROVEN. No chip-name heuristic,
second Target authority, Device/HIL entitlement or new Runtime mode is introduced.
See [typed target routing v1](../implementation/EMBEDDED_TARGET_ROUTING_V1.md).

All ten Skill catalog entries now carry complete **DEFINED-stage** input,
required-material, method, output, blocking/prohibited-action and evaluation
descriptions. **Newly created v1 Skill-guidance TaskContracts** with an
explicit typed TargetContext and frozen selected-Skill digest deliver the
**actual selected methods and BLOCK/evaluation guidance** to the Core-bound
Codex turn after an independent Worker-side **typed TaskType/TargetContext
Capability/Skill route check plus source/catalog digest check**. Even a
self-consistent Task/Intent/Preparation with a valid Skill digest cannot
substitute another platform's methods. Before any new Codex turn can be finalized/delivered, the Worker also checks
the real provider-turn receipt's PromptDigest against the **exact rendered
prompt bytes** and its frozen PromptIdentity; a mismatch stops delivery.
This is not proof the model obeyed instructions, and archived Result readback
does not re-render prompts with a newer Skill catalog.
A new optional `skill_guidance_version=1` field separates these new tasks from
older typed/digested or untyped Tasks; the latter continue to use the exact
name-only PromptIdentity. Historical Task prompts and digests remain unchanged.
See [typed Skill Runtime guidance](../implementation/TYPED_SKILL_RUNTIME_GUIDANCE_V1.md).
This is prompt delivery and a machine-readable planning contract, **not PROVEN
engineering quality**. No Skill was promoted from `DEFINED`; actual expert
effectiveness and time/cost improvements remain unqualified until independent,
repeated, real task Evidence exists. Catalog entries grant no Action permissions.

**Team Skill ownership / deferred work:** The existing team-wide reusable
engineering Skill/method assets are maintained in
[agent-dev-kit](https://github.com/jiying2007/agent-dev-kit). This platform's
ten locally versioned, `DEFINED` Skill entries remain bounded Task/Prompt
method and identity contracts, not a replacement team-wide Skill authority.
No catalog parity, synchronization, migration, additional runtime dependency
or proven engineering effectiveness is claimed. Independent Skill evaluation,
maturity promotion and catalog expansion are deferred. Keep the shipped routing,
method-digest, prompt-byte and historical Result verification fences; do not
create a second Skill delivery/qualification service to resolve this ownership
boundary. This deferral does not affect RC or production acceptance gates.

## Current contracts and limits

| Area | Implemented | Not implied |
| --- | --- | --- |
| Core | PostgreSQL business/audit/outbox, frozen Task/RunInput, execution and Recovery epochs; schema v12 adds explicit non-success abandonment reconciliation for exact expired offline/Codex executions | Production service operating acceptance or replay authority |
| Live controls | mTLS RunControl, durable one-shot steering/interrupt, exact actor/sequence/epoch/turn binding and sealed transcript | ACK means stopped; accepted input means objective fulfilled |
| Process lifetime | Dedicated Linux user/PID namespace; receipt v4 and transcript v2 bind kernel-observed init reap | Complete filesystem/resource isolation or control of unrelated host processes |
| Stopped source | Private exact-byte archive after confirmed stop; archive v2 also embeds a self-contained, empty-repo-verified Git graph for the frozen base/direct Finalize child; immutable Core descriptor and new-directory restore | Full model session, automatic replay, successful Delivery or takeover authority |
| Source continuation | Work owner with RunStart + RunControl explicitly creates one fresh successor; old Run fenced as STOPPED_NO_DELIVERY | In-memory pause/resume, inherited execution permit or model-memory reconstruction |
| Finalization | Independent trusted-Git checkout reproduces complete source/tree before publication; recipe v2 emits a self-contained result bundle and proves exact base+result import from an empty object database | Clean Git status alone proves all source was delivered; successful result retention equals full repository history |
| Post-turn failures | Immutable fsynced phase records; catchable failures preserve stopped source; read-only local/Core reconciliation; actual offline Worker SIGKILL after permit/renewal remains unresolved/unreplayed; schema v12 can explicitly abandon an exact expired AUTHORIZED/UNKNOWN offline/Codex execution under Recovery without creating a result or replay authority | Automatic source capture after arbitrary Codex/host death, automatic repair or replay |
| Offline build outputs | Frozen output names/budgets, guarded tmpfs collection after child reap, existing local/Core receipt and Evidence byte checks; actual native C compiler, freestanding host-ISA firmware and Cortex-M0 ARM EABI cross-toolchain ELF/map references all traverse the installed Worker/Core/private-restore chain | Writable host builds, large production firmware or target-board boot/timing/electrical qualification |
| Raw artifact sets | Explicit plans or producer-derived execution/Context/offline-output selection, byte packing/verification and fresh-directory restore; successor execution capture also retains and verifies the exact upstream continuation checkpoint; execution capture requires the exact sealed ControlTranscript plus bounded raw Codex item/completion history; native PostgreSQL dump/restore drill; required ENOSPC pack/restore fail-closed test; externally pinned scheduled verification/copy into a second owner-private root; required real-systemd CI runs the maintenance oneshot as canonical `engineering-preparation:engineering-platform` and proves first-copy/idempotent readback/fail-closed missing config without installing production-named units | Provider credential/session bootstrap and internal streaming state, actual off-host/second-site or production-host retention qualification, encryption/key custody, destructive GC or automatic disk-exhaustion repair |
| Provider | Profile v3 binds provider, credential, execution mode, binary and configuration identity | Admission mechanics equal live provider qualification |
| Compatibility | Schema3/contract2 probes actual Codex under engineering namespace/config; repeatable environment identity | Authentication, model/tool execution or production qualification |
| Publisher | Independent mTLS service; production startup rejects in-process publishing; bounded authenticated /healthz observation is bound to exact remote configuration digest | Endpoint health is upstream GitHub/provider health, capacity, publication authority or production readiness |
| Distribution | One six-role list; source-matched immutable install/readback; previous-version readback by retained source+manifest identity; explicit stopped-release switch/rollback keeps replaced bytes; real cross-version upgrade/rollback drill | Service activation, database downgrade/migration rollback, restricted-/proc bypass, in-flight external-effect recovery or checksum-only authenticity |
| Service manager | Explicit restart/cgroup stop policy; installed-service lifecycle under real systemd; transient four-role Publisher -> Control -> admission/preparation graph uses distinct DynamicUser identities and exact Wants/Requires/After readiness ordering; required CI also creates the canonical four named Unix users + shared group on a disposable runner, executes one constrained transient unit per identity, then proves full account/group cleanup | Actual production-host account provisioning/unit installation, active publication/model crash recovery or replay safety |
| Production preflight | v2 checks configuration and actual host facts with expected source SHA, service-user primary/supplementary groups, parent traversal/read-write mode access and hard-link rejection | CONFIG_VALIDATED or HOST_VALIDATED means operational READY; ACL-only grants are not inferred |
| Operational status | v5 separates database authority from unobserved readiness; reports PLANNED, DISPATCHED and unqualified historical SAFE_TO_RETRY external reservations as degraded, plus bounded per-profile Worker poll facts from existing last_seen_at, exact freshness checks and queue-progress windows; Publisher endpoint health is a separate authenticated observation; required Go/PostgreSQL CI runs five consecutive bounded 96-Run/8-Worker concurrent admission/security characterizations, each with three interleaved security rounds (48 rejected unauthorized/profile-mismatch probes; aggregate 480 Runs / 240 denied probes) | Worker identity count/recent poll, measured CI latency or Publisher endpoint reachability proves calibrated capacity, upstream health or production readiness |
| SLO | v2 separates unverified summaries from subject-bound source readback | Calibrated targets or production qualification |
| Pre-live | Event SHA/tree, terminal plan v3 with frozen single-canary, emergency-stop, no-provider-fallback and no-automatic-DB-downgrade gates, exact nonempty no-skip test inventory, and an internal RC delivery envelope bound to exact successful main-push CI/source tree/governance bytes; main-only CI emits it as a separate retained artifact | Passing the frozen external canary/provider-revoke/Publisher-revoke/DB-restore gates, independent human Review or production qualification |
| Operator CLI | Run controls, stopped-source restore/continuation, private readback/artifact storage, authenticated Work -> Task -> Run intake and exact-epoch Human Takeover | A WorkBuddy-specific backend/authority, automatic model execution, or a completed end-user WorkBuddy UX |

## Execution, failure and continuation

[Live controls](../implementation/LIVE_CODEX_CONTROLS_V1.md) persist DISPATCHING
before provider effects. Ambiguous delivery is not replayed. Input acceptance,
interrupt ACK, matching terminal event and
[kernel stop](../implementation/PROCESS_NAMESPACE_CONTAINMENT_V1.md) are distinct
observations; Core Finish, Publisher and Evidence enforce their binding. No
plain-process or missing-proof fallback is admitted for current execution.

[Source preservation](../implementation/STOPPED_SOURCE_CHECKPOINT_V1.md) runs only
after sealed quiescence. The Preparer rechecks ownership, Task/input/preparation,
Git base/configuration and approved Context. Modified/untracked/ignored source
is preserved within bounded private archives. Core records a Worker attestation,
not a claim it downloaded private bytes. Restore requires external digest/Run,
a new private destination, full byte/mode/link/tree readback and fsync; it never
imports .git/HOME/login state or grants execution.

The actual Worker records immutable ENTERED phases before post-turn persistence,
Finalize and reporting. Catchable later failures attempt private source capture
and one immutable checkpoint registration. Preservation-only Git inspection
allows the original base or one direct Finalize child; normal execution/reopen
retains its original-base fence. Success makes no unsolicited source copy.
Local FAILED_UNCONFIRMED cannot override an already committed Core FINISHED.
Abrupt Codex/host death and exhausted storage may still prevent source
capture/journaling; phase entry alone is not phase completion or proof of a
crash. This is an accepted fail-closed boundary, not a promise of impossible
post-mortem source recovery: the platform must preserve/reconcile surviving
facts and must not auto-replay, invent missing bytes or delete private data to
manufacture space. Separately, required native CI now kills an actual installed offline
Worker with SIGKILL after its permit is fsynced and its lease renewed: Core stays
non-terminal without a receipt, a second Worker cannot replay the same
execution, and Recovery proof remains blocked until reconciliation. Schema v12
adds the bounded terminal reconciliation for this exact orphan class: an existing
`recovery:reconcile` mTLS identity must bind the exact Run/execution/current
Recovery epoch, a retained external observation digest and fixed
`ABANDON_NO_REPLAY` disposition after lease expiry. The durable row becomes
`ABANDONED_RECONCILED`, never `FINISHED`; its unique Run reservation remains,
execution/replay/production authority remain false, exact retry is idempotent,
and changed observation or reconciler identity conflicts. The normal immutable
Recovery proof plus a distinct completion identity are still required before
returning to NORMAL.

`eng execution-readback` validates existing private permit/phase/turn/result/
checkpoint identities. Explicit artifact paths opt into source/bundle byte
checks; metadata paths are never followed. Offline default does not contact Core.
Optional --core makes one authenticated GET, validating token, sealed transcript,
result and checkpoint. Descriptor observation is not a private byte download.
No source/model/steering bodies are printed and no repair/replay is authorized.

[Continuation](../implementation/SOURCE_CONTINUATION_V1.md) requires Work owner,
both permissions, exact versions/epochs/checkpoint, observed requested
interruption, and no unresolved effects or prior Delivery. One transaction
retires the old Run, creates one new Run/Attempt/Session and emits normal
run.started intent. Concurrent decisions share the source fence; exact retry
observes the original decision. Generic Run creation cannot inject lineage.
A completed-turn failure checkpoint is not an interruption-only continuation.

Fresh host approval supplies private source to a new slot at the original Task
base. A NEW model turn gets explicit continuation context, not invented memory.
The final diff contains inherited and new representable changes. Independent
checkout rejects ignored-file/empty-directory loss and attribute transformation;
it neither force-adds private files nor deletes them. Failed Finalize may leave
a local commit: reconcile the failed execution instead of blindly retrying it.

## Private artifacts and historical evidence

[Private artifact sets](../implementation/PRIVATE_ARTIFACT_SET_V1.md) preserve an
explicit sorted member list with raw plan/archive digest and Run anchoring.
Files remain private and non-executable. Packing/readback/restore never calls
Core, runs a model/Git/SQL, overwrites old destinations or uploads public data.
Generic completeness is ONLY relative to declared members; producer semantics
are not inferred. `capture-execution` derives current producer records and every
frozen Context entry from the original anchored permit, requiring all referenced
source/result bytes plus private copies of the exact Permit-bound Codex runtime
binary and qualification receipt. It rejects missing/extra Context, runtime byte
drift, qualification semantic drift and unbound artifacts. For an explicit
source-continuation successor it additionally requires the exact upstream
source-checkpoint archive and validates the frozen ContinuationRef/Task/base/
Codex-tool/Worker-profile binding before retaining those bytes. It now also requires and retains the exact private sealed ControlTranscript raw
bytes, and Core readback compares that transcript structurally when present.
Schema-v5 Codex receipts bind a private bounded EngineeringHistory: exact raw
accepted `item/completed` Params plus terminal `turn/completed`, fsynced before
a successful turn proceeds and automatically included by `capture-execution`.
Those private bodies do not enter Core. Credential/session bootstrap, bearer
material and provider-internal streaming state remain excluded.
`capture-offline` derives the original permit/report and every collected output
from the frozen offline Profile. Both raw records require external digest anchors;
failed commands retain failure records with zero outputs, never successful-build
claims. It does not contact Core or execute restored data. Caller-selected native
backups still need their own consistency/restore.

Actual Worker tests delete original records/source/bundle locations, restore the
raw set and re-run existing semantic readback. A host C-compiler reference keeps
ELF/map outside source and restores outputs after removing both original source
and build directories. The existing [offline Runtime](../implementation/OFFLINE_EXECUTION_V1.md)
also supports an explicitly frozen bounded build-output contract: the non-root
PID1 guard collects exact files in private tmpfs after reaping children, and the
existing Worker/Core/Evidence path binds their bytes. A real scratch-container C compiler, a freestanding host-ISA firmware-like
ELF/map reference and private artifact restore are covered. The separate
no-network Cortex-M0 cross-toolchain reference is a controlled fixture, not
qualification of target-board boot/timing/hardware behavior or larger
production firmware capacity. A same-Run compiled Worker/mTLS/PostgreSQL/native
C build additionally captures and restores its original records and raw outputs
after deleting source/Git/preparation roots, without creating Evidence or new
execution. Producer-derived capture tests remove the entire prepared root
including Context, restore, and recheck every frozen input object and producer
record. Original Git-base bytes are now retained on the successful-result and
stopped-source producer paths; toolchain/container-image bytes and broader
credential/session/provider-internal state remain outside the set. Scheduled
verified private replica retention is implemented. Required native CI also
executes that maintenance path under real systemd using the canonical
`engineering-preparation:engineering-platform` identity on a disposable host,
proving first-copy verification, idempotent existing-replica readback,
fail-closed missing configuration and full account/group cleanup without
installing/enabling the production-named retention unit/timer. Actual production-host/off-host/second-site placement plus encryption/key
custody are deployment/operator acceptance gates. Bearer credentials and
provider-internal session/streaming state remain intentionally excluded from
artifact capture, and destructive GC remains an explicit operator action rather
than an automatic background repair path. A required
1 MiB private-tmpfs fault test also proves real ENOSPC during artifact pack and
restore cannot publish a successful archive/report; a private partial restore
directory may remain explicitly unverified for operator disposition.

The mandatory PostgreSQL 17 drill additionally retains a native dump and expected
authority snapshot through the raw set, removes and checks absence of the original
DB/dump/plan, restores bytes, then explicitly runs native transactional restore.
All existing authority, audit, Outbox and Recovery-epoch checks still apply. This
is a controlled database/container fixture, not a customer backup, production or
cross-version qualification; the storage layer itself never executes SQL.

[Two historical terminal fact archives](../evidence/m1-terminal-facts/README.md)
retain selected original bytes/source ZIP identities in Git. Final Review uses
allowlisted facts, not PKI/dumps/environment/log directories; verification deletes
its disposable bootstrap PKI after Control stops. Required intermediate database
transport is separate. Historical schemas and decisions remain historical proof.
Neither terminal facts nor source checkpoints are full runtime backups.

[Branch disposition](../reviews/BRANCH_DISPOSITION_2026-10-04.md) separates
integrated refs, unmatched prototypes and retained M1 subjects. Protected-main
retirement receipt `37476720045` applied the expanded manifest against
`310874bd0a6297310a31c919b9d530dbf502bfa6`: 32 refs were
`DELETED_READBACK_VERIFIED` and five were already `ABSENT`. Current governance
also requires the exact same-repository merged PR head tree to equal verified
main before automatically leasing deletion of that just-merged working ref.
Six divergent historical refs are explicitly disposed as frozen
RETAINED_SUPERSEDED_PROTOTYPE refs. The set includes the stale terminal-v3
status transport and the independent-artifact-mirror experiment; both are
exact-head guarded historical lineage, not alternate supported paths. Required CI pins their exact remote heads;
they remain source-history lineage and are not treated as byte-identical or
pending product work. Required main CI catches drift on each change. A separate
[read-only daily retained-ref drift sentinel](../implementation/RETAINED_REF_DRIFT_SENTINEL_V1.md)
also checks the actual remote inventory while GitHub scheduling remains active,
with a 90-day bounded diagnostic receipt. Public-repository schedules may be
delayed or automatically disabled after 60 days of no repository activity, so
continuous monitoring after extended inactivity remains an operator/scheduler gate. This is **not** GitHub administrator
mutation protection and cannot repair or promote retained refs.
Preserve unchanged:

- Feature `6009ea95785237ad6ff9f5c9cba911b4891dfa58`,
  `engineering-platform/3ac04fc7097e8375e5f7c1f8`.
- Debug `91d7d9fa068b7667bab5c211f13cd9e0151aeb92`,
  `engineering-platform/994964383ed085e51545105d`.

## Provider and production qualification

[Credential lanes](../implementation/CODEX_CREDENTIAL_LANES_V1.md) remain
`openai-codex / chatgpt-session / trusted-self-hosted` and
`openai-codex / workload-identity / unattended`. No relay identity or silent
credential/provider fallback is admitted. WIF #103 and relay #106 live
qualification remain deferred; speculative relay tooling stays frozen.

Qualification schema3/contract2 launches the native binary in the actual user/PID
namespace, exact workspace-write config and fresh credential-free HOME. Only
initialize/initialized/thread-start are sent, followed by confirmed init reap.
The stable kernel/architecture/UID/GID digest excludes PID/inode/time. CI probes
twice; each actual Worker CLI requalifies and compares the entire receipt. Old
current-admission receipts fail, but historical M1 records are not rewritten.

**Production #105 stays open.** Terminal plan v3 additionally freezes the
single-maintenance-canary profile, selected-provider emergency disablement,
Publisher credential revocation, Worker execution stop, authoritative
backup+reconcile DB rollback policy, no silent provider fallback and no
automatic database downgrade. These are required gates, not evidence that they
have passed. Provider-free dry runs are distinct from real qualification.
Completion still needs one qualified unattended lane, live canary/emergency/
restore acceptance, real lifecycle/recovery measurements, exact delivery,
calibrated SLO targets and independent human Review/Closure. Local protocol
fixtures are not live model evidence; real kernel/PostgreSQL/container tests
cannot promote them into account/provider/device qualification.

## Internal RC boundary and external qualification gates

W01-W10 remain historical accepted repository slices, not current internal
closure. Internal P0 #228 and P1 #229 remain open independently of external
qualification/deployment gates and explicit non-goals. No green CI, binary
provenance envelope or historical milestone can erase these internal blockers.
Do not add parallel authorities, compatibility shims or speculative services.

- **W01 — repository-side complete; production measurement external.** Bounded
  queue-progress windows, canonical Worker poll observations, authenticated
  Publisher endpoint health, and five consecutive 96-Run/8-Worker security
  matrices are required CI. Each matrix also runs three interleaved security
  rounds (48 denied probes), so one exact-head qualification covers 480 Runs and
  240 denied probes. p50/p95/max remain characterization only. The required
  evidence job additionally readbacks GitHub's exact four completed upstream
  job/step timelines and retains a SHA/run-bound, fail-closed **runner-wall
  characterization** report; this is not GitHub billing, utilization or an SLO.
  See [CI job timing characterization](../implementation/CI_JOB_TIMING_CHARACTERIZATION_V1.md).
  Actual deployed capacity, upstream publication health, operating cost and
  calibrated SLO targets require the selected production environment/provider
  and stay under #105.
- **W02 — external provider gate.** Account-free isolated startup and exact
  compatibility identity are implemented. Real auth/model/tool qualification is
  intentionally deferred to #103 or an explicitly qualified #106 provider; no
  silent fallback is permitted.
- **W03 — fail-closed terminal behavior complete.** Catchable preservation,
  actual Worker SIGKILL non-replay/Recovery blocking, ENOSPC fail-closed behavior
  and exact orphan abandonment under Recovery are covered. Automatic replay,
  automatic deletion to repair disk pressure, or guaranteed source capture
  after arbitrary host/storage loss are explicit non-goals; operators reconcile
  the surviving facts instead.
- **W04/W05 — bounded private retention complete for the declared RC scope.**
  Source/result/base graphs, Context, continuation checkpoint, sealed
  ControlTranscript, bounded Codex item history, build outputs, database backup
  and verified scheduled replica retention are covered. Bearer credentials and
  provider-internal session/stream state are intentionally excluded. Off-host/
  second-site placement and encryption/key custody are deployment policy gates;
  destructive GC is explicit operator action, not automatic repair.
- **W06 — repository/service mechanics complete; production host acceptance
  external.** Six-role immutable install/readback, distinct service identities,
  real-systemd ordering/lifecycle, schema startup fencing and cross-version
  binary switch/rollback are required CI. Production-host account/unit
  provisioning, live external-effect acceptance and the frozen canary/
  backup-restore/emergency-stop gates require the selected production
  environment under #105. Binary rollback never downgrades the database.
  **Read-only live-host diagnostics are now implemented but unqualified on an
  actual production host.** `eng production-live-observe` reuses the existing
  immutable host preflight and authenticated Core/Publisher probes and checks
  canonical systemd role PIDs/start-ticks/UIDs/executables/argv with a second identity
  readback. It cannot grant capacity, selected Provider qualification or
  `operational_status_ready`; those require real deployment evidence under
  #105. A separate config gate rejects admission/preparation Workers whose
  otherwise-valid mTLS `CONTROL_ENDPOINT` points outside the deployed
  Control listener. No Worker/model/publisher credential is provisioned or
  disclosed by this diagnostic.
- **W07/W08 — generic platform journey complete; product/hardware integrations
  external.** Authenticated Work -> Task -> Run intake, exact-epoch Human
  Takeover, host-ISA firmware and no-network Cortex-M0 cross-toolchain
  build/capture/restore are covered. WorkBuddy-specific transport/UX and real
  board boot/timing/electrical acceptance require those external systems.
- **W09/W10 — bounded fault/security/governance RC complete.** Required coverage
  includes Worker SIGKILL, ENOSPC, systemd fault/security behavior,
  cross-version rollback, five consecutive admission/security matrices,
  main-only RC delivery envelopes, exact-tree working-ref retirement and
  exact-head drift guards for retained evidence/prototypes. Long-horizon
  production soak/calibrated alerting remains a #105 measurement task.
  Repository-admin ruleset/branch mutation protection for retained historical
  refs requires administrator permissions; the repository-side guard is
  intentionally read-only and must not be mislabeled as admin protection.

Five GitHub issues remain open at this checkpoint: internal P0 **#228**
(effectful Action recovery) and P1 **#229** (remaining caller-context,
commit-ambiguity and maintenance qualification), plus external **#103**
(managed WIF), **#106** (provider relay) and **#105** (production acceptance).
The earlier W01–W10 repository milestones alone do not close newly observed
internal safety defects. Do not treat CI fixtures as external production proof.

Every future code slice still requires exact-head CI, fresh-main and
delivered-byte readback. The internal RC envelope proves only that exact
successful main CI/source/governance state; it does not close provider,
production-host, WorkBuddy, device or independent human acceptance. Production/
unattended readiness remains unclaimed until #105 closes.


Installed-service CI also starts the real installed Control --production and
Publisher entrypoints after original distribution removal. A test administrator
migrates only an isolated synthetic schema; service startup rejects a blank
schema/automatic migration. Actual Worker admission, graceful and idle forced
stops preserve database receipts/audit/outbox/epoch without model replay.
Publisher rejects unsupported arguments instead of silently listening, and its
bound-address announcement is not a provider-ready assertion. See the existing
[operations contract](../implementation/PRODUCTION_OPERATIONS_V1.md).
Quiescent installed-service tests are now complemented by real systemd
dependency/identity tests, immutable cross-version upgrade/rollback and an actual
offline Worker SIGKILL case. The exact expired orphaned execution class now has
explicit non-replay Recovery abandonment, but these tests still do not provision
the final named production accounts on a production host or prove live provider/
publication-effect acceptance.


The native CI additionally runs `scripts/ci-systemd-lifecycle.sh` against a real
system manager. It creates only random transient test names in an ephemeral
runner, retaining the installed Publisher service sandbox/restart/stop settings
while substituting private paths and the non-root test UID/GID. No production
unit is installed or enabled and no host-wide restriction is disabled. Actual
Publisher health, one idle forced-crash restart, three invalid-start attempts
ending at start-limit-hit, and a separate TERM-resistant setsid child fixture
are checked. The last must be killed after the configured stop timeout, remain
a timeout failure and not restart after explicit stop. A missing manager or
noninteractive privilege is a failed required CI gate, not a successful skip.

These remain controlled manager tests, not complete production W06: the
transient graph proves four distinct DynamicUser identities and canonical
dependency ordering; a separate required runner gate creates/executes/fully
removes the exact four canonical named service users and shared group. Production
host account/unit provisioning, database rollback/restore acceptance and live
active external-effect acceptance remain open. The previous direct-process PostgreSQL tests remain independent.
Rate-limit values are an explicit conservative restart policy, not measured SLOs.
Reaching the limit requires diagnosis and an explicit operator restart; a service
restart never grants authority to replay a non-replayable engineering Run.
