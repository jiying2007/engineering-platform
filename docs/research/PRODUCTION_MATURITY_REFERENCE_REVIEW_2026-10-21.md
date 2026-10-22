# Production maturity primary-source comparison — 2026-10-21

Date: **2026-10-21 (Asia/Shanghai)**. Source baseline:
`jiying2007/engineering-platform` at
`a0628df29439b4980a9c93ba2d840fa9b8d8319a`, tree
`6f1f8e055ae381616c985d2cfda8a6eef00f472a`.

Status: **Archived research and audit input**. This file is not a live backlog,
production acceptance receipt or another authority. The current status remains
owned by [Implementation Status](../status/IMPLEMENTATION_STATUS.md); concrete
source counterexamples are recorded in the
[production maturity review](../reviews/PRODUCTION_MATURITY_REVIEW_2026-10-21.md).
The sources below are first-party documentation or upstream specifications,
accessed for this review. Their URLs may evolve; they do not establish facts
about an unobserved deployment of this repository.

## Scope and conclusion

The comparison covers production readiness, monitoring/alerting, PostgreSQL
backup and major upgrades, durable-workflow external effects, Agent execution
isolation, and GitHub Actions identity/provenance. It does not recommend a
framework replacement or paid resource. Existing Core, Worker, Action Gateway,
Evidence, Verification, independent Review and Recovery remain the authority
chain; `agent-dev-kit` remains the team Skill source. No duplicate Skill
evaluation or digital-worker compatibility project follows from this review.

The highest-value work is to repair the reproduced proof/process boundaries,
retain replayable CI measurement inputs, and distinguish unfinished production
capabilities from missing real-environment evidence. Green CI and cryptographic
hashes are useful within their stated scope; neither alone establishes a
production operating posture.

## Directly applicable findings

| Priority | Repository finding | Selective application | Primary sources |
| --- | --- | --- | --- |
| P1 | Retained Recovery proof JSON can differ from its original row/audit while the baseline read/authorize/complete paths accept it: `internal/store/postgres/recovery_proof.go`, `internal/store/postgres/store.go`, `internal/recovery/proof.go`. | Validate the same original immutable projection at every authority boundary, preserve actor/epoch separation, and reject corruption without inventing historical evidence. This is a locally reproduced defect; an external framework is not needed to repair it. | [R05](#r05-temporal-activity-definition) supplies the complementary external-effect limitation, not proof of this repository defect. |
| P1 | `internal/production/status.go::EvaluateSnapshot` intentionally leaves service readiness unobserved; `live_services.go` only supplies a bounded wiring observation. Terminal plan v3 still requires `operational_status_ready`. | Implement and qualify bounded, authenticated, fresh, same-deployment observations under existing #105. Keep database authority, reachability, capacity and qualification distinct. Missing/stale/contradictory input must remain blocked. | [R01](#r01-google-sre-launch-checklist), [R02](#r02-google-sre-alerting-on-slos). |
| P1 | `internal/production/acceptance.go` freezes gates but `cmd/eng/production_acceptance.go` does not verify all live gate evidence. `docs/status/TERMINAL_BLOCKER_EVIDENCE_CONTRACT.md` states its receipt shape is documentation only. | A future coverage reader must first enforce the existing frozen plan and Core subject/issuer/procedure/artifact rules. It may report completeness; it must not mint READY, human Review or production qualification. Retain this work in #105, with #228/#229 case contracts separate. | [R01](#r01-google-sre-launch-checklist). |
| P1 | The bounded latency summary in `internal/production/acceptance.go` lacks operating success/failure denominators, workload/resource trends and a continuous measurement window. The #229 blocker contract already calls for these. | Preserve the current summary scope, and collect/validate the missing operating observations against explicitly predeclared workloads and targets. Retain failures, cancellations and aborted runs; do not infer availability from only successful latencies. | [R02](#r02-google-sre-alerting-on-slos), [R03](#r03-prometheus-alerting). |
| P1 | `production-status --observe-for` and a nonzero diagnostic exit do not establish continuous alert delivery or observer freshness. | Define a small actionable alert set, last-success freshness and a test of the whole notification path for the selected deployment. A stopped monitor must not appear healthy because it emitted no error. | [R03](#r03-prometheus-alerting). |
| P1 | Current logical restore proof in `docs/implementation/RECOVERY_RESTORE_V1.md` does not establish the production backup policy, PITR, key custody or cross-failure-domain recovery. | Choose backup method from accepted RPO/RTO and recovery scope. If PITR is needed, retain a usable base backup, continuous WAL and configuration/key custody, and perform actual restore/data checks. Add upgrade rehearsal for the selected PG major and extensions. | [R06](#r06-postgresql-17-backup-methods), [R07](#r07-postgresql-17-pitr), [R08](#r08-postgresql-17-backup-verification), [R09](#r09-postgresql-17-major-upgrades). |
| P1 | Qualification helpers and saved-login entry points in `internal/runtime/codexapp/{qualification,live,engineering,provider}.go` have reproduced subprocess/pin-order defects. | Bound the whole owned process lifetime and output, verify the expected executable before executing probes, and require existing process-scope quiescence before successful live receipt. Preserve the explicit unqualified filesystem read-isolation claim. | [R04](#r04-openhands-process-sandbox) supports the host-process trust distinction; the four concrete defects are proved by this audit's local fixtures. |
| P1/P2 | Candidate `eng` verifies its own distribution in the baseline workflow; CI timing/admission reports retain hashes without their original input files. | Use the exact checked-out source verifier and expected commit; retain original inputs alongside existing reports, then reconstruct after download. Do not change the business Evidence authority or call this production qualification. | [R10](#r10-github-actions-secure-use), [R13](#r13-slsa-build-requirements-and-tracks). |
| P2 | The dependent CI evidence job reads required-job facts once and immediately tests success; the first PR #289 run failed that collection step after four upstream successes. The missing failed response prevents proving the precise predicate. | Use bounded same-attempt observation, exact producer artifact IDs and independently read metadata, preserve the actual response bytes and reject timeout/failure/identity drift. Dependency scheduling alone must not supply fabricated API facts. | [R15](#r15-github-actions-dependencies-and-original-artifact-identities). |

## Existing controls to preserve

The repository's PostgreSQL authority, audit/outbox, exact run/recovery epochs,
original operation binding and UNKNOWN/no-blind-retry semantics address a real
distributed-systems boundary. Temporal's documentation explains that an
Activity's external effect may occur before completion is recorded; lost
completion reporting can lead to retry. A workflow engine therefore does not
eliminate application idempotency, original completion evidence or recovery
readback. Preserve the original dispatcher and independent reconciliation rules
in `internal/action`, `internal/githubpublish`, `internal/recovery` and
`internal/store/postgres` ([R05](#r05-temporal-activity-definition)).

Keep Publisher credentials outside Worker/model execution and preserve explicit
provider identities and no silent fallback. Keep complete action commit pins,
minimum `GITHUB_TOKEN` permissions, authenticated artifact retrieval and exact
PR-head/source binding. The reviewed workflow changes do not require additional
permissions. Full SHA pins still need a reviewed update process; they are not
evidence that action source is safe forever ([R10](#r10-github-actions-secure-use)).

Retain the explicit `host-uid-trust` and unqualified filesystem read-isolation
boundary. OpenHands documents its process sandbox as ordinary host execution
without isolation. An Agent SDK, a process group or the word "sandbox" cannot
replace tested filesystem, process, network and credential boundaries. A
container also requires appropriate mounts and privilege settings; this review
does not claim that changing an execution backend automatically qualifies it
([R04](#r04-openhands-process-sandbox)).

Keep the actual PostgreSQL logical backup/restore and authority snapshot tests.
Keep the distinction between integrity checking and a usable restore. PostgreSQL
explicitly states that `pg_verifybackup` cannot perform all checks made by a
running restored server and does not replace restoration/data validation
([R08](#r08-postgresql-17-backup-verification)).

## Machine-verifiable properties and real-environment evidence

| Area | Bounded code can verify | The selected environment must still prove |
| --- | --- | --- |
| Terminal coverage | Existing plan version/digest/rules; complete applicable gate/case references; no duplicates/unknown cases; exact run/subject/issuer/procedure; original artifact bytes and authenticated Core record relationships. | Genuine origin and meaning of the observations, qualified provider/host, and independently accepted operating results. A self-filled reviewer field is not a human Review. |
| Agent execution | Pin-before-exec ordering, byte/output bounds, cancellation/reaping behavior, permission denials in the exercised test environment and refusal of unsupported claims. | Actual production kernel/mount/UID/network/secret boundaries and long-running behavior. Local process cleanup is not a remote provider cancellation certificate. |
| Recovery | Original proof row/JSON/audit consistency, epoch and participant separation, known transaction/readback paths, no replay after ambiguity. | The original external dispatcher's quiescence and provider-specific outcome, including delayed/partial effects and actual emergency revocation. |
| Operating targets | Complete source records, failure denominators, exact workload/window/threshold identities, deterministic statistical derivation and consistency/freshness checks. | Representative load, capacity, tail latency, sustained resource trends, alert delivery and recovery under the selected host/provider's actual conditions. |
| Backup and upgrades | Backup identity/manifest, version/config references, availability of required WAL, parser/receipt consistency and a controlled restore procedure. | Usable independently stored backup, recoverable keys, measured RPO/RTO, actual storage durability and post-restore external-operation reconciliation. |

Numeric production targets must be chosen for the service and declared before
qualification, as the existing blocker contract requires. Values used as
examples in SRE books or CI tests are not this project's SLO. In particular,
provider latency and CI elapsed time must not be relabeled as billing,
engineer-efficiency or production-availability measurements.

The existing issue ownership is sufficient:

- [#105](https://github.com/jiying2007/engineering-platform/issues/105) owns
  authenticated readiness, operational alerting, SLO/capacity and terminal
  production acceptance. Its
  [READY design-gap record](https://github.com/jiying2007/engineering-platform/issues/105#issuecomment-6062648223)
  and [live-observation limitation](https://github.com/jiying2007/engineering-platform/issues/105#issuecomment-6073445837)
  already distinguish missing implementation from missing live evidence.
- [#229](https://github.com/jiying2007/engineering-platform/issues/229) retains
  caller-context/maintenance lifecycle, ambiguous outcomes and selected-host
  PostgreSQL/latency/soak qualification.
- [#228](https://github.com/jiying2007/engineering-platform/issues/228) retains
  independent no-replay recovery for the unresolved external-effect classes.
- [#103](https://github.com/jiying2007/engineering-platform/issues/103) and
  [#106](https://github.com/jiying2007/engineering-platform/issues/106) remain
  optional, independently qualified unattended credential/provider alternatives.

## Additions not justified by the present evidence

Do not replace the Core with Temporal simply to claim exactly-once external
effects. Do not introduce Kubernetes, OPA, MinIO or an entire telemetry service
because a production checklist names the general capability. The current
systemd/PostgreSQL baseline can be assessed on concrete operating requirements;
every additional service adds its own custody, availability and recovery work.

Do not replace the Runtime wholesale with OpenHands or another Agent framework
to address the four bounded subprocess defects. Absorb relevant execution
boundary designs while preserving Core authorization and independent Publisher
ownership. Do not add a second Skill source or re-evaluate team Skills here.

Do not require PITR or a new high-availability topology without accepted recovery
targets. Conversely, if the targets require PITR, a successful logical dump test
cannot stand in for the needed base-backup/WAL chain and real restoration.

Cryptographically authenticated build attestations are a possible future
distribution control, particularly for offline or cross-trust-domain consumers.
They are lower priority than the reproduced proof/process defects and missing
raw CI inputs. GitHub's attestations identify build provenance, not application
correctness or production acceptance; verification must constrain the expected
repository/workflow/source. This review neither enables a paid feature nor
claims that the current repository meets a SLSA level
([R12](#r12-github-artifact-attestations),
[R13](#r13-slsa-build-requirements-and-tracks)).

## Primary-source register

### R01 — Google SRE launch checklist

https://sre.google/sre-book/launch-checklist/

Use the readiness dimensions—dependencies, monitoring, capacity, release and
recovery—to assess actual service evidence. The checklist is a review aid; it
does not certify this repository or prescribe universal capacity targets.

### R02 — Google SRE alerting on SLOs

https://sre.google/workbook/alerting-on-slos/

Use good/total-event SLIs, error-budget impact, detection/reset time and alert
precision. The examples support collecting denominators and choosing useful
alerts; their numeric SLOs are illustrative.

### R03 — Prometheus alerting

https://prometheus.io/docs/practices/alerting/

Use actionable end-user symptoms, processing duration/last-success freshness,
capacity awareness and meta-monitoring. Test the notification chain end to end;
an emitted diagnostic alone does not establish delivery. These practices do not
require adopting Prometheus in this repository.

### R04 — OpenHands process sandbox

https://docs.openhands.dev/openhands/usage/sandboxes/process

The upstream explicitly distinguishes ordinary host-process execution from an
isolated sandbox. Use it to keep execution/permission claims tied to the actual
backend and exercised boundary.

### R05 — Temporal Activity definition

https://docs.temporal.io/activity-definition

The idempotency section explains that completion is recorded only after the
Activity reports back. An external effect followed by lost reporting still
requires application-level safety. Temporal does not supply this platform's
independent publisher/reconciler evidence by being installed.

### R06 — PostgreSQL 17 backup methods

https://www.postgresql.org/docs/17/backup.html

The official overview separates SQL dump, filesystem-level backup and
continuous archiving. Select the method from actual recovery needs and its
assumptions, rather than treating the names as interchangeable.

### R07 — PostgreSQL 17 PITR

https://www.postgresql.org/docs/17/continuous-archiving.html

PITR needs a filesystem base backup plus the required continuous WAL history;
`pg_dump` output cannot serve as that base. Archive completeness/lag and storage
failure affect recovery, and WAL does not restore manually edited PostgreSQL
configuration files. Preserve configuration custody alongside database backup.

### R08 — PostgreSQL 17 backup verification

https://www.postgresql.org/docs/17/app-pgverifybackup.html

`pg_verifybackup` checks a base backup against its manifest and required WAL,
with version-specific WAL tooling. It is an integrity check, not a substitute
for starting the restored server and validating its data and application use.

### R09 — PostgreSQL 17 major upgrades

https://www.postgresql.org/docs/17/upgrading.html

https://www.postgresql.org/docs/17/pgupgrade.html

Major-version migration requires an appropriate dump/restore, `pg_upgrade` or
replication procedure and application/extension compatibility checks. A physical
backup is not a generic cross-major migration; replacing binaries is not a
universal downgrade path. Review `pg_upgrade` copy/link/clone and durability
semantics when choosing the actual deployment procedure.

### R10 — GitHub Actions secure use

https://docs.github.com/en/actions/reference/security/secure-use

Use least-privilege tokens, immutable action identities, reviewed workflow
inputs and appropriate runner trust. A persistent self-hosted runner and an
environment approval do not by themselves isolate untrusted code.

### R11 — GitHub Actions OIDC

https://docs.github.com/en/actions/concepts/security/openid-connect

Short-lived exchanges reduce stored long-lived credentials, but provider trust
conditions must select the intended repository/workflow/ref/environment. OIDC
does not turn an unqualified provider or deployment into a qualified one.

### R12 — GitHub artifact attestations

https://docs.github.com/en/actions/concepts/security/artifact-attestations

https://docs.github.com/en/actions/how-tos/secure-your-work/use-artifact-attestations/use-artifact-attestations

Attestations authenticate build provenance associated with a workflow and
artifact. Consumers must verify expected identities; the attestation is not a
business Evidence/Verification/Review replacement or evidence of safe behavior.

### R13 — SLSA build requirements and tracks

https://slsa.dev/spec/v1.2/build-requirements

https://slsa.dev/spec/v1.2/tracks

The specification distinguishes provenance completeness, authenticity,
accuracy and build isolation. Retaining a report's raw input improves
reproducibility; hashes alone do not establish a trusted origin or a compliant
build platform. No SLSA level is awarded by this comparison.

### R14 — Go subprocess stdin ownership

https://pkg.go.dev/os/exec#Cmd.StdinPipe

https://pkg.go.dev/os/exec#Cmd.Wait

The documented `StdinPipe` contract gives `Wait` automatic closure after the
command exits. The follow-up fresh-main failure exposed an extra caller close
racing that lifecycle. The reviewed Go 1.26.10 source and a deterministic
production-helper regression confirmed the same ownership rule. Caller-owned
`os.Pipe` descriptors supplied as `*os.File` avoid that conflicting ownership;
they also make the parent responsible for closing both of its descriptors.
This supports the bounded repair recorded in the audit, not a claim that a
subprocess exit or a pipe close proves external provider quiescence.

### R15 — GitHub Actions dependencies and original artifact identities

https://docs.github.com/en/actions/reference/workflows-and-actions/workflow-syntax#jobsjob_idneeds

https://docs.github.com/en/rest/actions/workflow-jobs#list-jobs-for-a-workflow-run-attempt

https://docs.github.com/en/rest/actions/workflow-runs#get-a-workflow-run-attempt

https://docs.github.com/en/rest/actions/artifacts#list-workflow-run-artifacts

https://github.com/actions/upload-artifact/blob/b7c566a772e6b6bfb58ed0dc250532a479d7789f/action.yml

The dependency contract determines which jobs must succeed before scheduling.
Attempt-specific run/jobs endpoints support original execution binding. The
artifact response supplies identity, run/source, digest, size and expiry; it
does not provide a documented run-attempt field. The exact pinned upload action
exposes the newly uploaded artifact ID, which can be passed between jobs and
independently checked through the API. These sources do not promise a maximum
status-visibility delay. A finite observation budget is a repository design
choice, and failure to observe complete valid facts must remain a failed gate.
