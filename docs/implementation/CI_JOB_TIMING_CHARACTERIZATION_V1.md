# Required CI job timing characterization v1

Status: **REPOSITORY-SIDE DIAGNOSTIC ONLY — NOT BILLING, SLO OR PRODUCTION QUALIFICATION**

The small-team five-gate CI spends most of its execution time in real,
repeated offline container integration and Go race tests. We must measure
exactly before changing coverage, isolation or runner topology.

## Producer and exact binding

The existing `trusted-ci-artifact-evidence` job already retrieves GitHub
Actions `jobs.json` from the **current** run using the read-only job token.
After the canonical evidence collection step, the independent
`scripts/ci_job_timing.py` parser consumes those same raw bytes. It requires
exactly one completed/successful instance of each four upstream required jobs,
with matching **source SHA / run ID / run attempt**. It validates the complete
first-page job inventory, ordered nonduplicated steps, bounded UTC times and
successful or skipped step conclusions. Invalid or incomplete input fails CI;
it cannot create a successful diagnostic artifact.

The single JSON report contains only the allowlisted required job/step names,
durations, SHA/Run identity and the SHA-256 of original read-only jobs JSON.
No environment variables, logs, tokens, command arguments, arbitrary output,
runner identity, or private source content are copied into the report.
The artifact name is `ci-job-timing-<source-sha>`, retained for 90 days.

## Interpretation and lifecycle

Each job duration is the elapsed interval from GitHub Actions' recorded job
start to completion. `total_runner_wall_ms` adds the four job elapsed times,
including time when jobs overlap; it is **not** the workflow critical path,
billed minutes, money spent or energy consumption. `max_job_wall_ms` is the
slowest of the four measured jobs, not the complete workflow duration because
the dependent Evidence job runs afterward. Step duration includes its own
runner/command effects, not an independent CPU utilization figure.

The report intentionally sets `billing_qualified`, `slo_qualified`,
`provider_live_qualified`, and `production_qualified` all to false.
Its adjacent raw-jobs digest is integrity linkage, **not** independent
authentication of GitHub, actual billing, or long-term archival.
Original GitHub Actions logs retain their own expiration policy.

The canonical trusted CI Evidence envelope, four required upstream checks,
exact binary/artifact qualifiers, Release/Delivery controls and branch
protection remain unchanged. No new permission, provider, namespace, test
skip, workload reduction or production-ready claim is introduced.

## Optimizing without weakening

Compare identical test shapes and GitHub runner/toolchain provenance over
multiple main CI runs before optimizing. The first measured focus areas are
`offline-container-integration`'s real systemd/Cortex-M/SIGKILL tests and
`go`'s repeated race/integration suites. These are critical safety
regressions; do not reduce counts solely to make CI look faster. Any later
parallelization or fixture pooling needs isolation-proof tests and must
preserve the same exact failures, identities, Evidence and negative cases.

This report remains advisory and subordinate to
[Implementation Status](../status/IMPLEMENTATION_STATUS.md).
