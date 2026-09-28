# Codex compatibility qualification v1

This contract qualifies the **actual Codex CLI binary used by a Worker** before
that binary may participate in a retained engineering execution. It deliberately
does not make one Codex CLI version a long-lived platform authority.

A qualification is non-model protocol evidence: it does not perform
`turn/start`, does not consume a retained engineering turn, does not provision a
provider credential, and does not grant GitHub/publication authority.

## Admission model

Runtime admission is based on compatibility proof, not exact version equality:

```text
actual Codex executable
  -> observe codex-cli semantic version
  -> hash exact executable bytes
  -> generate stable + experimental app-server schemas
  -> validate required protocol vocabulary/surface
  -> validate fixed credential-safe + engineering profiles
  -> real initialize / initialized / ephemeral thread/start
  -> deterministic QualificationReceipt
  -> qualification_digest
  -> Codex Profile v2
       actual codex_version
       binary_digest
       qualification_digest
       engineering_config_digest
       model / sandbox / approval / network policy
```

Different team machines may therefore run different Codex versions. A version
change causes a different binary/qualification/profile identity, but does not
require source changes merely because the version string changed.

The final admission rule is **qualification PASS**, not a semver allow-list.
Version syntax is retained for provenance and diagnostics only. An old or future
version whose app-server/profiles no longer satisfy the contract fails closed
during qualification.

## Required compatibility surface

The executable must be a canonical trusted regular file and must report a
bounded `codex-cli <semver>` value. Qualification then proves all of the
following against that exact byte digest:

- fresh `codex app-server --stdio` process;
- stable client capability `experimentalApi=false`;
- generated stable schema contains `sandbox` and `approvalPolicy`;
- required stable sandbox vocabulary includes
  `read-only/workspace-write/danger-full-access`;
- required approval vocabulary includes `never/on-request`;
- stable `ThreadStartParams` does not silently expose the experimental
  `permissions` surface;
- experimental schema exposes the expected experimental surface;
- fixed credential-safe feature profile is accepted;
- fixed engineering profile is accepted;
- real `initialize` + `initialized` + ephemeral `thread/start` succeeds;
- managed daemon/proxy and per-thread config overrides are not execution
  authorities.

No `turn/start` occurs in compatibility qualification.

## Why a fresh stdio process

The executable selected by the host is canonicalized, hashed and launched
directly. The platform does not use a background Codex daemon as execution
identity because a daemon may be stale relative to the CLI selected by the
operator. A changed executable byte digest invalidates the frozen Profile.

Provider configuration belongs to the host launch profile and isolated HOME.
Per-thread config injection is not accepted as authority.

## QualificationReceipt

The deterministic receipt records at least:

- schema version and compatibility-contract version;
- actual Codex CLI version;
- raw executable SHA-256;
- stable/experimental schema digests;
- credential-safe and engineering config digests;
- required protocol/profile checks;
- thread-start model used for this compatibility check.

The canonical receipt digest is frozen into `codexexec.Profile v2`. The
Profile digest is then the Worker capability and RunInput tool-profile identity.

The Worker configuration also carries the full QualificationReceipt. Before a
model-reachable engineering turn, the Worker **reruns compatibility
qualification on the current executable** and requires the new deterministic
receipt/digest to equal the frozen receipt/Profile. This prevents a hand-written
Profile or stale qualification JSON from admitting an unqualified binary.

## CI sentinel baseline

Repository CI still selects one concrete Codex package version as a **sentinel
compatibility baseline** so protocol drift is detected continuously. That
version is CI input, not a runtime admission pin.

The durable required check is:

```text
codex-app-server-qualification
```

and the durable artifact is:

```text
codex-compatibility-qualification-<source-sha>
```

Branch protection therefore does not change when the CI sentinel version is
updated.

The CI qualification job:

1. installs the selected sentinel `@openai/codex@<version>` in an isolated
   runner directory;
2. locates the native executable rather than qualifying a wrapper;
3. records package integrity and exact native binary digest;
4. runs the full compatibility contract above;
5. uploads the deterministic QualificationReceipt.

## Runtime and retained evidence boundary

Qualification proves compatibility only. A retained Feature/Debug proof still
requires the normal authority chain:

```text
qualification
  -> frozen Profile / Run
  -> credential mode (saved ChatGPT login or WIF)
  -> Core-bound model turn
  -> result commit + Git bundle
  -> independent publication
  -> exact PR-head CI
  -> Codex/Git/CI Evidence
  -> Verification
  -> independent Review
  -> Closure
```

Repository fixture/fake app-server tests remain regression evidence only. They
must never be represented as a real retained model execution.
