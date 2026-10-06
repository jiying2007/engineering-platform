#!/usr/bin/env python3
"""Verify immutable retained evidence refs without mutating GitHub."""
import argparse
import json
from pathlib import Path
import re
import subprocess
import sys

REPOSITORY = "jiying2007/engineering-platform"
REF = re.compile(r"engineering-platform/[0-9a-f]{24}\Z")
SHA = re.compile(r"[0-9a-f]{40}\Z")

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
    require(isinstance(value, dict) and set(value) == {"version", "repository", "refs"}, "invalid retained-ref manifest")
    require(type(value["version"]) is int and value["version"] == 1 and value["repository"] == REPOSITORY, "invalid retained-ref authority")
    refs = value["refs"]
    require(isinstance(refs, list) and 0 < len(refs) <= 16, "bounded retained-ref list required")
    last = ""
    seen = set()
    for entry in refs:
        require(isinstance(entry, dict) and set(entry) == {"branch", "expected_head"}, "invalid retained-ref entry")
        name, sha = entry["branch"], entry["expected_head"]
        require(isinstance(name, str) and REF.fullmatch(name) and name > last and name not in seen, "canonical ordered retained ref required")
        require(isinstance(sha, str) and SHA.fullmatch(sha), "exact retained-ref SHA required")
        seen.add(name)
        last = name
    return value

def remote_inventory(root):
    result = subprocess.run(
        ["git", "-C", str(root), "ls-remote", "--heads", "origin", "refs/heads/engineering-platform/*"],
        capture_output=True, text=True, timeout=60,
    )
    require(result.returncode == 0, "cannot read retained evidence refs")
    out = {}
    for line in result.stdout.splitlines():
        sha, ref = line.split()
        require(SHA.fullmatch(sha) and ref.startswith("refs/heads/engineering-platform/"), "invalid retained ref inventory")
        name = ref.removeprefix("refs/heads/")
        require(name not in out, "duplicate retained remote ref")
        out[name] = sha
    return out

def verify(root, value):
    validate(value)
    expected = {entry["branch"]: entry["expected_head"] for entry in value["refs"]}
    actual = remote_inventory(root)
    require(actual == expected, "retained evidence ref inventory drift")
    return {
        "version": 1,
        "repository": REPOSITORY,
        "status": "RETAINED_EVIDENCE_REFS_EXACT",
        "refs": [{"branch": name, "head": actual[name]} for name in sorted(actual)],
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
