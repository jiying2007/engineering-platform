#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage: build-release.sh [OUTPUT_DIRECTORY]

Build the native Linux eng release with embedded SQLite (CGO required).
The default output directory is tool/dist. Set ENG_GO_BIN to a Go executable
and ENG_VERSION to a release label when needed. No model is invoked.

Build official packages on the oldest supported Ubuntu release (22.04).
This script intentionally does not cross-compile a CGO binary.
USAGE
}

if [[ ${1:-} == --help || ${1:-} == -h ]]; then
  usage
  exit 0
fi
if (( $# > 1 )); then
  usage >&2
  exit 2
fi

eng_tool_root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)"
eng_go_bin="${ENG_GO_BIN:-go}"
eng_output="${1:-$eng_tool_root/dist}"

if [[ $(uname -s) != Linux ]]; then
  printf 'eng release builds require Linux.\n' >&2
  exit 2
fi
if ! "$eng_go_bin" version >/dev/null 2>&1; then
  printf 'A Go compiler matching tool/go.mod is required; set ENG_GO_BIN.\n' >&2
  exit 2
fi
eng_goos="$("$eng_go_bin" env GOOS)"
eng_goarch="$("$eng_go_bin" env GOARCH)"
eng_hostarch="$("$eng_go_bin" env GOHOSTARCH)"
if [[ $eng_goos != linux || $eng_goarch != "$eng_hostarch" ]]; then
  printf 'Use a native Linux builder for this architecture; cross-CGO builds are not supported.\n' >&2
  exit 2
fi
case "$eng_goarch" in
  amd64|arm64) ;;
  *) printf 'Supported release architectures: amd64, arm64.\n' >&2; exit 2 ;;
esac
if ! command -v "${CC:-gcc}" >/dev/null 2>&1; then
  printf 'A native C compiler is required to build embedded SQLite.\n' >&2
  exit 2
fi

mkdir -p -- "$eng_output"
eng_output="$(cd -- "$eng_output" && pwd -P)"
eng_stage="$(mktemp -d "${TMPDIR:-/tmp}/eng-release.XXXXXXXX")"
trap 'rm -rf -- "$eng_stage"' EXIT
eng_package="eng-linux-$eng_goarch"
mkdir -- "$eng_stage/$eng_package"

eng_revision="$(git -C "$eng_tool_root" rev-parse HEAD)"
eng_version="${ENG_VERSION:-$eng_revision}"
if [[ ! $eng_version =~ ^[A-Za-z0-9._+-]+$ ]]; then
  printf 'ENG_VERSION must contain only letters, digits, dots, underscores, pluses or hyphens.\n' >&2
  exit 2
fi
eng_dirty=false
if [[ -n $(git -C "$eng_tool_root" status --porcelain --untracked-files=normal) ]]; then
  eng_dirty=true
fi

(
  cd -- "$eng_tool_root"
  # Git resolves linked worktrees correctly here; BUILD.txt and Version record
  # that identity without depending on Go's directory-based VCS discovery.
  CGO_ENABLED=1 "$eng_go_bin" build -mod=readonly -trimpath -buildvcs=false \
    -ldflags "-X github.com/jiying2007/engineering-platform/tool/internal/local.Version=$eng_version" \
    -o "$eng_stage/$eng_package/eng" ./cmd/eng
)
sed 's|../docs/small-team-migration.md|MIGRATION.md|g' "$eng_tool_root/README.md" > "$eng_stage/$eng_package/README.md"
cp -- "$eng_tool_root/../docs/small-team-migration.md" "$eng_stage/$eng_package/MIGRATION.md"
cp -- "$eng_tool_root/scripts/install.sh" "$eng_stage/$eng_package/install.sh"
cp -R -- "$eng_tool_root/examples" "$eng_stage/$eng_package/examples"
chmod 0755 "$eng_stage/$eng_package/eng" "$eng_stage/$eng_package/install.sh"

eng_goroot="$(cd -- "$eng_tool_root" && "$eng_go_bin" env GOROOT)"
eng_sqlite_module="$(cd -- "$eng_tool_root" && "$eng_go_bin" list -mod=readonly -m -f '{{.Dir}}' github.com/mattn/go-sqlite3)"
eng_yaml_module="$(cd -- "$eng_tool_root" && "$eng_go_bin" list -mod=readonly -m -f '{{.Dir}}' gopkg.in/yaml.v3)"
mkdir -p "$eng_stage/$eng_package/licenses/go" "$eng_stage/$eng_package/licenses/go-sqlite3" "$eng_stage/$eng_package/licenses/yaml"
cp -- "$eng_goroot/LICENSE" "$eng_stage/$eng_package/licenses/go/LICENSE"
cp -- "$eng_sqlite_module/LICENSE" "$eng_stage/$eng_package/licenses/go-sqlite3/LICENSE"
cp -- "$eng_yaml_module/LICENSE" "$eng_yaml_module/NOTICE" "$eng_stage/$eng_package/licenses/yaml/"

{
  printf 'version=%s\n' "$eng_version"
  printf 'source_commit=%s\nsource_modified=%s\n' "$eng_revision" "$eng_dirty"
  printf 'target=linux/%s\nsqlite=embedded-cgo\n' "$eng_goarch"
  printf 'go=%s\n' "$(cd -- "$eng_tool_root" && "$eng_go_bin" version)"
  printf 'libc=%s\n' "$(getconf GNU_LIBC_VERSION)"
  if [[ -r /etc/os-release ]]; then
    cat /etc/os-release
  fi
} > "$eng_stage/$eng_package/BUILD.txt"

(
  cd -- "$eng_stage/$eng_package"
  find . -type f ! -name SHA256SUMS -print0 | LC_ALL=C sort -z | xargs -0 sha256sum > SHA256SUMS
)
tar -czf "$eng_output/$eng_package.tgz" -C "$eng_stage" "$eng_package"
(
  cd -- "$eng_output"
  sha256sum "$eng_package.tgz" > "$eng_package.tgz.sha256"
)
printf '%s\n' "$eng_output/$eng_package.tgz" "$eng_output/$eng_package.tgz.sha256"
