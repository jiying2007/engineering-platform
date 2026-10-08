# Admission/security characterization v1

Status: **BOUNDED CI CHARACTERIZATION — NOT A PRODUCTION SLO**

The existing fivefold, race-enabled Worker-admission/security test still runs
96 precreated Runs, eight authorized Worker identities, and 48 denied identity
or profile probes in each repetition. It exercises real PostgreSQL, direct
mTLS Core and installed admission-only Worker processes, not model execution.

## Raw and derived facts

The Go test emits its standard JSON test events. The required CI step pipes
those events under \`set -o pipefail\` into
\`scripts/ci_admission_characterization.py\`. The parser **fails closed** unless
there are exactly five matching successful test completions and exactly five
complete metrics records. It requires exact workload shape; rejects malformed,
duplicate and reordered latency values, and emits an immutable newly created
one-file JSON report.

The report binds:

- exact tested Git SHA and GitHub Actions run ID/attempt;
- SHA-256 of the original local Go JSON event stream;
- five separate observed rounds with nanosecond integers;
- 480 admitted Run receipts and 240 explicitly denied probes;
- all three \`qualification_granted\`, \`production_qualified\` and
  \`slo_qualified\` strictly \`false\`.

The source event stream remains part of the ordinary CI job logs and is not
itself a long-term immutable production measurement. A matching checksum in
the report **does not authenticate** the observation source. GitHub Actions
retains the derived JSON under the same exact source SHA for 90 days. The
existing trusted CI Delivery/Evidence contract still binds its separate
binary and Codex artifacts; it has not been broadened to treat performance
characterization as a trusted verifier or acceptance result.

## What is measured

Process p50/p95/max include local admission-only process startup and a single
validation RPC in a disposable CI environment. The raw test also observes
intake and worker-wall durations. This is a regression baseline for identical
fixture dimensions, not a high-load throughput test.

No real Codex turn, model authentication, full Worker preparation, publisher
upstream, engineering result, board, GitHub CI-to-Verification lifetime,
production service availability, RPO/RTO or sustained multi-hour load is
measured. CI runner hardware/software changes invalidate cross-run direct
comparisons without normalization. There are deliberately **no pass/fail
latency thresholds** on this synthetic fixture.

Only real source-bound deployment measurements independently accepted under
issue #105 may freeze production SLOs and capacity. This file is explanatory
test documentation, subordinate to the single live Implementation Status.
