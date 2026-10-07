# Production Terminal Acceptance v1

Date: 2026-10-04

Status: **PRE-LIVE CONTRACT FROZEN — PROVIDER LIVE QUALIFICATION PENDING**

This document defines the final production/unattended acceptance sequence for
issue #105. It is not a new Runtime, Evidence family or approval authority.

## Immutable terminal plan

`eng production-terminal-plan` emits one deterministic plan and
`plan_digest`.

The plan fixes:

- fixture: `production-terminal-maintenance-v1`;
- task type: `RELEASE`;
- repository: `jiying2007/engineering-platform`;
- exact marker path and before/after bytes;
- at most one Core-bound engineering model turn;
- deployment profile: `canary-single-maintenance-fixture`;
- database rollback policy: `restore-authoritative-backup-and-reconcile`;
- emergency stops for provider credential/rule, Publisher credential and Worker execution;
- no silent provider fallback and no automatic database downgrade;
- Codex/Git/trusted-CI Evidence procedures;
- production preflight/readiness gates;
- live-qualified unattended Provider v3 gate;
- independent Publisher gate;
- exact PR-head CI;
- Verification PASS;
- independent human Review PASS;
- ClosureReceipt;
- shutdown/restart/recovery proof;
- source-verified SLO evidence and independent acceptance;
- canary deployment acceptance;
- provider emergency-disable proof;
- Publisher credential-revocation proof;
- database rollback/restore policy acceptance.

Human Review and provider live qualification are mandatory and cannot be
replaced by synthetic fixtures.

## Terminal maintenance fixture

The fixture lives under
`examples/production/terminal-maintenance`.

The only authorized source mutation is:

`qualification.txt: PRE_LIVE_READY -> TERMINAL_QUALIFIED`.

Any other changed file fails terminal acceptance.

The frozen requirement digest is:

`sha256:01edbdf525937b5cbd5464fc1c3b9e6b4b677af14b5bb802a137fbd70bfd0445`.

Work/Task/Run templates are validated through the real Core API in repository
tests. The task is RELEASE work and deliberately does not reopen the completed
Feature/Debug M1 pilots.

## Pre-live dry run

`.github/workflows/production-terminal-pre-live.yml` uses no provider secret. It
runs automatically on protected-main changes to the terminal contract/fixture
and also supports explicit `workflow_dispatch`.

On protected main it:

1. validates the terminal plan/SLO contracts;
2. dry-runs Work -> Task -> Run through Core;
3. verifies the exact requirement digest and PRE_LIVE marker;
4. retains plan + fixture bytes + SHA256SUMS as a 90-day artifact;
5. states explicitly that provider qualification and production readiness are
   not claimed.

This workflow must succeed before any live terminal qualification.

## Measured SLO calibration

`eng production-slo-report --observations FILE` consumes bounded measured
samples with a retained source digest for each measurement.

Required non-provider observations:

- `control_api_roundtrip`;
- `worker_admission`;
- `worker_preparation`;
- `engineering_run`;
- `publication_ci_verification`;
- `postgres_authority_restore`.

The provider observation is:

- `provider_authentication`.

The report computes deterministic sample count/min/p50/p95/max values. It does
not invent pass/fail latency targets.

Before provider access exists, all six non-provider observations produce
`UNVERIFIED_SUMMARY`, even if a provider measurement is supplied.

Report/input version 2 removes the old COMPLETE status. Source readback requires
`--source-root DIR --run RUN_ID --subject SUBJECT_DIGEST --require-verified`.
Each source is `<raw-sha256>.json` and binds version 1, metric name, exact run
and subject, `engineering-platform.slo.<name>.v1` procedure, collector digest,
start/completion window and the same sample array. It produces
`SOURCE_BYTES_VERIFIED`, with `qualification_granted=false`. Offline hashes do
not authenticate a collector or prove a real provider call. The existing
Evidence/Verification/independent Review must accept the provenance and targets.

Final production SLO thresholds are frozen only from real operational
measurements; they are not guessed in source code.

## Live terminal sequence

After one unattended provider identity is independently qualified:

1. the selected unattended provider is live-qualified with no silent fallback,
   and its emergency credential/rule disable path is proven;
2. production preflight must be HOST_VALIDATED for the admitted source commit;
3. the exact immutable release is admitted through the frozen canary deployment
   profile before broader rollout;
4. operational status must be READY;
5. the database rollback/restore policy is accepted and retains authoritative
   backup + Recovery/Reconciliation rather than automatic schema downgrade;
6. the independent Publisher credential-revocation path is proven;
7. exact terminal plan/fixture bytes are frozen;
8. one Core-authorized unattended engineering execution changes only the marker;
9. independent Publisher creates/updates the exact PR;
10. exact PR-head trusted CI passes;
11. Codex/Git/CI Evidence is imported;
12. Verification passes;
13. an independent human reviewer records PASS;
14. ClosureReceipt is created;
15. clean shutdown/restart/recovery evidence is retained;
16. source-verified provider-inclusive SLO evidence is accepted against frozen,
    measured targets by Verification and independent Review.

Only then may issue #105 and production readiness be closed.

## Failure semantics

A timeout or ambiguous provider/publication outcome never causes blind replay.
It enters the existing UNKNOWN/reconciliation authority.

A FAIL human review prevents Closure.

A degraded operational status prevents starting or closing terminal
qualification until reconciled.

## Audit revision

Terminal plan schema version 3 preserves the one-time maintenance fixture and
existing Evidence families while adding explicit rollout/emergency/rollback
constraints required by #105: canary admission, selected-provider kill path,
Publisher credential revocation, authoritative database restore/reconciliation,
no provider fallback and no automatic database downgrade. The earlier v2 plan
and pre-live artifact remain historical evidence and cannot qualify v3.

Terminal plan schema version 2 replaced the preflight-ready and SLO-complete
labels with `production_host_validated` and
`source_verified_slo_evidence_accepted`. The one-time maintenance fixture is
unchanged. The earlier v1 pre-live artifact remains historical evidence and
must not be reused as a v2/v3 qualification; retain a new provider-free dry run
for this exact v3 plan digest before any live attempt.
