#!/usr/bin/env python3
"""Read-only drift guard for semantically superseded divergent prototype refs."""
import argparse
import json
from pathlib import Path
import re
import subprocess
import sys

REPOSITORY = "jiying2007/engineering-platform"
SHA = re.compile(r"[0-9a-f]{40}\Z")
DISPOSITION = "RETAINED_SUPERSEDED_PROTOTYPE"
NAMES = (
    "docs/rc-status-after-terminal-v3",
    "feat/github-ci-core-evidence-import",
    "feat/independent-artifact-mirror",
    "feat/independent-publisher-service",
    "feat/independent-review-authority",
    "feat/relay-codex-config-renderer",
)

def require(value, message):
    if not value:
        raise ValueError(message)

def pairs(entries):
    result = {}
    for key, value in entries:
        require(key not in result, "duplicate manifest key")
        result[key] = value
    return result

def validate(value):
    require(isinstance(value, dict) and set(value) == {"version", "repository", "refs"}, "invalid prototype-ref manifest")
    require(type(value["version"]) is int and value["version"] == 1 and value["repository"] == REPOSITORY, "invalid prototype-ref authority")
    refs = value["refs"]
    require(isinstance(refs, list) and len(refs) == len(NAMES), "exact retained prototype list required")
    names = []
    seen = set()
    for entry in refs:
        require(isinstance(entry, dict) and set(entry) == {"branch", "expected_head", "disposition"}, "invalid retained prototype entry")
        name, sha, disposition = entry["branch"], entry["expected_head"], entry["disposition"]
        require(isinstance(name, str) and name not in seen, "duplicate retained prototype branch")
        require(isinstance(sha, str) and SHA.fullmatch(sha), "exact retained prototype SHA required")
        require(disposition == DISPOSITION, "prototype disposition must stay frozen")
        seen.add(name)
        names.append(name)
    require(tuple(names) == NAMES, "retained prototype names/order drift")
    return value

def remote_inventory(root, names):
    refs = ["refs/heads/" + name for name in names]
    result = subprocess.run(
        ["git", "-C", str(root), "ls-remote", "--heads", "origin", *refs],
        capture_output=True, text=True, timeout=60,
    )
    require(result.returncode == 0, "cannot read retained prototype refs")
    out = {}
    for line in result.stdout.splitlines():
        sha, ref = line.split()
        require(SHA.fullmatch(sha) and ref.startswith("refs/heads/"), "invalid prototype ref inventory")
        name = ref.removeprefix("refs/heads/")
        require(name in names and name not in out, "unexpected or duplicate prototype ref")
        out[name] = sha
    return out

def verify(root, value):
    validate(value)
    expected = {entry["branch"]: entry["expected_head"] for entry in value["refs"]}
    actual = remote_inventory(root, set(expected))
    require(actual == expected, "retained prototype ref drift")
    return {
        "version": 1,
        "repository": REPOSITORY,
        "status": "RETAINED_PROTOTYPE_REFS_EXACT",
        "refs": [{"branch": name, "head": actual[name], "disposition": DISPOSITION} for name in NAMES],
        "mutation_attempted": False,
    }

def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--manifest", required=True)
    args = parser.parse_args()
    try:
        root = Path.cwd().resolve()
        path = Path(args.manifest).absolute()
        require(path == path.resolve() and path.is_file() and 0 < path.stat().st_size <= 16384, "bounded non-aliased manifest required")
        value = json.loads(path.read_text(), object_pairs_hook=pairs)
        print(json.dumps(verify(root, value), sort_keys=True, indent=2))
        return 0
    except (ValueError, OSError, subprocess.SubprocessError, KeyError, TypeError) as error:
        print(json.dumps({"version": 1, "status": "REJECTED", "reason": str(error), "mutation_attempted": False}, sort_keys=True, indent=2))
        return 1

if __name__ == "__main__":
    sys.exit(main())
