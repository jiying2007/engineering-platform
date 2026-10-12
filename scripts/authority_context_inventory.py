#!/usr/bin/env python3
"""Emit bounded, non-secret static review candidates, not a call-graph proof."""
from __future__ import annotations
import argparse
import hashlib
import json
from pathlib import Path
import re
import subprocess

CONTEXT = re.compile(r"\bcontext\.(Background|TODO|WithoutCancel)\s*\(")
LEGACY = re.compile(r"\.(CreateWork|GetWork|UpdateWork|GetRecovery|BeginRecovery|CompleteRecovery|GetTask|GetTaskByDigest|GetExecution|GetAttempt|GetTaskRevision|GetRunInputByDigest|CreateExecutionAndUpdateWork|UpdateExecution|RecordSteering|GetSteering|UpdateExecutionAndWork|CreateCheckpoint|GetCheckpoint|CreateDelivery|GetDelivery|CreateEvidence|GetEvidence|CreateVerification|GetVerification|CreateReviewAndUpdateWork|GetReview|CreateClosureAndUpdateWork|GetClosure)\s*\(")

def inventory(root: Path) -> dict:
    findings = []
    source_hash = hashlib.sha256()
    for part in ("cmd", "internal"):
        folder = root / part
        if not folder.is_dir():
            continue
        for path in sorted(folder.rglob("*.go")):
            if path.name.endswith("_test.go") or path.is_symlink():
                continue
            raw = path.read_bytes()
            if len(raw) > 2 << 20:
                raise ValueError("source file exceeds inventory budget")
            relative = path.relative_to(root).as_posix()
            source_hash.update(relative.encode() + b"\0" + raw + b"\0")
            for number, line in enumerate(raw.decode("utf-8").splitlines(), 1):
                # Candidates only: comments/aliases/indirect calls need review.
                if line.lstrip().startswith("//"):
                    continue
                for regex, kind in ((CONTEXT, "context_constructor"), (LEGACY, "unscoped_name_candidate")):
                    for match in regex.finditer(line):
                        findings.append({"path": relative, "line": number, "kind": kind, "name": match.group(1)})
    return {"version": 1, "status": "STATIC_CANDIDATES_NOT_CALL_GRAPH_PROOF",
            "source_files_digest": "sha256:" + source_hash.hexdigest(),
            "context_or_call_candidates": findings, "issue_229_closed": False,
            "limitations": ["No alias/interprocedural or pool-wait proof", "Background may be authorized maintenance", "Candidate names may refer to other interfaces", "No production latency or lifecycle qualification"]}

def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--root", type=Path, required=True)
    parser.add_argument("--out", type=Path, required=True)
    args = parser.parse_args()
    root = args.root.resolve()
    report = inventory(root)
    try:
        report["git_head"] = subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=root, text=True).strip()
        report["git_dirty"] = bool(subprocess.check_output(["git", "status", "--porcelain"], cwd=root))
    except subprocess.CalledProcessError:
        report["git_head"] = None
        report["git_dirty"] = None
    args.out.write_text(json.dumps(report, indent=2) + "\n")
    print(f"Context review candidates: {len(report['context_or_call_candidates'])}; not #229 closure")

if __name__ == "__main__":
    main()
