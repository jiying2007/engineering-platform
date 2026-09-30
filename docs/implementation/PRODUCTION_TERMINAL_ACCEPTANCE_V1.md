# Production Terminal Acceptance v1

Date: 2026-09-30

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
- Codex/Git/trusted-CI Evidence procedures;
- production preflight/readiness gates;
- live-qualified unattended Provider v3 gate;
- independent Publisher gate;
- exact PR-head CI;
- Verification PASS;
- independent human Review PASS;
- ClosureReceipt;
- shutdown/restart/recovery proof;
- complete SLO report.

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

`.github/workflows/production-terminal-pre-live.yml` is manual-only and uses no
provider secret.

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
`NON_PROVIDER_COMPLETE`. After the real provider observation is retained, the
report becomes `COMPLETE`.

Final production SLO thresholds are frozen only from real operational
measurements; they are not guessed in source code.

## Live terminal sequence

After one unattended provider identity is independently qualified:

1. production preflight must be INTERNAL=READY;
2. operational status must be READY;
3. exact terminal plan/fixture bytes are frozen;
4. one Core-authorized unattended engineering execution changes only the marker;
5. independent Publisher creates/updates the exact PR;
6. exact PR-head trusted CI passes;
7. Codex/Git/CI Evidence is imported;
8. Verification passes;
9. an independent human reviewer records PASS;
10. ClosureReceipt is created;
11. clean shutdown/restart/recovery evidence is retained;
12. provider-inclusive SLO report is COMPLETE.

Only then may issue #105 and production readiness be closed.

## Failure semantics

A timeout or ambiguous provider/publication outcome never causes blind replay.
It enters the existing UNKNOWN/reconciliation authority.

A FAIL human review prevents Closure.

A degraded operational status prevents starting or closing terminal
qualification until reconciled.
