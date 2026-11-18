#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage: install.sh [--bin-dir DIRECTORY] [--replace]

Install eng from this extracted release package. The default destination is
$HOME/.local/bin. This script verifies the package and native architecture;
it does not download files, change PATH, configure a runtime, or invoke a model.

Before --replace: stop accepting tasks, resolve active executions, and back up
the matching data directory. Installation never migrates or deletes task data.
USAGE
}

eng_bin_dir="${HOME:?HOME must be set}/.local/bin"
eng_replace=false
while (( $# )); do
  case "$1" in
    --bin-dir)
      if (( $# < 2 )) || [[ -z $2 ]]; then
        printf '%s\n' '--bin-dir requires a directory.' >&2
        exit 2
      fi
      eng_bin_dir=$2
      shift 2
      ;;
    --replace) eng_replace=true; shift ;;
    --help|-h) usage; exit 0 ;;
    *) printf 'Unknown option: %s\n' "$1" >&2; usage >&2; exit 2 ;;
  esac
done

eng_package_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
if [[ ! -f $eng_package_dir/eng || ! -f $eng_package_dir/SHA256SUMS || ! -f $eng_package_dir/BUILD.txt ]]; then
  printf 'Run install.sh from a complete extracted eng release package.\n' >&2
  exit 2
fi
if [[ $(uname -s) != Linux ]]; then
  printf 'This release requires Linux.\n' >&2
  exit 2
fi
case "$(uname -m)" in
  x86_64) eng_host_arch=amd64 ;;
  aarch64|arm64) eng_host_arch=arm64 ;;
  *) printf 'Unsupported host architecture.\n' >&2; exit 2 ;;
esac
eng_target="$(sed -n 's/^target=//p' "$eng_package_dir/BUILD.txt")"
if [[ $eng_target != "linux/$eng_host_arch" ]]; then
  printf 'Package target %s does not match linux/%s.\n' "$eng_target" "$eng_host_arch" >&2
  exit 2
fi
(
  cd -- "$eng_package_dir"
  sha256sum --check --status SHA256SUMS
) || { printf 'Package checksum verification failed.\n' >&2; exit 2; }

mkdir -p -- "$eng_bin_dir"
eng_bin_dir="$(cd -- "$eng_bin_dir" && pwd -P)"
eng_destination="$eng_bin_dir/eng"
if [[ -L $eng_destination || ( -e $eng_destination && ! -f $eng_destination ) ]]; then
  printf 'Destination must be a regular file or absent: %s\n' "$eng_destination" >&2
  exit 2
fi
if [[ -e $eng_destination && $eng_replace != true ]]; then
  printf 'eng already exists at %s; use --replace after stopping tasks and backing up.\n' "$eng_destination" >&2
  exit 2
fi
eng_install_tmp="$(mktemp "$eng_bin_dir/.eng-install.XXXXXXXX")"
trap 'rm -f -- "$eng_install_tmp"' EXIT
cp -- "$eng_package_dir/eng" "$eng_install_tmp"
chmod 0755 "$eng_install_tmp"
mv -f -- "$eng_install_tmp" "$eng_destination"
printf 'Installed %s\n' "$eng_destination"
printf 'Run "%s doctor" to check the local environment without a model call.\n' "$eng_destination"
printf 'If needed, add %s to your shell PATH.\n' "$eng_bin_dir"
