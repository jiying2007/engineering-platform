#!/usr/bin/env python3
"""Negative tests against delivered executables, without rebuilding any role."""
import hashlib
import json
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile

source = Path(sys.argv[1]).resolve(strict=True)
verifier = source / "eng"
for mutation in ("missing-publisher", "wrong-role", "checksum-drift", "extra-file", "symlink"):
    with tempfile.TemporaryDirectory(prefix="ep-distribution-negative-") as tmp:
        destination = Path(tmp) / "bin"
        shutil.copytree(source, destination)
        publisher = destination / "publisher-service"
        if mutation == "missing-publisher":
            publisher.unlink()
        elif mutation == "wrong-role":
            shutil.copyfile(source / "eng", publisher)
            facts = json.loads((destination / "file-manifest.json").read_text())
            for item in facts:
                if item["path"] == "publisher-service":
                    data = publisher.read_bytes()
                    item.update(digest="sha256:" + hashlib.sha256(data).hexdigest(), size=len(data))
            (destination / "file-manifest.json").write_text(json.dumps(facts) + "\n")
            (destination / "SHA256SUMS").write_text("".join(f'{f["digest"][7:]}  {f["path"]}\n' for f in facts))
        elif mutation == "checksum-drift":
            (destination / "SHA256SUMS").write_text("forged\n")
        elif mutation == "extra-file":
            (destination / "unexpected").write_text("extra")
        elif mutation == "symlink":
            publisher.unlink()
            publisher.symlink_to(source / "publisher-service")
        result = subprocess.run([str(verifier), "distribution-verify", "--dir", str(destination)], capture_output=True, timeout=30)
        if result.returncode == 0:
            raise SystemExit(f"unsafe distribution accepted: {mutation}")
        print(f"PASS rejected {mutation}")
