# Engineering process-tree containment v1

The Core-bound engineering app-server is now launched as PID 1 in a new Linux
PID namespace, owned by a new user namespace. The numeric host UID/GID are
preserved: this does not introduce a root launcher, new daemon, provider lane,
credential broker, cgroup delegation or process-group fallback. This lifecycle
boundary complements the existing Codex filesystem/network/tool sandbox; it is
not a filesystem, resource, or complete hostile-code sandbox on its own.

Linux terminates the namespace's remaining processes when its init exits.
Descendants may double-fork, setsid or create nested PID namespaces; none of
those operations escapes into an ancestor PID namespace. The outer Worker
holds the original Go Process handle and waits for that exact init, not a
recycled numeric PID. The launch OS thread stays locked until Wait completes;
SIGKILL parent-death notification protects the subtree if that Worker thread
or process dies. External brokers/remote work and unrelated host processes are
outside this boundary and are not asserted stopped.

Sources: Linux man-pages `pid_namespaces(7)` and `user_namespaces(7)`; Go
`src/syscall/exec_linux.go`, especially Cloneflags, identity maps and Pdeathsig.
https://man7.org/linux/man-pages/man7/pid_namespaces.7.html
https://man7.org/linux/man-pages/man7/user_namespaces.7.html
https://go.dev/src/syscall/exec_linux.go

## Evidence and failure semantics

The Worker observes the child namespace inode, different parent namespace,
original host PID, start time, and successful init reap. A bounded kill/wait
that cannot establish the reap is unconfirmed, never quiescent. Missing or
failed namespace creation fails before the model turn; there is no plain-exec
fallback. Failed/interrupt/transport paths close the protocol and terminate the
same namespace before recording outcomes. Diagnostic stderr uses a synchronized
bounded buffer while the process is live.

New engineering receipts are schema **4** and control transcripts are version
**2**. Both bind the exact same process-scope proof. Core Finish, publication
and Evidence reject missing, unconfirmed, foreign or mismatched proofs. An
interrupt ACK remains distinct from the turn event and kernel termination.
Old receipt bytes remain historical; they are not upgraded with invented
process observations. Drain or explicitly reconcile active old Workers before
upgrading the complete six-role distribution; do not mix old/new binaries.
No production host or database is changed automatically by this source merge.

## Host prerequisite

Run `eng runtime-isolation-probe` under the actual intended Worker Unix user.
It launches only `/bin/cat`, reads no provider credentials and contacts no
network. `LOCAL_PROCESS_SCOPE_VERIFIED` means only this user's kernel lifecycle
probe passed. It is not full platform, model, checkpoint or production admission.

Hosts must permit the required unprivileged user/PID namespaces and expose the
host procfs for observing the immediate child. Ubuntu AppArmor may require the
explicit path-scoped example in `examples/production/apparmor/engineering-worker`.
An operator must review and install it for the admitted executable path; neither
this command nor runtime execution edits host policy, asks for root, or silently
disables restrictions. The mechanism currently leaves the existing procfs mount
unchanged; it does not claim a private filesystem/process listing. Codex's own
sandbox remains required and its real-host/model compatibility is a separate
live qualification, not inferred from fake-provider integration tests.

Canonical Go CI explicitly requires namespaces. Any AppArmor exception is
limited to that job's temporary Go test executables, with no host-wide sysctl
change. Real kernel tests cover natural exit, cancellation, double-fork/setsid,
continued-write rejection and abrupt Worker death. Local unsupported developer
hosts may skip kernel tests with an explicit reason; canonical CI may not.

## Next boundary

A quiescent workspace can now be captured safely against this process subtree.
This alone is not a verified checkpoint, provider-thread resume, transferable
lease, or human takeover grant. Those operations require independent persisted
artifact identities and existing Core authority. Do not revive metadata-only
pause/resume/takeover or replay an ambiguous model turn.
