# Authenticated Core instance capacity observation v1

`eng production-capacity` reads bounded reservation and connection-pool facts
from one explicitly expected Core instance. Its scope is
`authenticated-core-instance-capacity`. It extends observation under #105 and
#229; it is not the whole-deployment readiness evaluator or a new authority
ledger. No migration, heartbeat writer, Provider call, lease acquisition or
external-effect retry is introduced.

## Select the source before observing

The authenticated `GET /api/v1/operations/capacity` requires the existing
`core:read` capability. Control derives its source identity from the exact
configuration already loaded for that startup:

| Identity | Source and check |
| --- | --- |
| Server certificate | SHA256 of the loaded server leaf DER; the client independently checks the actual verified TLS peer and current chain validity |
| Access policy | SHA256 of the original loaded policy bytes; policy paths are not reopened |
| Control source | Clean Git revision in the running Linux/amd64 Control executable's build information |
| Core instance | Random 128-bit nonce created for this startup and printed after the listener binds |

An authorized operator must obtain all four expected values from the selected
host's startup record and independently accepted distribution/configuration.
They must not be copied from an otherwise untrusted first endpoint response.
Reusing the same certificate and policy on another Core, or restarting this
Core, cannot satisfy a previously selected instance nonce. These identities
bind the authenticated source; they do not attest the entire host or prove
that a compromised server accurately reports its internals.

Use the existing Control client mTLS environment, with an identity authorized
for `core:read`, then supply all four expected values:

```sh
eng production-capacity \
  --server-cert-digest "sha256:<expected-leaf-DER-digest>" \
  --access-policy-digest "sha256:<expected-loaded-policy-digest>" \
  --source-commit "<expected-40-lowercase-hex-source-commit>" \
  --instance "<expected-32-lowercase-hex-startup-nonce>"
```

The command has one signal-owned, 15-second Context budget, uses the existing
direct-mTLS client and does not follow redirects. It commits output only after
the complete response, source identity, actual peer, schema and freshness checks
succeed. Linux pipe output shares the existing deadline-aware descriptor
mechanism. Other synchronous storage/device output retains its operating-system
blocking boundary. Transport failure, cancellation, truncation, duplicate or
missing facts, stale timestamps and identity drift cannot produce a successful
partial observation.

A development/dirty/unidentified build, dynamic certificate selection or policy
population beyond the bounded observation schema makes this diagnostic
unavailable. Existing valid Control operations remain available. The endpoint
also requires the PostgreSQL backend; in-memory development state cannot stand
in for deployed capacity.

## Facts and their limits

The Store uses the caller's existing authority budget and a five-second child
budget. The original v6 operational status, Recovery epoch/mode, current-policy
Worker registrations, active/expired Worker leases and selected-profile queues
are read in one read-only repeatable-read transaction. Every lease boundary uses
that transaction's single database timestamp. The original v6 public endpoint
and its semantics remain unchanged.

The Worker population comes exclusively from the already loaded access policy's
Worker-poll grants, including their profile sets and admission/preparation lanes.
Historical or unauthorized registrations do not add reservation headroom.
Selected worker/queue counts and online registration histories must agree with
the original global snapshot; that snapshot retains blockers outside the
selected profiles. Malformed retained lease facts, including a LEASED row with
no expiration or a non-LEASED row retaining an expiration, fail observation.

After releasing the transaction, Core samples its own pgx pool and records a
separate timestamp. This local pool snapshot is not atomic with the database
snapshot and is not PostgreSQL server-wide capacity. A changing workload may
change the facts immediately after either observation.

Transaction release uses the existing independent five-second rollback budget.
Cleanup failure prevents a pool sample or successful facts. Successful cleanup
does not erase caller cancellation: the original Context is checked again before
sampling. Existing deferred cleanup remains responsible for every early return.

| Output | Exact meaning |
| --- | --- |
| Original `authority` | Existing v6 authority diagnostic, including UNKNOWN operations, expired leases and Recovery blockers |
| Worker `recorded_registration_usable` | Retained registration has the expected protocol/status and a profile authorized by the current policy; it is not a current-process heartbeat |
| Worker `live_lease_limit` | Existing enforced limit of one live reservation per Worker identity |
| Worker `unoccupied_reservations` | One minus that usable identity's currently active leases; no reservation is acquired |
| `worker_reservation_headroom` | Sum of those unoccupied slots, without inferring actual Worker availability or throughput |
| `pool_reservation_headroom` | Local configured maximum minus acquired and constructing connections; not a measured latency, throughput or database SLO |
| Queue facts | Pending count/oldest pending time and active/expired leases for the current policy's profiles |

An expired lease remains in the original authority diagnostic even if a numeric
reservation slot is unoccupied. A nonzero headroom number never authorizes
admission, execution, publication or Recovery. Existing Core guards remain the
only authority for those operations.

The report retains the exact response bytes and their digest, request/receipt
times, expected source and observed peer certificate. All required fields must
be present, including explicit zero/false/empty arrays and nullable timestamps.
Observations must fit the finite 30-second freshness window and five-second
clock-skew bound. Those limits are transport/readback rules, not operating SLOs.
Retained JSON and digest agreement alone do not authenticate a later collector
or establish a previously accepted Evidence subject/procedure/issuer.

## Qualification boundary

Every result preserves `service_readiness=NOT_OBSERVED`,
`worker_process_capacity_observed=false`, `publisher_capacity_observed=false`,
`execution_authorized=false`, `ready=false` and `production_qualified=false`.
The existing `production-status --require-ready` remains fail-closed.

Same-deployment live Worker/Publisher/provider health and capacity, all terminal
operational gates, independently accepted collection/evidence procedures,
continuous watchdog and real notification delivery, declared workload/SLO/soak,
rollout/restore/emergency acceptance and independent human review remain under
#105 and #229. Historical missing witnesses and uncertain post-invocation
external effects remain under #228. This observation supplies a bounded source
of facts for those future evaluations; it cannot close those gates by itself.
