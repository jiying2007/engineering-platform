#!/usr/bin/env bash
set -euo pipefail
# Run from a trusted checkout. Do not silently replace a previously installed eng.
source_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)"
target="${ENG_BIN_DIR:-$HOME/.local/bin}"
replace="${1:-}"
if [[ "$replace" != "" && "$replace" != "--replace" ]]; then
  printf 'usage: bash scripts/install.sh [--replace]\n' >&2
  exit 2
fi
command -v python3 >/dev/null || { printf 'Python 3 is required\n' >&2; exit 1; }
command -v git >/dev/null || { printf 'Git is required\n' >&2; exit 1; }
mkdir -p -- "$target"
if [[ -e "$target/eng" && "$replace" != "--replace" ]]; then
  printf 'eng already exists at %s; use --replace only after stopping old tasks and backing up data\n' "$target/eng" >&2
  exit 1
fi
if [[ -e "$target/eng" && "$replace" == "--replace" ]]; then
  cp -p -- "$target/eng" "$target/eng.before-v0.$(date -u +%Y%m%dT%H%M%SZ)"
fi
install -m 0755 -- "$source_dir/eng" "$target/eng"
printf 'Installed eng V0 at %s. No model or service was started.\n' "$target/eng"
