# Operational SLI source comparison v1

This is the version-3 operational mode of the existing `production-slo-report`
command. It verifies retained bytes, computes deterministic operating statistics,
and compares them with explicit criteria. It is not a new Core Evidence
procedure, collector trust authority, production READY evaluator or acceptance
receipt. Terminal plan v3 and the historical latency-only SLO report/input v2
remain unchanged.

```sh
eng production-slo-report \
  --operational /absolute/path/manifest.json \
  --source-root /absolute/path/retained-sources \
  --run ACTUAL_RUN_ID \
  --subject sha256:ACTUAL_OPERATIONAL_SUBJECT_DIGEST \
  --deployment sha256:ACTUAL_DEPLOYMENT_IDENTITY_DIGEST \
  --require-criteria-met
```

The example placeholders are not valid digests and must not be used as real
observations. No provider call, network request, service mutation, notification,
credential access or Core state transition occurs in this command.

## Separate versioned mode

`--observations FILE` retains its exact v2 behavior and source-v1 readback. The
new `--operational FILE` input is mutually exclusive with it. Operational mode
requires `--source-root`, `--run`, `--subject` and `--deployment`. The deployment
and criteria-comparison flags cannot be used to reinterpret a v2 report.

An operational manifest contains:

| Field | Meaning |
| --- | --- |
| `version` | Exactly `3` |
| `criteria_digest` | SHA-256 of the original criterion-document bytes |
| `source_digests` | Exact metric-name to original event-source SHA-256 map |
| `resource_source_digest` | SHA-256 of the original resource-source bytes |

Each referenced file is `<64-lowercase-hex-sha256>.json` directly under the source
root. The criteria file, all metric files and resource file must have distinct
digests. A missing metric cannot be replaced by another metric's file.

All JSON uses the existing strict parser: one object, valid UTF-8, unique
lower-snake-case keys, no unknown fields and bounded nesting. The manifest and
each source are at most 1 MiB. Files must be regular and not group/world writable;
source paths are confined to the existing `os.Root`, with no final-file symlink.
The root and the resolved manifest parent must be normalized non-symlink safe
directories. Linux uses a directory-anchored `O_NONBLOCK | O_NOFOLLOW` opener,
so replacing a checked regular file with a FIFO cannot block the open operation.
Other platforms explicitly reject v3 source readback; legacy v2 is unchanged.
Reads are bounded before allocation and compare both path and opened descriptor
identity, size, mode and modification time before and after reading, plus the
actual original byte digest for referenced objects. Sources are never
rewritten or canonicalized before their raw digest is checked.

## Explicit criterion document

The criterion document has `version: 1` and these required identities:

- `run_id`, `subject_digest`, `deployment_digest`;
- `workload_digest`, `collector_digest`;
- `declared_at`, `started_at`, `completed_at` as RFC3339 timestamps;
- `minimum_soak_ms`, `maximum_source_age_ms`, `maximum_resource_gap_ms`;
- `metrics` and `resources` maps with the limits described below.

The run, subject and deployment must exactly equal the separately supplied CLI
expectations. The workload and collector digests are required and repeated in
all source files. Digests bind identifiers and bytes; they do not prove that
those identities belong to an authentic deployment or authorized collector.

The declared time must precede the measurement start. This only checks the
document's internal chronology: an operator can create a document later and
claim an earlier timestamp. Authentic pre-registration must be established by
the existing trusted Evidence/Verification/independent Review path. The output
therefore always has `pre_registration_authenticated: false`.

The declared measurement window must be a positive whole number of milliseconds,
at most 31 days, already completed at the CLI's actual evaluation time, and at
least the explicitly supplied minimum soak. The allowed source age and resource
gap must both be positive; age is at most 31 days and gap cannot exceed the
measurement window. No default production target is supplied. A missing/null
limit differs from a deliberate zero and is rejected. Changing any criterion
changes its digest and requires every raw source to bind the new criterion
document. This reader cannot establish whether limits were changed after an
earlier result; the independent pre-registration authority must establish that.

### Per-metric comparison limits

Every configured metric supplies all of:

| Field | Unit and comparison |
| --- | --- |
| `minimum_attempts` | At least this many total recorded attempts; 1..4096 |
| `maximum_non_success_ppm` | Maximum non-success count per million total attempts; 0..1,000,000 |
| `maximum_p99_ms` | Maximum nearest-rank p99 of all observed terminal-event latencies |
| `maximum_latency_ms` | Maximum individual observed terminal-event latency; at least the configured p99 limit |
| `minimum_success_milli_per_second` | Minimum thousandths of successful attempts per second over the entire window |

The six existing non-provider metric names are required:
`control_api_roundtrip`, `worker_admission`, `worker_preparation`,
`engineering_run`, `publication_ci_verification`, `postgres_authority_restore`.
`provider_authentication` remains optional for this offline report; its presence
does not prove a real provider call or satisfy final provider-inclusive acceptance.

Six named database-stage measurements are also supported when explicitly
included in the criteria: `postgres_queue`, `postgres_connection_acquisition`,
`postgres_lock`, `postgres_statement`, `postgres_commit`,
`postgres_complete_request`. These are separate populations, not a claim that
stage latencies add exactly to complete request latency. The criteria and
manifest must have exactly the same metric set. Missing configured metrics,
unknown names and extra unconfigured sources fail.

### Resource comparison limits

All seven resources are required, each with `maximum_peak` and
`maximum_positive_net_growth`:

| Resource | Raw unit |
| --- | --- |
| `cpu_millicores` | CPU usage in millicores |
| `rss_bytes` | Resident memory bytes |
| `disk_bytes` | Used disk bytes in the declared measurement scope |
| `fd_count` | File descriptors |
| `goroutine_count` | Goroutines |
| `thread_count` | Threads |
| `connection_count` | Connections |

Raw values and resource limits are nonnegative integers, at most 2^60. The
workload/deployment/collector identity must define a consistent aggregation
scope; this reader cannot establish that a claimed deployment-wide count actually
covered every process. Zero is a legitimate explicit value; absent map keys and
JSON `null` values are rejected and never interpreted as zero.

## Raw event sources

Every event source has `version: 1`, `kind: "request_events"`, the exact metric
`name`, and these common bindings:

`run_id`, `subject_digest`, `deployment_digest`, `workload_digest`,
`collector_digest`, `criteria_digest`, `started_at`, `completed_at`.

All must match the exact criterion document and original criterion digest.
The `events` array contains 1..4096 records, with:

| Field | Constraint |
| --- | --- |
| `sequence` | Exactly 1..N in retained order; no gaps or duplicate sequence |
| `id` | Unique nonblank bounded identifier in this metric population |
| `observed_at` | Nondecreasing timestamp inside the declared window |
| `outcome` | `success`, `failure`, `cancelled`, `aborted`, `timeout` or `dropped` |
| `latency_ms` | Explicit nonnegative integer for observed terminal outcomes; absent/null only for `dropped` |

An observed latency cannot exceed the elapsed window at its observation time.
Millisecond zero is valid for an observation with finer underlying resolution;
it is not a missing sample. A dropped event records one claimed missing attempt,
never an invented duration. Every dropped record makes the comparison fail,
even if the selected non-success rate limit would otherwise permit it.

All six outcomes count toward the total denominator. Non-success includes
failure, cancellation, abort, timeout and dropped records. Latency statistics
include every non-dropped outcome, including failed and cancelled requests;
they are not success-only latency summaries. The `latency_count` explicitly
differs from total when events were dropped. With no observed latencies the
zero-initialized statistic fields have count zero, and comparison fails.

The reader checks declared sequence completeness, not external completeness of
the collector. An unauthorized source can omit whole attempts while fabricating
a contiguous sequence. The report therefore never authenticates its collector.

## Resource samples and coverage

A resource source uses the same common binding, `version: 1`,
`kind: "resource_samples"`, and 2..4096 `samples`. Each record has a contiguous
`sequence`, strictly increasing `observed_at` within the window, and a `values`
map containing exactly the seven resource names. All gaps, including start to
first sample and last sample to completion, must meet the explicitly configured
maximum resource gap. This detects missing coverage under that chosen policy;
it cannot detect every unsampled transient.

Each resource report records first, last, peak, signed net change and positive
net growth `max(0, last-first)`. The peak and positive net growth are compared to
their configured limits. Net growth is an endpoint statistic, not a regression
slope, proof of steady state, or proof that memory/resource leaks are absent.
Original time-series bytes remain available by digest for independent analysis.

Longer windows or higher event/sample volumes that exceed these explicit format
budgets are rejected. The implementation never silently truncates or subsamples
data and then reports success. A workload that requires more records needs an
explicitly reviewed future representation or suitably scoped independent
measurement windows; split-window results cannot be silently combined into one
claimed soak qualification.

## Arithmetic and result interpretation

Percentiles use the existing nearest-rank helper: sorted index
`ceil(percent*N/100)-1`. Successful throughput is
`floor(success_count * 1,000,000 / window_ms)`, expressed in milli-successes per
second. Non-success rate is
`ceil(non_success_count * 1,000,000 / total_count)`, expressed in ppm. Conservative
rounding must not turn a threshold miss into a pass. At most 4096 records bound
both products well within int64. Counts, latencies and resources are integers;
no NaN, infinity or implicit floating-point rounding enters the report digest.

Every structurally valid comparison emits report version 3,
`status: "SOURCE_BYTES_EVALUATED"`, `source_bytes_verified: true`, exact criteria,
source digests, evaluation time, metrics, resources and all located comparison
failures. `criteria_met` describes only those local comparisons. These flags
always remain false:

- `collector_authenticated`;
- `pre_registration_authenticated`;
- `qualification_granted`;
- `production_qualified`.

Without `--require-criteria-met`, exit zero means the comparison completed; check
the report's result. With that flag, a comparison miss still prints the complete
report, then exits nonzero. Output errors or short writes also exit nonzero,
with a fixed diagnostic. Malformed, unsafe, incomplete, future, stale or
identity-drifted input fails before emitting a report. `report_digest` covers
the complete report including the actual evaluation time; fixed inputs and a
fixed evaluation time reproduce identical output, while later invocations bind
their own freshness evaluation.

The operational subject must be frozen separately from a product Delivery that
already precedes lifecycle/SLO measurement. Do not insert a source hash that
contains a subject digest into that same subject's digest calculation. The
existing Core Verification/independent Review chain must accept authentic
provenance, predeclared real targets and operational coverage before production
qualification or issue closure. This offline implementation alone cannot close
#105, #228 or #229, and it does not change `BLOCKED [228,229]`.
