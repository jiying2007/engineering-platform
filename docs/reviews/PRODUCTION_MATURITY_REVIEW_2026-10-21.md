# Production maturity review — 2026-10-21

Date: **2026-10-21 (Asia/Shanghai)**. Reviewed repository:
`jiying2007/engineering-platform`. Exact protected-main source baseline:
`a0628df29439b4980a9c93ba2d840fa9b8d8319a`; tree:
`6f1f8e055ae381616c985d2cfda8a6eef00f472a`.

This is a historical audit record and implementation input, not another live
plan, readiness ledger, qualification receipt or independent human Review.
[Implementation Status](../status/IMPLEMENTATION_STATUS.md) remains the single
live readiness authority. The associated PR and retained CI artifacts must
identify the final changed source, exact-head CI, protected merge, fresh-main CI
and actual delivered bytes. This record does not predict those results.

## Finding and evidence boundary

The reviewed baseline contains substantial source-bound delivery, recovery,
authorization and isolated-environment tests. It also contains reproducible
Recovery proof and Runtime subprocess defects, CI verification/retention
gaps, and unfinished production-observation and acceptance capabilities.
Production qualification is therefore not merely waiting for credentials and
a human signature.

The investigation used isolated PostgreSQL data and synthetic local subprocess
fixtures. No actual provider/model call, credential qualification, production
deployment, paid resource or external publication was performed by these
reproductions. In particular, a synthetic protocol success does not establish a
successful real Codex/provider turn.

The following findings refer to the exact baseline above. Source repairs are
included in this audit change. A corrected implementation and regression
result must be read from the final change and its retained CI; neither a repair
description nor this audit document is proof of completed delivery.

## Reproduced source defects

| ID / priority | Baseline implementation | Observed counterexample | Required correction and retained regression |
| --- | --- | --- | --- |
| PM01 — P1 | `internal/store/postgres/recovery_proof.go::readRecoveryProof` reads only `proof_json`; `internal/store/postgres/store.go::CompleteRecoveryContext` independently decodes that JSON. `internal/recovery/proof.go::Validate` checks internal shape/digests but cannot establish agreement with the original SQL row and causal audit. | In an isolated database, changing only retained JSON `reconciler`, `recovery_epoch`, or `facts` to `null` was accepted by proof GET. Completion authorization did not reject the projection-integrity error; changing `reconciler` also let the original reconciler pass that separation check. Completion accepted each corrupted proof, changed Recovery to `NORMAL`, and appended a completion audit. | Reuse one strict original row/JSON/causal-audit projection across GET, authorization and completion, including requested epoch, original actor, full facts shape and the original audit binding. Reject corruption without changing Recovery or appending completion evidence. Preserve valid historical/restore readback and distinct completion authority. |
| PM02 — P1 | `internal/runtime/codexapp/qualification.go::codexVersion` and `codexCommand` use subprocess output collection whose join is not bounded by the caller once an exited parent leaves a child holding its pipes. | For both helpers, a 75 ms caller deadline returned only after about 1.003 s with a nil error while the Context had expired. The fixture child exited itself after one second. | Bound the whole subprocess and pipe lifetime, not only initial process cancellation; preserve cancellation as failure, reap owned descendants, and add a child-holds-pipe negative regression. |
| PM03 — P1 | The same qualification helpers collect output without a suitable retained-output ceiling. | Both helpers accepted 4,194,304 bytes of synthetic output without a size error. This demonstrates the absent bound, not an observed production OOM. | Use explicit bounded stdout/stderr or combined-output handling, reject overflow and complete owned-process cleanup. Keep required valid protocol/version output intact. |
| PM04 — P1 | `internal/runtime/codexapp/live.go::LiveSavedLoginProbe` waits for the app-server parent but does not require an owned process-namespace quiescence proof before returning successful qualification. | A synthetic valid `LiveReceipt` returned after about 63 ms; a detached descendant then wrote a marker after a 0.7 s delay. The helper was finite and investigator-owned. | Reuse the existing `internal/processscope` lifetime proof. A successful receipt must follow successful bounded termination/reaping of the owned execution scope; a parent exit or final protocol event alone is insufficient. |
| PM05 — P1 | `internal/runtime/codexapp/live.go` and `engineering.go` execute the version probe before the pinned provider path rejects changed executable bytes. | With an intentionally wrong expected digest, both saved-login entry points ran a marker-writing `--version` script before returning `qualified executable bytes changed`. | Validate canonical executable path, regular-file/executable permissions and the expected byte digest before executing even a version probe. Cover the live and engineering entry points, retain later execution pinning, and do not claim this establishes host filesystem read isolation. |

PM01 requires corruption or tampering of the persisted proof projection; the
experiment is not evidence of an unauthenticated database-write or SQL-injection
path. Its significance is that the promised retained-proof integrity and
independent-completer boundary failed once that projection differed from its
original authority. Repair must reject the damaged record rather than rewrite
historical proof to make it pass.

The Runtime experiments intentionally asserted the vulnerable observations.
Their test-run `PASS` means that a counterexample was reproduced; it is not a
security or production acceptance result. All synthetic helpers were finite
(at most one second for the reproducing descendants), had no real credentials
or network/provider work, and were contained and cleaned up by the experiment's
owned process group. The proposed permanent regressions belong in
`internal/runtime/codexapp/probe_process_test.go` and
`internal/runtime/codexapp/live_process_regression_test.go`; final test names
and outcomes must be checked in the delivered revision.

### Implemented repair and regression scope

The shared Recovery reader now fetches the retained JSON, independent row fields,
preceding audit head identity and the original proof audit at exactly head + 1
in one SQL snapshot. It requires the full strict facts shape, requested epoch,
actor, original timestamp and reconstructed original audit digest. GET,
authorization and serializable completion use that same reader. A later duplicate
event cannot substitute for the original proof audit. This is a fixed-position
causal projection check, not whole-history uniqueness or protection against an
administrator rewriting every authority together. No migration or historical
proof rewrite is introduced.

Cross-review found that decoding the entire preceding audit with a new 16 KiB
ceiling would reject a valid historical audit with a long aggregate ID. The
repair instead reads only its existing sequence/digest identity, while still
fully validating the original proof event. A positive 20 KiB aggregate-ID
regression preserves that existing compatibility boundary; restore regressions
also reject a retained proof transplanted from another recovery epoch.

Runtime helpers now use the existing user/PID namespace implementation. A fixed
positional-argument shell gate lets the parent observe namespace identity before
the validated target executes. Every helper launch receives and rechecks the
expected executable digest, including launches after provider construction.
The version helper has a five-second execution budget with 8 KiB stdout and
64 KiB stderr limits; schema/features helpers have a twenty-five-second budget
and a 1 MiB combined output ceiling. The shared path keeps a one-second pipe
wait ceiling and an independent five-second stop/reap budget. Overflow,
cancellation and unconfirmed stop remain failures with their original causes.

Both live lanes require successful exit and observed namespace quiescence before
returning a receipt; cancelled or failed cleanup yields no receipt. Regression
fixtures cover detached `setsid`/double-fork descendants, inherited pipes, fast
parent exit, both output streams, path/mode/digest rejection and cancellation.
The CLI profile path reuses the same verifier. Real local namespace/race and
isolated PostgreSQL restore tests exercised these changes without provider or
model calls; their logs belong in the associated delivery evidence.

LiveReceipt remains schema 2 and does not embed a process-scope proof. The new
guarantee belongs to this producer's execution before it returns; it does not
retroactively qualify older schema-2 receipts. Filesystem read isolation,
unrelated host process control and remote provider cancellation remain outside
the guarantee. The Context inventory includes the one new bounded cleanup
origin and continues to declare that #229 is not closed.

### Follow-up: fresh-main startup-gate ownership regression

The seven baseline findings PM01–PM07 were integrated by
[PR #288](https://github.com/jiying2007/engineering-platform/pull/288).
Its exact head `70d99f74738e458a91a389f18efdf7088275600a` passed all five
required jobs in [CI 39612489812](https://github.com/jiying2007/engineering-platform/actions/runs/39612489812).
The protected squash merge produced
`e10a8025d0c78ba404b2b16822fec60822cda90b`, but its fresh-main
[CI 39615422618](https://github.com/jiying2007/engineering-platform/actions/runs/39615422618)
failed `TestBuildCodexProfileBindsExactBinaryModelAndPolicyGrant` with
`codex version: close |1: file already closed`. That failed run is retained;
the earlier PR success did not establish completed main delivery.

This was a regression introduced in the startup gate repair, not an eighth
finding against the original `a0628df` baseline. `exec.Cmd.StdinPipe` registers
its writer for automatic closure by `Wait`. A valid fast-exit target can finish
before the caller explicitly closes the gate writer, so those two owners race
and reject a successful version probe. A deterministic regression uses the
same production `probeStdinPipe` helper and requires process `Wait` to finish
before writer `Close`; the original helper fails with the same closed-file
error without sleeps or a global scheduling hook.

The correction creates a caller-owned `os.Pipe` and passes its read end as
`cmd.Stdin`. After `Start`, the parent closes its read copy, releases the gate
and explicitly closes its write end. `exec.Cmd.Wait` owns neither caller-held
descriptor. Deferred closure covers failed starts and early returns; real
write/close errors remain failures, and existing bounded namespace stop/reap
handling remains in force. A second regression runs the production probe with
a target that waits for EOF, detecting a leaked writer or retained gate input.
The final follow-up change still requires its own exact-head and fresh-main
gates and actual artifact readback; this source description predicts none of
those results and changes no production qualification state.

### Investigation record identities

These hashes identify the local investigation records inspected during this
audit. They are not GitHub artifact identities or production receipts. The
durable, repeatable acceptance evidence must be the delivered regression source
and the associated completed CI artifacts.

| Investigation record | SHA-256 |
| --- | --- |
| Recovery baseline `baseline-proof-integrity.log`, including `TestPostgresRecoveryProofRejectsInconsistentRetainedJSON/{reconciler,epoch,null_facts}` | `9ab1eb6284dbd339d5a9775d4aa3d6d31588d8d1b430f6638d1caba49d90a70a` |
| Runtime baseline `repro.log`, four top-level reproductions | `b274513ce264899a14044b46caaed6c75699c014615e926d26d1b638c5b90b5c` |
| Runtime synthetic reproduction source `audit_repro_test.go` | `b9177394af5df8cd21d0b43e014948368423190d1de3e41e20b0d6b85d230e38` |

## CI verification and retained-source gaps

| ID / priority | Baseline evidence | Change under review | What still has to be verified |
| --- | --- | --- | --- |
| PM06 — P1 | `.github/workflows/ci.yml` executes the downloaded candidate `eng distribution-verify`. A candidate executable therefore participates in verifying its own distribution. `internal/distribution/contract.go::Verify` checks agreement among build revisions but the workflow did not independently require its result to equal the exact expected CI source SHA. | Invoke `go run ./cmd/eng distribution-verify` from the checked-out source, first assert checkout HEAD equals `SOURCE_SHA`, and require `BYTES_VERIFIED`, exact `source_commit == SOURCE_SHA`, and the six canonical roles. | Exact changed workflow execution, negative identity coverage, final main bytes and the original authenticated artifact identities. Go build metadata and hashes do not independently prove a trustworthy compiler/runner or defeat arbitrary modification of that whole trust domain. |
| PM07 — P2 | `scripts/ci_admission_characterization.py` reports `input_sha256`; `scripts/ci_job_timing.py` reports `raw_jobs_sha256`. The baseline workflow uploads only the summaries, leaving `admission-matrix-events.jsonl` and `jobs.json` in runner temporary storage. A later reader cannot recompute those exact input hashes from the retained summary alone. | Retain each report and its actual raw input together under the existing artifact name and retention policy. Preserve exact source/run/attempt fields and the false production/SLO qualification flags. | Download the final artifacts, require both expected members, verify the raw digest, and deterministically rerun the corresponding `build_report` against the same source/run/attempt. A newly fetched GitHub jobs response is not the retained original snapshot. |

The CI diff was independently read against
`internal/distribution/contract.go`, `cmd/eng/distribution.go`, and both existing
characterization parsers. The source verifier reads candidate bytes and Go build
metadata without invoking a candidate executable. The shell step already uses
`set -euo pipefail`, so a failing verifier or JSON predicate must fail the step.
The new uploaded raw paths are the same paths consumed by the parsers, and the
change introduces no new token permission, service or Evidence authority.
No additional blocking defect was found in this small diff by read-only review.
That conclusion does not replace execution of the changed workflow and
independent archive-member/readback verification.

## Supplemental PM08 — P1 terminal fixture contract enforcement

Continued review at `e10a8025d0c78ba404b2b16822fec60822cda90b` found three
bounded enforcement gaps in the already frozen terminal maintenance contract.
The relevant API test, Git importer, CI importer and Task-template Git blobs
are unchanged from the original `a0628df` audit baseline.

| Existing contract | Counterexample or source gap | Bounded correction |
| --- | --- | --- |
| The only terminal source mutation changes the exact marker from `PRE_LIVE_READY\n` to `TERMINAL_QUALIFIED\n`, followed by exact PR-head CI. | `TestProductionTerminalMaintenanceTemplatesDryRunToRunning` only accepts the initial marker. A private fixture with the legitimate final bytes fails that original test, making canonical CI reject the intended terminal result. | The general template test accepts both exact valid source states; permanent private-fixture regressions run the full template dry run in each state. The dedicated pre-live workflow still requires the initial state. |
| The result changes no other file and preserves the exact marker bytes. | The generic Git manifest binds whole base/result snapshots but its import entry did not enforce the terminal-only change constraint. It cannot establish that constraint merely by naming `git.changed-tree.v1`. | Reuse the existing isolated Git snapshots, compare every NUL-delimited leaf's path/mode/type/object ID, and read the marker blobs through controlled Git. The existing CLI offers `git-change-manifest --terminal`, while import selects the constraint automatically from the authenticated Delivery's terminal TargetID. Omitting the optional capture flag cannot bypass import checks. |
| Terminal evidence must be for the exact PR head. | A valid retained main-push CI fixture with the terminal Delivery TargetID produced PASS Evidence in the baseline importer. | Terminal imports require the original receipt's PR event, then retain all existing live repository, event, head/base, workflow, job and archive checks. Ordinary deliveries continue to support valid main-push evidence. |

The authoritative Delivery inherits its TargetID from the frozen Task in Core;
the delivery-create request does not choose a replacement target. The Git
checker does not treat a caller-supplied flag or local remote URL as authority.
It does not independently authenticate repository ownership: the original
frozen Task/Core chain and trusted CI repository checks retain that role.
The manifest stays schema 1, the terminal plan stays v3, and the actual repository
marker remains at its initial bytes. No model turn, publication, terminal
qualification, new Evidence procedure or new authority ledger is performed.

These corrections enforce existing source/CI constraints. They do not complete
the missing fourteen-gate operational mapping described below. Their final
delivery requires the follow-up PR, fresh-main gates and actual artifact
readback, with the failed first main run preserved separately.

## Supplemental PM09 — P2 bounded CI fact visibility

The first head of PR #289,
`13645fc381c078714edd8752cc590bd0e80244b5`, passed all four upstream jobs in
[CI 39620793853](https://github.com/jiying2007/engineering-platform/actions/runs/39620793853).
Its dependent `trusted-ci-artifact-evidence` job failed the `Collect exact Git,
CI and artifact facts` shell step with exit 1 in less than one second. The PR
was not merged on those four successful jobs. The failed run and its decoded
logs remain separate from any later successful delivery.

The collection segment is byte-identical at the original `a0628df` baseline,
PR #288's merged `e10a802`, and this first PR #289 head: it fetches jobs and
artifacts once, then immediately requires four successful job conclusions.
Independent API observations showed completed step lists while the job's
overall state still read `in_progress`. Together with the short silent failure,
this supports a visibility-delay hypothesis. The workflow did not retain the
failed original `jobs.json`, and its log does not identify each executed shell
command, so the precise failed predicate and original response are not proved.
Do not rewrite this inference as an observed GitHub service defect or SLA.

The correction uses a bounded read-only collector for the original run,
source and attempt. A two-minute total budget and five-second retry interval
apply only to incomplete/nonterminal facts; actual failed conclusions,
identity drift, ambiguous duplicates and malformed input remain failures.
The existing timing parser validates the completed jobs' original bytes.
The two pinned producer upload steps pass their actual artifact IDs through
job outputs; independently retrieved metadata must match those IDs, exact
names, run, source, digest and size. Artifact metadata has no invented
`run_attempt` field, and dependency results never substitute for observed
successful job facts.

Successful collection keeps the original `jobs.json`, `artifacts.json`,
`run.json` and bounded collection diagnostic beside the timing report in its
existing archive. A failure-only diagnostic archive retains the last available
responses. The existing CI receipt schema, successful archive names, required
jobs, token permissions and five-minute evidence-job ceiling remain unchanged.
The wait budget is a local safety choice, not an assurance that an upstream
service converges within that period. Missing facts at its deadline fail closed.
This source correction requires a new exact-head run, protected merge,
fresh-main run and actual retained-byte readback before delivery is claimed.

## Repository capabilities still missing

These are capability findings, distinct from PM01–PM09 and from the evidence
that only a real selected deployment can provide. They remain within existing
issues and contracts, not a parallel implementation roadmap.

| Priority / capability | Exact source or contract | Existing tracking and boundary |
| --- | --- | --- |
| P1 — authenticated operational readiness and capacity | `internal/production/status.go::EvaluateSnapshot` derives `service_readiness=NOT_OBSERVED`, `ready=false` and `service_health_and_capacity_not_observed`. `ValidateAt` rejects a caller-modified result. `internal/production/live_services.go` and `docs/implementation/PRODUCTION_OPERATIONS_V1.md` keep the live wiring diagnostic separate from the final `operational_status_ready` gate. | [#105](https://github.com/jiying2007/engineering-platform/issues/105), acceptance areas 3, 4 and 6. This deliberate fail-closed state must remain until an authenticated, fresh, same-deployment observation contract and actual acceptance exist. Starting units or toggling JSON cannot complete it. |
| P1 — terminal gate and case evidence coverage | `internal/production/acceptance.go` freezes terminal plan v3 with three required Evidence procedures and fourteen gates. `cmd/eng/production_acceptance.go` generates the plan and SLO report; it is not a complete live-gate verifier. `docs/status/TERMINAL_BLOCKER_EVIDENCE_CONTRACT.md` explicitly identifies its proposed receipt as documentation, not a deployed parser. | [#105](https://github.com/jiying2007/engineering-platform/issues/105), area 7, owns terminal production acceptance; [#228](https://github.com/jiying2007/engineering-platform/issues/228) and [#229](https://github.com/jiying2007/engineering-platform/issues/229) retain their separate case requirements. A future checker must validate the existing frozen plan and original Core objects; this audit does not add a new contract or qualifying boolean. |
| P1 — operating SLI denominators and sustained resource evidence | `internal/production/acceptance.go::BuildSLOReport` accepts bounded positive latency samples and reports min/p50/p95/max. `internal/production/slo_sources.go` binds source bytes; it does not authenticate the collector or create production targets. The blocker contract already requires failures/throughput, p99, resource trends and a declared soak. | [#229](https://github.com/jiying2007/engineering-platform/issues/229) owns selected-host lifecycle, latency and long-run evidence; [#105](https://github.com/jiying2007/engineering-platform/issues/105), area 6, owns production SLO/capacity acceptance. Existing summaries should retain their limited scope. |
| P1 — continuous operational alerting and monitor freshness | `docs/implementation/PRODUCTION_OPERATIONS_V1.md` documents a bounded `production-status --observe-for` diagnostic and `--require-no-alert` exit behavior. Those commands alone do not establish continuous observation, delivery of an actionable alert or detection that the observer itself has stopped. | [#105](https://github.com/jiying2007/engineering-platform/issues/105), area 4. Define and prove the required alert/observer lifecycle for the selected deployment; do not automatically install an entire observability stack. |

This assignment is already explicit in existing #105 discussion:

- [2026-10-08 design audit](https://github.com/jiying2007/engineering-platform/issues/105#issuecomment-6062648223)
  identifies the READY implementability gap and forbids promotion by credentials,
  `/healthz`, synthetic JSON or a Boolean override.
- [2026-10-09 live observation scope](https://github.com/jiying2007/engineering-platform/issues/105#issuecomment-6073445837)
  records the narrower source/systemd/mTLS diagnostic and keeps live capacity,
  provider qualification, steady-state evidence and final aggregation open.

Those existing records are the appropriate place to retain the remaining
readiness and terminal-parser work. #229 is not a replacement production
acceptance issue; its caller-context, maintenance, PostgreSQL and selected-host
qualification scope remains distinct.

## Constraints on a future coverage checker

First validate the original `BuildTerminalPlan`/`TerminalPlan.Validate` rules and
the plan digest. Preserve the single maintenance fixture, the exact three
required Evidence procedures, fourteen gates, one-model-turn ceiling, human
Review requirement, no provider fallback and no automatic database downgrade.
Do not quietly replace them with a new checklist or a second authority model.

Reuse the original Core `DeliveryReceipt`, `EvidenceRef`, frozen Verification
Plan, Verification, Review and Closure records, including their authenticated
issuer, procedure, run, subject and artifact relationships. A diagnostic may
report missing or inconsistent coverage, but locally asserted gate booleans,
self-consistent hashes or reviewer-name strings are not authoritative evidence.
Existing Core identity and role checks remain necessary.

`verification_pass`, `independent_human_review_pass` and `closure_created` must
refer to existing persisted Core objects, not be self-issued evidence used to
create those same objects. Operational and SLO acceptance must have a clear
relationship to the independently reviewed subject. Preserve already frozen
Delivery/Task/Plan bytes; any future contract change requires the existing
explicit freeze/version process and cannot rewrite historical qualification.
This design constraint is not an implemented checker or a production PASS.

The terminal v3 gate inventory is not a complete gate-to-evidence mapping.
The frozen maintenance Task contains only `terminal-ci`, `terminal-codex` and
`terminal-git`; Core rejects requirements outside that frozen Verification Plan.
Existing authenticated importers, Core Verification/Review/Closure checks and
retained M1 archive readback already cover substantial parts of that chain and
must be reused. The archive reader is limited to historical Feature/Debug
subjects. These checks do not define the missing operational gate procedures,
issuers, deployment bindings or per-case acceptance rules. The PM08 follow-up
adds the terminal PR-event restriction to the existing CI importer; a future
coverage readback must still inspect its bound original event facts, rather
than infer the event from an Evidence procedure name alone.

The fixture already retains operational and recovery/SLO evidence outside the
product Delivery subject. An SLO source contains its measured `subject_digest`;
putting that source's hash into the same Delivery whose digest it names would
create a digest cycle. This is a constraint on future integration, not an
observed execution defect. Preserve the original product subject and freeze a
non-circular relationship to the operational subject and its existing Core
Verification/Review records. Source-byte verification alone cannot supply an
authenticated operational requirement or accepted operating targets.

The documented live sequence places lifecycle and SLO acceptance after the
maintenance Closure, while Core requires Work to be `VERIFYING` before Review
and atomically moves it through `REVIEWING` to `CLOSED`. A completed maintenance
Closure cannot retroactively attest later observations. The operational
evidence and its independent acceptance need an explicit frozen subject and
sequence before final read-only aggregation can establish complete coverage.
This remaining contract and implementation work belongs to #105, with #228/#229
boundaries intact; credentials alone do not resolve it.

## External evidence and release disposition

The following cannot be established by these source repairs or hosted CI:

- Original provider/Publisher process quiescence and exact positive/negative
  external effect readback for the remaining effectful #228 cases.
- Selected-host filesystem read denial, secret and network boundaries,
  actual service lifecycle, durable storage behavior, capacity and long soak.
- Qualified unattended provider identity, real emergency provider disablement,
  Publisher credential revocation and the frozen production maintenance canary.
- Accepted backup/restore policy and measured RPO/RTO, including the selected
  backup medium, key/configuration custody, off-host restoration and subsequent
  external-operation reconciliation.
- Independent human acceptance of the complete production evidence.

Maintain **`BLOCKED [228,229]`** and keep production **#105**, optional WIF
**#103** and optional relay **#106** qualification separate. A source repair is
delivered only after its actual exact-head and fresh-main gates and retained
bytes have been checked; production qualification requires the additional
applicable real-environment evidence and independent acceptance. The
[primary-source comparison](../research/PRODUCTION_MATURITY_REFERENCE_REVIEW_2026-10-21.md)
explains the external reasoning and selective-reuse decisions behind this audit.
