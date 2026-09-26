# Frozen Requirement / Reproduction Context

Source: https://github.com/jiying2007/engineering-platform/issues/55
Issue: #55 M1 retained Debug pilot: reject non-canonical firmware identity
Snapshot: 2026-09-26

## Pilot role

Candidate retained **Debug** pilot for M1 with an authoritative source-level reproduction.

## Reproduction

`internal/material.Evaluate` treats DEVICE_TEST firmware identity as exact authority, but currently checks only `FirmwareIdentity != ""`.

Therefore a manifest equivalent to:

```go
Manifest{
    TaskType: "DEVICE_TEST",
    Repository: "repo",
    BaseCommit: "0123456789abcdef0123456789abcdef01234567",
    AcceptanceCriteria: []string{"passes HIL"},
    DeviceID: "dut-001",
    FirmwareIdentity: "not-a-digest",
}
```

is accepted as READY even though `not-a-digest` is not an exact immutable firmware identity.

The existing test also uses `sha256:abc`, which is not a canonical SHA-256 digest, so current coverage masks the bug.

## Expected behavior

DEVICE_TEST material readiness must require a canonical immutable firmware identity rather than any non-empty string.

## Acceptance criteria

1. Empty firmware identity remains BLOCKED.
2. Arbitrary text such as `not-a-digest` is BLOCKED.
3. Truncated values such as `sha256:abc` are BLOCKED.
4. A canonical `sha256:<64 lowercase hex>` identity is READY when all other DEVICE_TEST material is valid.
5. Degradation approval cannot bypass an invalid firmware identity because exact source/device identity is a hard authority requirement.
6. Existing DEBUG/FEATURE material readiness semantics remain unchanged.
7. `go test -race ./...`, repeated regressions, vet/build and repository CI all pass.

## Authoritative reproduction evidence

The failing behavior above is the reproduction. Before the retained Debug Task is created, preserve a failing focused test demonstrating the current READY result for an invalid firmware identity, then let the real Codex engineering turn implement the fix and regression tests.

## Retained-pilot constraints

- Task type must be DEBUG with `has_reproduction=true`.
- The result PR must not modify `.github/workflows/ci.yml`.
- The Codex Worker must not receive publisher credentials.
- Require Codex execution, Git changed-tree and trusted-CI evidence before Verification.
- Independent Review must PASS before Closure.

## Base

Freeze the exact current `main` SHA only when the Debug pilot begins. If the Feature pilot lands first, Debug must use the later main SHA rather than reusing the Feature base.
