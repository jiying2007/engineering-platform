# Production terminal maintenance fixture

This directory is the one-time production/unattended terminal qualification
fixture for issue #105.

It is deliberately a `RELEASE` task, not another Feature/Debug M1 pilot.

## Authorized mutation

Only:

`qualification.txt`

may change, from exact bytes:

`PRE_LIVE_READY\n`

to:

`TERMINAL_QUALIFIED\n`.

Any other changed file fails terminal acceptance.

## Pre-live use

Before provider credentials exist, the repository can validate:

- immutable terminal plan and digest;
- frozen requirement digest;
- Work/Task/Run templates through the real Core API;
- RELEASE routing;
- evidence requirements;
- independent human Review requirement.

The `Production terminal pre-live dry run` workflow retains these facts without
performing a model turn.

## Live use

Do not mutate the marker manually.

After one unattended Provider v3 identity is independently live-qualified, the
terminal qualification must consume this exact fixture and execute the existing
authority chain:

Requirement -> Task -> Run -> qualified Codex execution -> independent
Publisher -> PR -> exact PR-head CI -> Codex/Git/CI Evidence -> Verification ->
independent human Review -> ClosureReceipt.

Operational readiness must be READY before execution and again before Closure.
The terminal qualification also retains shutdown/restart/recovery and measured
SLO evidence outside the product Delivery subject.

A failed or ambiguous provider/publication operation is reconciled through the
existing UNKNOWN/Recovery path; it is never blindly replayed.
