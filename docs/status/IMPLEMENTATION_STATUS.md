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
| Stopped source | Private exact-byte source archive after confirmed stop, immutable Core descriptor and new-directory restore | Full model session, automatic replay, successful Delivery or takeover authority |
| Source continuation | Work owner with RunStart + RunControl explicitly creates one fresh successor; old Run is fenced as STOPPED_NO_DELIVERY | In-memory pause/resume, inherited execution permit or model-memory reconstruction |
| Result finalization | Independent trusted-Git checkout must reproduce the recorded complete source/tree before a result bundle is issued | Clean Git status alone proves all source was delivered |
| Provider | Profile v3 binds provider, credential, execution mode, binary and configuration identity | Admission mechanics equal live provider qualification |
| Publisher | Independent mTLS service; production startup rejects in-process publishing | Pilot-only local publisher is a second production lane |
| Distribution | One six-role list in `internal/distribution/binaries.txt`; build, exact-byte verification and Evidence import use it | Package checksums authenticate an untrusted download by themselves |
| Production preflight | v2 checks configuration and actual host facts with expected source SHA | CONFIG_VALIDATED or HOST_VALIDATED means operational READY |
| Operational status | v2 separates database authority and unobserved service readiness; exact response/freshness validation, pending ages and last database progress | No inferred consumer/publisher health, capacity or production readiness |
| SLO | v2 separates unverified summaries from exact, subject-bound source readback | Either report status grants production qualification or calibrated targets |
| Operator CLI | `run-control`, `runtime-isolation-probe`, `source-checkpoint`, `run-continue` and delivery checks | Completed WorkBuddy UX or a Human Takeover interface |

## Actual execution and continuation

[Live controls](../implementation/LIVE_CODEX_CONTROLS_V1.md) persist DISPATCHING
before the provider effect. Ambiguous delivery is not replayed. Input acceptance,
interrupt acknowledgement, matching terminal event and
[kernel-backed stop](../implementation/PROCESS_NAMESPACE_CONTAINMENT_V1.md) are
distinct observations; Core Finish, Publisher and Evidence enforce their binding.
No plain-process or missing-proof fallback is admitted for current execution.

[Stopped source capture](../implementation/STOPPED_SOURCE_CHECKPOINT_V1.md) runs
synchronously on the unsuccessful ExecuteCodex path only after sealed quiescence.
The Preparer rechecks ownership, frozen Task/input/preparation, base/configuration
and approved Context. Modified/untracked/ignored source is preserved within the
bounded archive; missing proof prevents capture. One immutable registration
attempt retains ambiguous readback without replay or promotion to FINISHED.
Core stores a Worker attestation, not a claim it downloaded private host bytes.

Source restore requires externally anchored digest and Run identity, a new
private destination, full byte/mode/link/tree readback and fsync. It never
imports .git/HOME/login state or launches a model. SOURCE_BYTES_RESTORED grants
`execution_authorized=false`.

[Explicit continuation](../implementation/SOURCE_CONTINUATION_V1.md) separately
requires the authenticated Work owner, both capabilities, exact versions/epochs
and checkpoint, requested observed interruption, and no unresolved effects or
prior Delivery. One transaction retires the old Run without success, creates one
new Run/Attempt/Session and emits normal run.started intent. Concurrent decisions
and late action creation share the source fence; exact retry only observes the
original decision. Generic Run creation cannot inject continuation lineage.

A new exact host approval supplies the private archive. The fresh prepared slot
retains the original Git base and binds its restored seed. A NEW model turn gets
explicit source-continuation context, not invented memory. Final diff/bundle must
include both inherited and new representable source changes against the original
Task base. Independent result checkout rejects ignored-file/empty-directory loss
and attribute transformations that contradict the recorded source. It neither
force-adds private files nor deletes them. A failed Finalize can leave a local
commit; reconcile the failed execution instead of blindly retrying it.

## Provider and production acceptance

[Credential lanes](../implementation/CODEX_CREDENTIAL_LANES_V1.md) remain
canonical: `openai-codex / chatgpt-session / trusted-self-hosted` and
`openai-codex / workload-identity / unattended`. No relay identity or silent
credential/provider fallback is admitted. WIF #103 and relay #106 live
qualification remain deferred; additional speculative relay tooling is frozen.

**Production #105 remains open.** Terminal plan v2 requires
`production_host_validated` and `source_verified_slo_evidence_accepted`, not the
old READY/COMPLETE labels. A fresh provider-free v2 dry run remains distinct from
live acceptance; historical PRE_LIVE_COMPLETE v1 evidence is not promoted.
Completion needs an independently qualified unattended provider, real lifecycle
and recovery measurements, exact-head delivery/CI, calibrated SLO targets,
independent human Review and Closure. Namespace/control/continuation integration
uses local protocol fixtures; real PostgreSQL, kernel and container tests do not
turn those fixtures into live model or provider evidence.

## Retained evidence and repository hygiene

[Two historical terminal fact archives](../evidence/m1-terminal-facts/README.md)
retain original selected bytes and source ZIP/artifact/run identities in Git.
Terminal Review uploads allowlisted facts rather than PKI, dumps, environment or
logs. Verification deletes its disposable bootstrap PKI after Control stops;
required intermediate database transport is separate. Historical schemas and
decisions are preserved, not silently admitted as current runtime proof.

These records and private source checkpoints are NOT full runtime/raw-artifact
backups. Model output, binaries, Git bundles and database bytes still require a
complete long-term retention and independently tested restore policy. Private
source/steering must not be automatically published.

[Branch disposition](../reviews/BRANCH_DISPOSITION_2026-10-04.md) distinguishes
integrated working refs from unmatched prototypes and retained M1 subjects.
The maintenance manifest is a bounded candidate list, never a deletion receipt.
Only the executed atomic SHA-guarded maintenance result and remote readback prove
cleanup; zero open PRs does not imply every branch has been retired.

Preserve these historical result subjects unchanged:

- Feature `6009ea95785237ad6ff9f5c9cba911b4891dfa58`,
  `engineering-platform/3ac04fc7097e8375e5f7c1f8`.
- Debug `91d7d9fa068b7667bab5c211f13cd9e0151aeb92`,
  `engineering-platform/994964383ed085e51545105d`.

## Remaining implementation work

Controlled Human Takeover/ownership transfer, WorkBuddy UX, full raw-artifact
retention/restore, unmatched prototype disposition and evidence-ref protection
remain unclosed. Model-memory resume is not provided by source continuation.
Live runtime/provider, production-host lifecycle/SLO and device qualification
remain separate from these internal gaps. Prioritize actual user journeys and
verifiable delivery, not another parallel authority or checklist framework.

## Approved internal RC implementation sequence

W01-W10 remain the approved scope; external accounts, production hosts, devices
and human decisions are separate gates. This is not a new authority framework.

- W01 implemented slice: status v2 no longer claims READY from database counts;
  CLI rederives and freshness-checks every field. Pre-live binds the event SHA,
  actual tree, plan v2 and an exact nonempty no-skip test inventory. Actual
  component heartbeats/capacity and measured progress thresholds remain open.
- W02: real Codex through the actual isolated launch path without account/model
  calls; W03: preservation/reconciliation for post-turn failures.
- W04/W05/W06: source/build-output boundary, complete private raw-artifact
  retention and fresh-host install/upgrade/restore.
- W07/W08: normal authorized user journey and representative board-free embedded
  build/evidence scenarios. No tenant or hardware integration is implied.
- W09/W10: fault/load/security regression and exact RC delivery/ref governance.

Implemented code is not completion evidence. Each slice requires exact-head CI,
fresh-main readback and delivered artifacts; no earlier green run qualifies new
source. Historical M1 remains immutable, and production #105 remains open.

## W02 account-free isolated startup qualification

Current qualification schema 3/compatibility contract 2 launches the exact native
Codex binary in the same user/PID namespace mechanism as engineering, with the
exact workspace-write config and fresh credential-free HOME. It sends only
initialize/initialized/thread-start, then requires actual init reaping. It does
not warm credentials or start a model turn. Missing isolation/config/reap checks
cannot fall back to ordinary process startup or old schema qualification.

The receipt binds the stable isolation mechanism, configuration and a bounded
kernel/architecture/UID/GID environment digest. PIDs/inodes/timestamps are not
stable qualification identity; each actual Worker CLI execution still repeats
qualification and compares the entire receipt. CI repeats the real native probe
twice to test deterministic identity. This is not live model/tool/provider
qualification, a resource sandbox, or production approval. Regenerate current
qualification/Profile identity; do not rewrite historical M1 records.

## W03 catchable post-turn failures

The actual Worker now records bounded immutable local phase entries before
post-turn persistence, Finalize and report work. Catchable failures after a
completed/quiescent turn retain a private source checkpoint and attempt one
immutable Core registration. A failed Finalize's one direct local child commit
may be inspected only for source preservation; normal execution/reopen keeps its
original-base fence. No successful path gets an unsolicited extra source copy.
A completed turn's checkpoint remains ineligible for interruption-only source
continuation. A lost response may follow an already committed Core receipt;
local failure observations never override FINISHED or authorize model replay.

These records are not new authority or automatic crash recovery. Abrupt host/
Worker death, disk exhaustion that prevents all writes, comprehensive raw model/
Git/binary/database retention and automatic repair remain separate unclosed work.


## W03 read-only execution readback

`eng execution-readback` verifies the actual existing private permit/phase/turn/
result/checkpoint records without modifying them or granting any effect. Explicit
artifact paths opt into exact source or bundle byte checks; stored metadata paths
are never followed. The offline default does not contact Core. Optional `--core`
performs one existing authenticated GET and independently checks token, sealed
transcript, expected result and checkpoint identity. A local failure does not
replace an already-committed Core FINISHED receipt, and a Core descriptor does
not prove private bytes were read. The report exposes no model/source/steering
bodies and never authorizes execution, replay or production qualification.

This is bounded consistency/observation tooling for the W03 failure path, not
complete crash recovery, full raw-artifact retention, a second state authority,
or an automatic repair decision. W01 service observation/capacity, abrupt-crash
recovery and the remaining W04–W10 RC work packages are still not closed.
