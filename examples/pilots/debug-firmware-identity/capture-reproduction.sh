#!/usr/bin/env bash
set -euo pipefail
umask 077

: "${GITHUB_REPOSITORY:?repository required}"
: "${GITHUB_SHA:?source SHA required}"
: "${GITHUB_REF:?ref required}"
: "${GITHUB_REF_PROTECTED:?protected-ref fact required}"
: "${GITHUB_WORKSPACE:?workspace required}"
: "${RUNNER_TEMP:?runner temp required}"

test "$GITHUB_REPOSITORY" = "jiying2007/engineering-platform"
test "$GITHUB_REF" = "refs/heads/main"
test "$GITHUB_REF_PROTECTED" = "true"
[[ "$GITHUB_SHA" =~ ^[0-9a-f]{40}$ ]]
test "$(git -C "$GITHUB_WORKSPACE" rev-parse HEAD)" = "$GITHUB_SHA"
test -z "$(git -C "$GITHUB_WORKSPACE" status --porcelain=v1 --untracked-files=all)"

STATE_ROOT="$RUNNER_TEMP/retained-debug-firmware-identity-reproduction"
TEST_FILE="$GITHUB_WORKSPACE/internal/material/retained_debug_firmware_identity_repro_test.go"
OUTPUT_FILE="$STATE_ROOT/go-test-output.txt"
RETAINED_TEST="$STATE_ROOT/reproduction-test.go"
install -d -m 0700 "$STATE_ROOT"

cleanup() {
  rm -f "$TEST_FILE"
}
trap cleanup EXIT

cat > "$TEST_FILE" <<'EOF'
package material

import "testing"

func TestRetainedDebugFirmwareIdentityReproduction(t *testing.T) {
	m := Manifest{
		TaskType:           "DEVICE_TEST",
		Repository:         "repo",
		BaseCommit:         "0123456789abcdef0123456789abcdef01234567",
		AcceptanceCriteria: []string{"passes HIL"},
		DeviceID:           "dut-001",
		FirmwareIdentity:   "not-a-digest",
	}
	if got := Evaluate(m).Status; got != Blocked {
		t.Fatalf("expected BLOCKED for noncanonical firmware identity, got %s", got)
	}
}
EOF
cp "$TEST_FILE" "$RETAINED_TEST"
chmod 0600 "$RETAINED_TEST"

set +e
(
  cd "$GITHUB_WORKSPACE"
  go test ./internal/material -run '^TestRetainedDebugFirmwareIdentityReproduction$' -count=1
) >"$OUTPUT_FILE" 2>&1
test_rc=$?
set -e

test "$test_rc" -ne 0
grep -F 'expected BLOCKED for noncanonical firmware identity, got READY' "$OUTPUT_FILE" >/dev/null

TEST_DIGEST="sha256:$(sha256sum "$RETAINED_TEST" | awk '{print $1}')"
OUTPUT_DIGEST="sha256:$(sha256sum "$OUTPUT_FILE" | awk '{print $1}')"

jq -n   --arg repository "$GITHUB_REPOSITORY"   --arg base_commit "$GITHUB_SHA"   --arg test_digest "$TEST_DIGEST"   --arg output_digest "$OUTPUT_DIGEST"   --arg expected_status "BLOCKED"   --arg observed_status "READY"   --arg firmware_identity "not-a-digest"   --argjson test_exit_code "$test_rc"   '{
    version:1,
    reproduction:"m1-debug-firmware-identity",
    repository:$repository,
    base_commit:$base_commit,
    test_digest:$test_digest,
    output_digest:$output_digest,
    test_exit_code:$test_exit_code,
    expected_status:$expected_status,
    observed_status:$observed_status,
    firmware_identity:$firmware_identity,
    reproduction_confirmed:true
  }' > "$STATE_ROOT/reproduction-receipt.json"
chmod 0600 "$STATE_ROOT/reproduction-receipt.json" "$OUTPUT_FILE"

rm -f "$TEST_FILE"
trap - EXIT
test -z "$(git -C "$GITHUB_WORKSPACE" status --porcelain=v1 --untracked-files=all)"

echo "retained Debug firmware-identity reproduction: CONFIRMED"
echo "base_commit=$GITHUB_SHA"
echo "expected=BLOCKED"
echo "observed=READY"
echo "artifact_root=$STATE_ROOT"
