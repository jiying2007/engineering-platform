# Independent observer journal check v1

`eng production-observer-check` lets an independently scheduled process detect
missing, malformed, stopped or expired progress-observer output. It closes the
gap between the existing finite monitor and its previously library-only
freshness evaluator. It supplies a local diagnostic exit status; it does not
grant service readiness, execution, Evidence acceptance or production
qualification.

## Select the observer before checking it

The caller supplies all three inputs explicitly:

```sh
eng production-observer-check \
  --journal /var/lib/engineering-platform/observer/SELECTED_SESSION.jsonl \
  --session EXPECTED_32_LOWERCASE_HEX \
  --endpoint-digest sha256:EXPECTED_CONTROL_ENDPOINT_DIGEST \
  --require-current-clear
```

The session and endpoint digest must come from the accepted launcher and its
Control endpoint configuration. The checker does not discover them from the
first journal record. The digest is SHA-256 of the exact configured
`CONTROL_ENDPOINT` string, not a server certificate, Core instance identity or
authenticated Evidence issuer.

Monitor mode now accepts an optional externally selected `--session`. Generate
a fresh 128-bit value for each invocation, give the same value to the producer
and checker, and use a new journal path. Do not reuse a session when restarting
the monitor or selecting a different Recovery epoch. Omitting this option keeps
the existing randomly generated monitor session behavior; single-snapshot and
single-window commands reject it.

For an operator-controlled launch, the producer remains the existing command:

```sh
# Run as the journal owner, with that producer's existing core:read mTLS env.
# SESSION is a fresh externally generated 32-character lowercase hex value.
umask 077
set -C # refuse to overwrite an existing session journal
eng production-status --monitor-for 1h --observe-for 30s --interval 5s \
  --max-pending-age 2m --stale-after 45s --session "$SESSION" \
  --require-no-alert > "/var/lib/engineering-platform/observer/$SESSION.jsonl"
```

The duration, interval and age settings above are operating examples, not
measured SLOs. The producer remains finite. On successful schedule completion
it emits STOPPED, which the independent checker deliberately treats as no
longer fresh. Renewal requires a new explicitly selected session and matching
checker configuration; there is no silent following of a new producer.

## Input and failure boundary

The Linux reader accepts an absolute canonical path of at most 4096 bytes and
128 path components. It opens every directory component with descriptor-bound
`openat` and `O_NOFOLLOW`; the final descriptor also uses `O_NONBLOCK` so a FIFO
substitution cannot block opening. The file must be regular, single-link,
owned by the invoking UID and mode 0600 or 0400. Symlinks, hard links, devices,
pipes, empty inputs and oversized files fail closed.

The reader binds the original descriptor, directory identities, file owner,
mode, length, modification time and change time. After reading, it rechecks the
same descriptor and reopens the absolute path with the same safe traversal.
Changes observed during this read prevent a report. This is a stable readback
of the supplied prefix; it is not proof that no process retains a write
descriptor or that no append can happen after the last check. A concurrent
append or partial write may therefore make one timer attempt fail. A later
complete, stable observation can recover; no bad tail is silently discarded.

The limits match the producer: 8 MiB per JSONL record and 128 MiB per
invocation, including newlines. Every record must end in a newline. Empty,
partial, duplicate-key, schema-incomplete or contradictory records are rejected.
Required zero/false fields and required nullable snapshot timestamps retain
their existing schema meaning. Unknown fields and omitted facts cannot become
default healthy values.

The checker requires STARTED first and replays each record through the existing
Observer state machine. Sequence, policy, session, endpoint, Recovery binding,
window contents, counters, state changes and STOPPED ordering must agree. A
FAILED record may legitimately contain the first Recovery binding when the
producer observed a valid first snapshot before that window later failed.
This claimed binding is retained as a diagnostic fact, not as proof of the
unretained request.

The report retains the digest and length of the exact journal bytes, record
count, latest state, timestamps and original `had_alert`. `current_clear` is
true only while both the observer record and its complete source window are
fresh and the latest state has no diagnostic alert. Recovery can make the
current state clear while earlier alerts remain recorded.

Without `--require-current-clear`, exit zero means a valid journal was checked.
With it, missing/unavailable source observations, WAITING, diagnostic alerts,
STOPPED and expiry all produce nonzero exit. Freshness is checked again after
output succeeds, so a slow sink cannot preserve an earlier successful strict
exit. The printed report's `checked_at` remains the time of that snapshot.
An invalid journal or failed read produces no successful partial report.

The invocation has a signal-owned 15-second Context. Directory traversal,
parsing and final validation check cancellation; Linux pipe output uses the
existing independently opened deadline-aware descriptor. Arbitrary synchronous
filesystem or device syscalls retain the operating system's blocking boundary.
No unbounded detached reader/writer is introduced.

## Independently scheduled local failure signal

The embedded distribution includes:

- `observer-watchdog.env.example`;
- `systemd/maintenance/engineering-observer-watchdog.service`;
- `systemd/maintenance/engineering-observer-watchdog.timer`.

An authorized host administrator configures an `engineering-observer` identity,
the selected journal directory and private environment file, then installs the
two units using the normal deployment procedure. The example checks after boot
and 15 seconds after each completed check. Choose that cadence together with
the explicit producer freshness policy; these sample timings are not a
calibrated outage-detection target. The distribution installer only delivers
the templates and never creates accounts, enables timers or starts services.

The checker is a separate systemd unit and process from the producer. It does
not load Core API credential variables, call Core, restart the producer,
mutate a ledger or replay an operation. Private networking, read-only journal
access and an empty capability set limit accidental effects. Since the private
journal is owner-only, producer and checker normally share a UID. Process and
scheduling independence do not imply credential isolation or an independent
security principal; same-UID host access remains part of the declared trust
boundary.

Failed strict checks are exposed through the unit exit status and journal.
Connect those failures to the deployment's accepted monitoring/notification
channel, and verify actual receipt there. No default recipient, external
webhook or successful human notification is invented. If the whole host or
its service manager dies, detection requires a separately hosted monitor.

## Evidence and acceptance

Repository tests exercise the real compiled producer and checker, authenticated
mTLS Core middleware, a deliberately synthetic Store, actual producer process
death, file boundaries and record expiry. They do not qualify a production
database, actual operator installation or delivered external notification.

A contiguous prefix and its digest do not prove producer authenticity, absence
of a deleted tail or protection against rollback between separate checks. The
external session selection, source expiry and protected host storage are
necessary operating inputs. All reports keep READY, execution and production
qualification false. Full deployment health/capacity, independent evidence
acceptance, live notification, host failure handling and operational SLOs remain
within #105/#229.
