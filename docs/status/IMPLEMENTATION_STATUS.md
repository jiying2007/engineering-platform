# Implementation Status

This is the single live implementation-status authority. Detailed contracts are
linked below; historical checkpoints and receipts remain in Git/PR history, not
parallel live checklists. Audit baseline: `e4ca13e254c883d336ec990e1767f8cc85b8b98b`.

## Maturity boundary

**Trusted self-hosted M1 phase 1 has historical proof. Current repository
implementation and automated tests are not production/unattended qualification.**
The default internal lane remains trusted Ubuntu with a saved ChatGPT Codex
session. This project is independent of digital-worker and inherits no old
schema/runtime compatibility obligation.

[The fixed M1 proof](M1_RETAINED_PHASE1_CLOSURE_2026-09-29.md) records real Feature
#54 and Debug #55 closure. It does not qualify later runtime changes. Do not rerun
those subjects or rewrite their receipts to manufacture current production proof.

## Current contracts and limits

| Area | Implemented | Not implied |
| --- | --- | --- |
| Core | PostgreSQL business/audit/outbox, frozen Task/RunInput, execution and Recovery epochs | Production service operating acceptance |
| Live controls | mTLS RunControl, durable one-shot steering/interrupt, exact actor/sequence/epoch/turn binding and sealed transcript | ACK means stopped; accepted input means objective fulfilled |
| Process lifetime | Dedicated Linux user/PID namespace; receipt v4 and transcript v2 bind kernel-observed init reap | Complete filesystem/resource isolation or control of unrelated host processes |
| Stopped source | Private exact-byte archive after confirmed stop, immutable Core descriptor and new-directory restore | Full model session, automatic replay, successful Delivery or takeover authority |
| Source continuation | Work owner with RunStart + RunControl explicitly creates one fresh successor; old Run fenced as STOPPED_NO_DELIVERY | In-memory pause/resume, inherited execution permit or model-memory reconstruction |
| Finalization | Independent trusted-Git checkout reproduces complete source/tree before bundle publication | Clean Git status alone proves all source was delivered |
| Post-turn failures | Immutable fsynced phase records; catchable failures preserve stopped source; read-only local/Core reconciliation; actual offline Worker SIGKILL after permit/renewal remains unresolved, unreplayed and blocks Recovery proof | Automatic source capture after arbitrary Codex/host death, approved repair or replay |
| Offline build outputs | Frozen output names/budgets, guarded tmpfs collection after child reap, existing local/Core receipt and Evidence byte checks; actual C compiler and freestanding host-ISA firmware ELF/map reference | Writable host builds, large production firmware, MCU cross-compilation, boot/timing/board qualification or unattended retention scheduling |
| Raw artifact sets | Explicit plans or producer-derived execution/Context/offline-output selection, byte packing/verification and fresh-directory restore; native PostgreSQL dump/restore drill; required ENOSPC pack/restore fail-closed test | Complete per-Run dependency coverage, second-site backup, retention scheduling/encryption policy or automatic disk-exhaustion repair |
| Provider | Profile v3 binds provider, credential, execution mode, binary and configuration identity | Admission mechanics equal live provider qualification |
| Compatibility | Schema3/contract2 probes actual Codex under engineering namespace/config; repeatable environment identity | Authentication, model/tool execution or production qualification |
| Publisher | Independent mTLS service; production startup rejects in-process publishing; bounded authenticated /healthz observation is bound to exact remote configuration digest | Endpoint health is upstream GitHub/provider health, capacity, publication authority or production readiness |
| Distribution | One six-role list; source-matched immutable install/readback; previous-version readback by retained source+manifest identity; explicit stopped-release switch/rollback keeps replaced bytes; real cross-version upgrade/rollback drill | Service activation, database downgrade/migration rollback, restricted-/proc bypass, in-flight external-effect recovery or checksum-only authenticity |
| Service manager | Explicit restart/cgroup stop policy; installed-service lifecycle under real systemd; transient four-role Publisher -> Control -> admission/preparation graph uses distinct DynamicUser identities and exact Wants/Requires/After readiness ordering; required CI also creates the canonical four named Unix users + shared group on a disposable runner, executes one constrained transient unit per identity, then proves full account/group cleanup | Actual production-host account provisioning/unit installation, active publication/model crash recovery or replay safety |
| Production preflight | v2 checks configuration and actual host facts with expected source SHA, service-user primary/supplementary groups, parent traversal/read-write mode access and hard-link rejection | CONFIG_VALIDATED or HOST_VALIDATED means operational READY; ACL-only grants are not inferred |
| Operational status | v3 separates database authority from unobserved readiness; adds bounded per-profile Worker poll facts from existing last_seen_at, exact freshness checks and queue-progress windows; Publisher endpoint health is a separate authenticated observation; required Go/PostgreSQL CI also runs a bounded 32-Run/8-Worker concurrent admission/security characterization | Worker identity count/recent poll, measured CI latency or Publisher endpoint reachability proves calibrated capacity, upstream health or production readiness |
| SLO | v2 separates unverified summaries from subject-bound source readback | Calibrated targets or production qualification |
| Pre-live | Event SHA/tree, terminal plan v2, exact nonempty no-skip test inventory and an internal RC delivery envelope bound to exact successful main-push CI/source tree/governance bytes; main-only CI emits it as a separate retained artifact | Live provider/service acceptance, independent human Review or production qualification |
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
crash. Separately, required native CI now kills an actual installed offline
Worker with SIGKILL after its permit is fsynced and its lease renewed: Core stays
non-terminal without a receipt, a second Worker cannot replay the same
execution, and Recovery proof remains blocked until reconciliation.

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
source/result bytes rather than relying on a hand-written list. It rejects
missing/extra Context and unbound artifacts. It does not capture the original Git
base, toolchain, full control history or upstream continuation dependencies.
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
ELF/map reference and private artifact restore are covered, but no target MCU
cross-toolchain, boot/timing/hardware behavior or larger production firmware
capacity is qualified. A same-Run compiled Worker/mTLS/PostgreSQL/native
C build additionally captures and restores its original records and raw outputs
after deleting source/Git/preparation roots, without creating Evidence or new
execution. Producer-derived capture tests remove the entire prepared root
including Context, restore, and recheck every frozen input object and producer
record. Original Git base/toolchain, full model/tool/control history, second-site
retention, encryption/key policy, scheduling and GC remain open. A required
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
Four older divergent prototypes are explicitly disposed as frozen
RETAINED_SUPERSEDED_PROTOTYPE refs. Required CI pins their exact remote heads;
they remain source-history lineage and are not treated as byte-identical or
pending product work. Preserve unchanged:

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

**Production #105 stays open.** Terminal plan v2 requires host validation and
accepted source-verified SLO evidence, not old READY/COMPLETE labels. Provider-free
dry runs are distinct from real qualification. Completion still needs one
qualified unattended lane, real lifecycle/recovery measurements, exact delivery,
calibrated SLO targets and independent human Review/Closure. Local protocol
fixtures are not live model evidence; real kernel/PostgreSQL/container tests
cannot promote them into account/provider/device qualification.

## Remaining approved RC work

W01-W10 remain the approved scope, not a new authority/checklist framework.

- W01: bounded queue-progress windows, canonical Worker poll observations and
  authenticated Publisher endpoint health are implemented without promoting
  readiness. Actual Worker/Publisher capacity, upstream publication health and
  calibrated operating SLOs remain open.
- W02: account-free isolated startup implemented; actual auth/model/tool and
  deployment-environment acceptance remain external gates.
- W03: catchable preservation/readback, actual offline Worker SIGKILL
  non-replay/Recovery blocking, and artifact ENOSPC fail-closed behavior are
  covered. Arbitrary Codex/host crash source capture, disk-space repair and
  approved reconciliation/repair remain open.
- W04/W05: bounded offline build-output contracts and raw-set/context capture
  implemented; larger build profiles, unattended retention scheduling, full Run
  dependencies and durable private retention/production-native acceptance remain open.
- W06: source-bound six-role install/readback, real transient four-role systemd
  ordering/identity separation and immutable cross-version binary switch/rollback
  are implemented. Disposable fresh-host CI now proves the canonical named Unix
  identity lifecycle without persisting accounts or installing production units.
  Actual production-host account/unit provisioning, database migration rollback
  policy and active external-effect recovery remain open. Installation/switch never grants execution or downgrades the database.
- W07/W08: authenticated Work -> Task -> Run intake and exact-epoch Human
  Takeover are implemented using existing Core APIs; a board-free freestanding
  firmware reference is in required native CI. WorkBuddy transport/UX and real
  target cross-compilation/board acceptance remain open.
- W09/W10: actual Worker SIGKILL, ENOSPC, service-manager fault/security,
  cross-version rollback and a bounded real PostgreSQL/mTLS 32-Run/8-Worker
  admission/security matrix are implemented. The load matrix asserts exact
  once-only admission and authorization invariants while reporting p50/p95/max
  only as characterization, not SLOs. The expanded exact-tree retirement batch
  removed 32 refs with five already absent, and future exact-tree same-repository
  merged heads are leased for automatic cleanup after successful main CI.
  Main-only internal RC delivery-envelope generation is implemented and each
  main push must still actually emit/read back its source-bound artifact. Broader
  sustained/soak security coverage and repository-administrator mutation
  protection for retained historical refs remain open. The four divergent
  prototype refs have an explicit frozen-history disposition and required
  repository-side drift guard.

Every slice needs exact-head CI, fresh-main and delivered-byte readback. The
internal RC envelope is provenance for that exact successful main CI only; it
does not close external terminal gates. Earlier green runs do not qualify new
source. External accounts, production hosts,
devices and human decisions remain separate gates; no local implementation can
stand in for those decisions. Prioritize actual user journeys and reproducible
delivery rather than parallel authorities, speculative tooling or more states.


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
offline Worker SIGKILL case. They still do not provision the final named
production accounts on a fresh host or prove active model/publication effect
recovery.


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
host account/unit provisioning, database migration rollback/restore acceptance
and active external-effect recovery remain open. The previous direct-process PostgreSQL tests remain independent.
Rate-limit values are an explicit conservative restart policy, not measured SLOs.
Reaching the limit requires diagnosis and an explicit operator restart; a service
restart never grants authority to replay a non-replayable engineering Run.
