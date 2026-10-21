#!/usr/bin/env python3
"""Reject mutable external uses references in the repository's workflow format."""

import re
import sys
from pathlib import Path

USES = re.compile(r"^\s*(?:-\s+)?(?:uses|'uses'|\"uses\"):\s*(.*?)\s*$")
ANY_USES = re.compile(r"(?:^|[\s{,])(?:uses|'uses'|\"uses\")\s*:")
ACTION = re.compile(r"[A-Za-z0-9_.-]+/[A-Za-z0-9_./-]+@[0-9a-f]{40}")
CONTAINER = re.compile(r"docker://[^\s@]+@sha256:[0-9a-f]{64}")


def violations(text):
    result = []
    for number, line in enumerate(text.splitlines(), 1):
        if line.lstrip().startswith("#"):
            continue
        match = USES.fullmatch(line)
        if not match:
            if ANY_USES.search(line):
                result.append((number, "uses must be a direct single-line scalar"))
            continue
        value = match.group(1).split("#", 1)[0].strip()
        if len(value) >= 2 and value[0] == value[-1] and value[0] in "\"'":
            value = value[1:-1]
        if "${{" in value:
            result.append((number, "dynamic uses identity is not allowed"))
        elif value.startswith("./"):
            # Repository-local actions are bound to the exact checked-out source.
            continue
        elif not ACTION.fullmatch(value) and not CONTAINER.fullmatch(value):
            result.append((number, "external uses requires a full commit SHA or image digest"))
    return result


def main(argv):
    root = Path(argv[1]) if len(argv) == 2 else Path(".github/workflows")
    paths = sorted(set(root.glob("*.yml")) | set(root.glob("*.yaml")))
    if not paths:
        print("no workflow files found", file=sys.stderr)
        return 1
    errors = []
    for path in paths:
        errors.extend(f"{path}:{line}: {message}" for line, message in violations(path.read_text()))
    if errors:
        print("\n".join(errors), file=sys.stderr)
        return 1
    print(f"Verified immutable external uses references in {len(paths)} workflows")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
