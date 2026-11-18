#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage: smoke-release.sh PACKAGE_TGZ

Verify and install an existing native Linux release, then run its README
example in a temporary Git repository. Requires bash, Git, Python 3 and
standard Linux archive/checksum tools. Does not build, access the network,
invoke a model, or use a device. Temporary files are removed on exit;
failures print the current stage and captured diagnostics.
USAGE
}
if [[ ${1:-} == --help || ${1:-} == -h ]]; then
  usage
  exit 0
fi
if (( $# != 1 )); then
  usage >&2
  exit 2
fi
command -v python3 >/dev/null || { printf 'Python 3 is required for smoke assertions.\n' >&2; exit 2; }
eng_archive="$(realpath -- "$1")"
case "$(uname -m)" in
  x86_64) eng_arch=amd64 ;;
  aarch64|arm64) eng_arch=arm64 ;;
  *) printf 'Unsupported smoke host architecture.\n' >&2; exit 2 ;;
esac
eng_smoke="$(mktemp -d "${TMPDIR:-/tmp}/eng-release-smoke.XXXXXXXX")"
eng_stage='archive verification'
eng_log="$eng_smoke/smoke.log"
touch "$eng_log"
cleanup() {
  local eng_exit=$?
  if (( eng_exit != 0 )); then
    printf 'Release smoke failed during %s (exit %d).\n' "$eng_stage" "$eng_exit" >&2
    tail -n 60 "$eng_log" >&2 || true
  fi
  rm -rf -- "$eng_smoke"
  exit "$eng_exit"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

(
  cd -- "$(dirname -- "$eng_archive")"
  sha256sum -c "$(basename -- "$eng_archive").sha256"
) >> "$eng_log" 2>&1
tar -xzf "$eng_archive" -C "$eng_smoke" >> "$eng_log" 2>&1
eng_package="$eng_smoke/eng-linux-$eng_arch"
eng_stage='package installation'
"$eng_package/install.sh" --bin-dir "$eng_smoke/bin" >> "$eng_log" 2>&1
eng_binary="$eng_smoke/bin/eng"
cmp "$eng_package/eng" "$eng_binary" >> "$eng_log" 2>&1
"$eng_binary" version > "$eng_smoke/version.json" 2>> "$eng_log"

eng_stage='damaged package rejection'
cp "$eng_package/README.md" "$eng_smoke/README.original"
printf '\nDeliberately changed smoke-test bytes.\n' >> "$eng_package/README.md"
if "$eng_package/install.sh" --bin-dir "$eng_smoke/tampered-bin" > "$eng_smoke/tamper.log" 2>&1; then
  printf 'Installer accepted a package with changed content.\n' >> "$eng_log"
  exit 1
fi
cat "$eng_smoke/tamper.log" >> "$eng_log"
grep -Fq 'Package checksum verification failed.' "$eng_smoke/tamper.log"
mv "$eng_smoke/README.original" "$eng_package/README.md"

eng_stage='README Git example preparation'
eng_demo="$eng_smoke/example"
mkdir "$eng_demo"
eng_git() {
  env -u GIT_DIR -u GIT_WORK_TREE -u GIT_COMMON_DIR -u GIT_INDEX_FILE \
    -u GIT_OBJECT_DIRECTORY -u GIT_ALTERNATE_OBJECT_DIRECTORIES -u GIT_CONFIG \
    GIT_CONFIG_COUNT=0 GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1 \
    git -c core.hooksPath=/dev/null -c commit.gpgsign=false "$@"
}
eng_git init -b main "$eng_demo/repo" >> "$eng_log" 2>&1
printf 'eng local example\n' > "$eng_demo/repo/README.md"
cp "$eng_package/examples/project-fixture.yaml" "$eng_demo/repo/.eng.yaml"
eng_git -C "$eng_demo/repo" add README.md .eng.yaml >> "$eng_log" 2>&1
eng_git -C "$eng_demo/repo" -c user.name='eng example' \
  -c user.email='eng-example@localhost' commit -m 'Initialize local example' >> "$eng_log" 2>&1
eng_original_sha="$(eng_git -C "$eng_demo/repo" rev-parse HEAD)"
run_eng() { "$eng_binary" --data-dir "$eng_demo/state" "$@"; }
cd "$eng_package"
eng_stage='README runtime and project registration'
run_eng runtime add fixture --no-model --stdin task --timeout 1m -- \
  bash "$eng_package/examples/fixture-runtime.sh" >> "$eng_log" 2>&1
run_eng project add example "$eng_demo/repo" --config .eng.yaml >> "$eng_log" 2>&1
run_eng doctor > "$eng_smoke/doctor.json" 2>> "$eng_log"
eng_stage='README local command execution'
run_eng run example --task "$eng_package/examples/task.md" > "$eng_smoke/run.json" 2>> "$eng_log"
eng_task_id="$(python3 -c 'import json,sys;print(json.load(open(sys.argv[1]))["id"])' "$eng_smoke/run.json" 2>> "$eng_log")"
run_eng status "$eng_task_id" > "$eng_smoke/status.json" 2>> "$eng_log"
run_eng logs "$eng_task_id" > "$eng_smoke/runtime.log" 2>> "$eng_log"
# Second real Git checkout: a master-default repository with no .eng.yaml,
# no --base and no configured checks. Its intentional waiting -> accept path
# proves first use need not invent a "main" branch or forge test success.
eng_stage='master-default onboarding and no-check acceptance'
eng_git init -b master "$eng_demo/master-repo" >> "$eng_log" 2>&1
printf 'master default example\n' > "$eng_demo/master-repo/README.md"
eng_git -C "$eng_demo/master-repo" add README.md >> "$eng_log" 2>&1
eng_git -C "$eng_demo/master-repo" -c user.name='eng example' \
  -c user.email='eng-example@localhost' commit -m 'Initialize master checkout' >> "$eng_log" 2>&1
master_original_sha="$(eng_git -C "$eng_demo/master-repo" rev-parse HEAD)"
run_eng project add master-example "$eng_demo/master-repo" --runtime fixture > "$eng_smoke/master-project.json" 2>> "$eng_log"
if run_eng run master-example --task "$eng_package/examples/task.md" > "$eng_smoke/master-waiting.json" 2>> "$eng_log"; then
  printf 'No-check task bypassed required manual acceptance.\n' >> "$eng_log"
  exit 1
fi
master_task_id="$(python3 -c 'import json,sys;print(json.load(open(sys.argv[1]))["id"])' "$eng_smoke/master-waiting.json" 2>> "$eng_log")"
run_eng accept "$master_task_id" --note "Reviewed deterministic smoke output; no model or hardware claimed" > "$eng_smoke/master-accepted.json" 2>> "$eng_log"
test "$(eng_git -C "$eng_demo/master-repo" rev-parse HEAD)" = "$master_original_sha"
test -z "$(eng_git -C "$eng_demo/master-repo" status --porcelain)"
eng_stage='local result assertions'
test "$(eng_git -C "$eng_demo/repo" rev-parse HEAD)" = "$eng_original_sha"
test -z "$(eng_git -C "$eng_demo/repo" status --porcelain)"
python3 - "$eng_smoke" "$eng_package" 2>> "$eng_log" <<'PY'
import hashlib
import json
import pathlib
import sys

root, package = map(pathlib.Path, sys.argv[1:])
assert (package / "MIGRATION.md").is_file()
# Daily task templates must survive packaging, checksum and installation without
# fetching an optional reference repository or loading a model.
for template in ("debug-fix.md", "feature-change.md", "review-release.md"):
    data = (package / "examples" / "templates" / template).read_text(encoding="utf-8")
    assert len(data) > 200 and data.startswith("# "), template
assert "](MIGRATION.md)" in (package / "README.md").read_text()
doctor = json.loads((root / "doctor.json").read_text())
assert doctor["ready"] and doctor["model_invocations"] == 0, doctor
status = json.loads((root / "status.json").read_text())
assert status["task"]["state"] == "done", status
assert len(status["attempts"]) == 1, status
result = status["attempts"][0]["result"]
assert result["runtime_exit_code"] == 0 and result["local_process_stopped"], result
assert not result["remote_outcome_unknown"], result
assert len(result["checks"]) == 1 and result["checks"][0]["exit_code"] == 0, result
assert result["checks"][0]["source_commit"] == result["result_commit"], result
assert pathlib.Path(result["summary_path"]).is_file(), result
assert pathlib.Path(result["diff_path"]).is_file(), result
assert len(result["artifacts"]) == 1, result
artifact = result["artifacts"][0]
content = pathlib.Path(artifact["path"]).read_bytes()
assert artifact["digest"] == "sha256:" + hashlib.sha256(content).hexdigest(), artifact
assert b"No model or device was used" in content
assert (root / "runtime.log").stat().st_size > 0
master_project = json.loads((root / "master-project.json").read_text())
master_waiting = json.loads((root / "master-waiting.json").read_text())
master_accepted = json.loads((root / "master-accepted.json").read_text())
assert master_project["base"] == "master", master_project
assert master_waiting["state"] == "waiting" and "No checks configured" in master_waiting["reason"], master_waiting
assert master_accepted["state"] == "done" and master_accepted["id"] == master_waiting["id"], master_accepted
print(json.dumps({"release_smoke": "passed", "damaged_package_rejected": True,
                  "task_id": status["task"]["id"], "result_commit": result["result_commit"],
                  "scope": "local command fixture and real Git; no model or device call"}, indent=2))
PY
