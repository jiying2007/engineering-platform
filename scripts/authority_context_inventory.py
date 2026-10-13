#!/usr/bin/env python3
"""Emit bounded, non-secret static review candidates, not a call-graph proof."""
from __future__ import annotations
import argparse
import hashlib
import json
from pathlib import Path
import os
import shutil
import stat
import subprocess

MAX_TOTAL_SOURCE_BYTES = 64 << 20
MAX_SOURCE_FILES = 10_000
MAX_SOURCE_BYTES = 2 << 20


def _read_source(path: Path) -> bytes:
    # Bound allocation before parsing; a renamed FIFO must not wait for a writer.
    flags = os.O_RDONLY | getattr(os, "O_CLOEXEC", 0)
    flags |= getattr(os, "O_NOFOLLOW", 0) | getattr(os, "O_NONBLOCK", 0)
    try:
        with os.fdopen(os.open(path, flags), "rb") as stream:
            before = os.fstat(stream.fileno())
            if not stat.S_ISREG(before.st_mode) or before.st_size > MAX_SOURCE_BYTES:
                raise ValueError("bounded regular Go source required")
            raw = stream.read(MAX_SOURCE_BYTES + 1)
            after = os.fstat(stream.fileno())
            if (len(raw) > MAX_SOURCE_BYTES or len(raw) != before.st_size or
                    before.st_size != after.st_size or before.st_mtime_ns != after.st_mtime_ns):
                raise ValueError("Go source changed during read")
            return raw
    except OSError as exc:
        raise ValueError("Go source could not be opened safely") from exc


def inventory(root: Path) -> dict:
    source_hash = hashlib.sha256()
    sources = []
    total = 0
    for part in ("cmd", "internal"):
        folder = root / part
        if not folder.is_dir():
            continue
        for path in sorted(folder.rglob("*.go")):
            if path.name.endswith("_test.go") or path.is_symlink():
                continue
            raw = _read_source(path)
            total += len(raw)
            if len(raw) > MAX_SOURCE_BYTES or total > MAX_TOTAL_SOURCE_BYTES or len(sources) >= MAX_SOURCE_FILES:
                raise ValueError("source exceeds inventory budget")
            relative = path.relative_to(root).as_posix()
            source_hash.update(relative.encode() + b"\0" + raw + b"\0")
            sources.append({"path": relative, "text": raw.decode("utf-8")})
    go = shutil.which("go")
    if not go:
        raise ValueError("Go AST frontend requires the configured Go toolchain")
    # Single standard-library source file: never download a toolchain/module,
    # run source from the scanned checkout, or interpolate shell arguments.
    environment = dict(os.environ, GO111MODULE="off", GOWORK="off", GOTOOLCHAIN="local", GOFLAGS="")
    tool = Path(__file__).with_name("context_inventory_ast.go").resolve()
    try:
        proc = subprocess.run([go, "run", str(tool)], input=json.dumps(sources, ensure_ascii=False).encode(),
                              capture_output=True, env=environment, timeout=120)
    except (OSError, subprocess.TimeoutExpired) as exc:
        raise ValueError("Go AST frontend did not complete") from exc
    if proc.returncode:
        raise ValueError("Go AST inventory rejected source: " + proc.stderr.decode("utf-8", "replace")[:1000])
    findings = json.loads(proc.stdout)
    if not isinstance(findings, list) or len(findings) > 20_000:
        raise ValueError("invalid Go AST result")
    return {"version": 1, "status": "STATIC_CANDIDATES_NOT_CALL_GRAPH_PROOF",
            "scanner": "GO_AST_IMPORT_BOUND_REFERENCES_V1",
            "source_files_digest": "sha256:" + source_hash.hexdigest(),
            "context_or_call_candidates": findings, "issue_229_closed": False,
            "limitations": ["No dynamic points-to or interprocedural lifetime proof",
                            "Dot context imports rejected, not inferred across files",
                            "Background may be authorized maintenance",
                            "Legacy names may refer to other interfaces",
                            "No production latency or lifecycle qualification"]}

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
