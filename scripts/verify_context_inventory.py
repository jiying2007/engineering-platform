#!/usr/bin/env python3
"""Bind the reviewed direct Context inventory to current source; no closure claim."""
import argparse
from collections import Counter
from pathlib import Path
import re
from authority_context_inventory import inventory

CATEGORIES = {
    "Explicitly timed command root", "Signal-owned command/service root",
    "Command with downstream budget", "Independent bounded cleanup/settlement",
    "Test support helper",
}
ROW = re.compile(r"^\| `([^`]+):(\d+)` \| `(Background|TODO|WithoutCancel)` \| ([^|]+) \|$", re.M)

def verify(root: Path, document: Path) -> dict:
    text = document.read_text(encoding="utf-8")
    current = inventory(root)
    digests = re.findall(r"`(sha256:[0-9a-f]{64})`", text)
    if digests != [current["source_files_digest"]]:
        raise ValueError("reviewed Context source digest is stale or ambiguous")
    expected = [(f["path"], f["line"], f["name"]) for f in current["context_or_call_candidates"] if f["kind"] == "context_constructor"]
    rows = [(path, int(line), name, category.strip()) for path, line, name, category in ROW.findall(text)]
    if [(p, line, name) for p, line, name, _ in rows] != expected:
        raise ValueError("reviewed Context rows are missing, duplicated or stale")
    counts = Counter(category for _, _, _, category in rows)
    if set(counts) - CATEGORIES:
        raise ValueError("unreviewed Context lifecycle category")
    declared = re.findall(r"^\| ([^|]+) \| (\d+) \|$", text, re.M)
    summary = [(category.strip(), int(count)) for category, count in declared]
    if len(summary) != len(CATEGORIES) or set(category for category, _ in summary) != CATEGORIES or dict(summary) != {c: counts[c] for c in CATEGORIES}:
        raise ValueError("Context category counts disagree with reviewed rows")
    return {"version": 1, "status": "EXACT_DIRECT_CONTEXT_INVENTORY_VERIFIED",
            "source_files_digest": current["source_files_digest"], "direct_sites": len(rows),
            "interprocedural_proof": False, "issue_229_closed": False}

if __name__ == "__main__":
    import json
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--root", required=True, type=Path)
    args = parser.parse_args()
    try:
        print(json.dumps(verify(args.root, args.root / "docs/implementation/CONTEXT_LIFECYCLE_INVENTORY_V1.md"), indent=2))
    except (OSError, ValueError) as exc:
        raise SystemExit(f"Context inventory rejected: {exc}")
