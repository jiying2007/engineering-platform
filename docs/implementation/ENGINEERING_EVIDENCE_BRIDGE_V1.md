# Engineering Evidence Bridge v1

Base: `e66cb4bee39ab9b876da6e2e420ebb3085b02ba0` (#47).

This increment converts two already available engineering facts into frozen,
requirement-bound Core Evidence without allowing a Runtime or Worker to self-issue
acceptance.

No service or database table is added.

## Dedicated evidence authorities

Two mTLS principals are reserved:

- `urn:engineering-platform:worker-evidence-importer`
  - issuer: `worker-execution-importer`
  - procedure: `worker.offline.execution.v1`
- `urn:engineering-platform:git-evidence-importer`
  - issuer: `git-change-importer`
  - procedure: `git.changed-tree.v1`

Each principal may hold only `core:read` and `evidence:register`. It cannot also
be a Worker, engineer, Action authority, verifier, reviewer, recovery operator or
another evidence issuer.

The frozen VerificationPlan must name the exact requirement ID, procedure and,
when configured, issuer. Existing Evidence registration repeats those checks.

## Worker offline execution evidence

The Control API exposes read-only retained preparation and offline execution
receipts for exact Run IDs. A memory/dev backend without durable Worker receipts
returns unavailable rather than fabricating them.

Before Delivery, an operator can compute the canonical digest of the retained
`WORKER_ATTESTED_OFFLINE_EXECUTION` receipt. Delivery must bind that digest as
an artifact with media type:

`application/vnd.engineering-platform.worker-offline-execution+json`

The dedicated importer then independently checks:

- Delivery subject digest recomputes exactly;
- preparation task digest and base commit match Delivery;
- preparation facts digest recomputes exactly;
- offline state is FINISHED and retains a receipt;
- Run ID, Worker identity and preparation digest agree across all receipts;
- execution token/profile and result digests are internally consistent;
- Docker recipe/container/output hashes and bounds remain valid;
- Delivery artifact digest equals the canonical retained receipt digest.

Exit code 0 creates PASS Evidence. A valid non-zero exit creates FAIL Evidence.
Invalid or drifting facts create no Evidence.

This procedure proves the result of the exact bounded offline command. It does
not prove the command is a sufficient engineering test unless the frozen
VerificationPlan explicitly requires this procedure.

## Git changed-tree evidence

`eng git-change-manifest` takes an operator-selected absolute local repository
and exact 40-hex base/result commits. It uses the existing trusted
`workspace.Manager` twice, producing two independent object stores and detached
checkouts with:

- no repository hooks;
- no inherited credentials;
- no filters or filesystem monitor;
- no alternates or linked worktree;
- no submodule materialization;
- exact commit verification;
- root Git tree SHA;
- full checkout SHA-256 snapshot digest.

The retained manifest contains only exact Git/content identities, not the local
repository path. It is valid only when both Git tree SHA and full source snapshot
digest changed.

Delivery must bind the exact manifest-file SHA-256 artifact with media type:

`application/vnd.engineering-platform.git-changed-tree+json`

At import time the dedicated Git importer re-captures both exact commits from the
operator-selected repository and requires byte-equivalent manifest facts before
registering PASS Evidence.

This procedure proves that the Delivery names two exact, different source trees.
It does not prove:

- ancestry or merge policy;
- who authored the change;
- that Codex or another model produced it;
- semantic correctness;
- CI success;
- release authorization.

Those remain separate evidence requirements.

## Operator flow

Worker execution:

1. complete the bounded offline execution;
2. `eng offline-receipt-digest --run RUN`;
3. include the returned digest/media type as a Delivery artifact;
4. create Delivery;
5. with the dedicated Worker evidence certificate, run
   `eng import-offline-evidence ...`.

Changed tree:

1. create the exact result commit by an independently authorized engineering path;
2. `eng git-change-manifest --repository ... --base ... --result ... --output ...`;
3. retain the manifest and include its raw file SHA-256/media type in Delivery;
4. with the dedicated Git evidence certificate, run
   `eng import-git-change-evidence ...`.

## Relationship to real model evidence

The existing WIF live receipt is a platform qualification probe with a fixed
prompt. It is intentionally NOT accepted as engineering-task model Evidence.

A future model-turn Evidence procedure must bind a real Core Run, frozen task,
prepared Context/workspace, qualified binary/model identity, exact model turn and
its produced result. Until a retained WIF engineering turn exists, no model
Evidence is issued.

## Remaining path to M1

After this bridge, the internally available evidence authorities are:

- trusted main CI provenance;
- bounded Worker execution result;
- exact Git changed-tree provenance;
- independent Verification;
- independent Review;
- durable recovery/restore proof.

M1 still requires a real authenticated model execution path (or another explicit
engineering Runtime) and one retained Feature plus one retained Debug pilot
through Delivery -> Evidence -> Verification -> Review -> Closure.
