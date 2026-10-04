# Core-bound live Codex controls v1

This is a bounded implementation slice: actual steering input and interrupt
cancellation for an already-authorized single engineering turn. It adds no new
provider lane, service, model retry, Task authority or approval bypass.

## Authority and delivery

The existing mTLS RunControl principal submits a control with its own actor URI,
stable command ID, exact execution epoch, strictly increasing Session sequence,
actual bound thread ID and expected turn ID. Steering carries 1..8192 bytes of
nonempty UTF-8 input. Interrupt carries no text. Up to 63 steering inputs plus
one reserved interrupt slot are allowed per execution. Inputs cannot change the
frozen Task, tool grant, provider, credential, sandbox or approval policy.

Database migration 0009 extends the existing `steering_commands` ledger and
adds a one-to-one actual Runtime binding to `worker_codex_executions`. It does
not introduce a second control authority. Historical digest-only records stay
RECORDED and readable; they cannot be dispatched as live commands. Digest-only
POST /steer requests are rejected, not silently adapted.

Worker binds the actual thread/turn after starting the authorized model turn.
Every control claim rechecks the Worker subject, exact Profile/token, current
Run/Session epoch, runtime ownership, Recovery epoch and active execution lease.
The claim commits DISPATCHING before any provider RPC. A claimed command is
never returned a second time. No blind retry follows a lost reply. The operator
may read back its command ID; an exact submission retry is observation-only,
not a new dispatch. Reusing an ID with changed input is rejected.

States are intentionally distinct:

| State | Meaning |
| --- | --- |
| QUEUED | Core accepted the input, but it has not been dispatched |
| DISPATCHING | One dispatch was reserved; its result is not yet known |
| STEER_ACCEPTED | Matching provider RPC accepted the exact input; this is not proof that the task was fulfilled |
| INTERRUPT_ACKNOWLEDGED | Provider acknowledged cancellation; not proof that the turn or process has stopped |
| TURN_INTERRUPTED | Matching thread/turn completion event reported interrupted |
| NOT_APPLIED | Input was not dispatched, or was locally rejected before the RPC |
| UNKNOWN | A dispatch outcome is ambiguous; no replay or successful delivery |

A separate terminal observation records only the app-server process exit.
It does **not** prove that every background tool/descendant has quiesced. No
Human Takeover permission follows from either the ACK or that process exit.

After the event reader and control pump join, the Worker terminates/joins its
app-server process and seals a deterministic control transcript in Core. The
transcript retains exact input bytes/digests, actors, sequence, identities and
outcomes. Untouched queued inputs become NOT_APPLIED; dispatched inputs without
an outcome become UNKNOWN. Only acknowledged interruption plus the exact
interrupted event becomes TURN_INTERRUPTED. Terminal observations may be saved
after revocation, but they cannot restore execution authority.

A new FINISHED result must bind the sealed transcript digest, matching actual
thread/turn, successful completion and process exit. Every retained input must
be STEER_ACCEPTED. Interrupt, late/unapplied input, missing/ambiguous outcome or
failed sealing blocks successful result delivery. Publication and Codex Evidence
import independently check that same binding. A transcript is not Verification
or independent Review. Historical receipt fields/bytes are not rewritten or
silently admitted as current control evidence.

## Operator CLI

Use the existing direct-mTLS client environment. Actor comes from its certificate;
there is no --actor override. No provider credential is sent to this CLI.

```sh
eng run-control inspect --run RUN_ID
eng api GET /api/v1/runs/RUN_ID
```

Read the bound `runtime.binding.thread_id`, `turn_id`, `execution_epoch` and the
Session's `last_steering_sequence`. Submit a new strictly increasing sequence:

```sh
eng run-control steer --run RUN_ID --id CONTROL_ID \
  --epoch EPOCH --sequence NEXT_SEQUENCE --thread THREAD_ID --turn TURN_ID \
  --text-file steering.txt
eng run-control status --id CONTROL_ID
```

For cancellation, use a new ID/sequence, the same exact binding, and no text:

```sh
eng run-control interrupt --run RUN_ID --id INTERRUPT_ID \
  --epoch EPOCH --sequence NEXT_SEQUENCE --thread THREAD_ID --turn TURN_ID
eng run-control status --id INTERRUPT_ID
eng run-control inspect --run RUN_ID
```

A successful submission exit means the displayed receipt was read back, not
that steering fulfilled the objective or interruption is complete. On transport
failure, inspect the same ID before any explicit exact retry. Never substitute
a new ID to hide an ambiguous submission or provider call.

The legacy metadata-only pause/resume/takeover endpoints reject Codex-profile
Runs rather than falsely claiming a live process was paused or transferred.
Generic checkpoint metadata is not a Runtime snapshot or restart capability.

## Validation scope and remaining work

Unit/race tests exercise real local subprocess pipes, actual JSON-RPC requests,
ACK-versus-terminal separation, missing replies, report/seal failures, exact
input validation and CLI mTLS actor binding. PostgreSQL integration tests use
the actual mTLS API, durable authority, shared production Worker turn segment
and a deliberately fake local provider protocol. They also test lease/epoch and
Recovery fencing, immutable retry, transcript binding and cancellation capacity.
Fake provider output and fixture credentials are not account/model evidence.

This slice does not claim full interactive completion. Durable pause/resume,
quiescent process-tree proof, a verified workspace checkpoint, controlled
Human Takeover, interactive WorkBuddy UX and their live qualification remain
separate work. Full raw engineering artifact retention/restore remains separate
from the already-retained terminal fact records. No M1 pilot is rerun and no
production/provider/device qualification is granted by these tests.
