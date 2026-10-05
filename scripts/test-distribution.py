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


# Exercise the actual shipped installer, never a substitute local build. The
# original distribution is copied, then removed before installed-byte readback.
def invoke(binary, *args, ok=True):
    result = subprocess.run([str(binary), *args], capture_output=True, timeout=60)
    if (result.returncode == 0) != ok:
        raise AssertionError((args, result.returncode, result.stderr.decode()))
    return json.loads(result.stdout) if ok and result.stdout else None

verified = invoke(verifier, "distribution-verify", "--dir", str(source))
revision = verified["source_commit"]
with tempfile.TemporaryDirectory(prefix="ep-installed-byte-test-") as tmp:
    root = Path(tmp)
    original = root / "original"
    shutil.copytree(source, original)
    installed = root / "installed"
    report = invoke(original / "eng", "distribution-install", "--from", str(original),
                    "--into", str(installed), "--source-commit", revision)
    assert report["status"] == "INSTALLED_BYTES_VERIFIED"
    assert report["binary_count"] == 6 and report["template_count"] == 21
    for claim in ("services_started", "configuration_applied", "dependencies_included",
                  "execution_authorized", "production_qualified"):
        assert report[claim] is False
    shutil.rmtree(original)
    active = installed / "bin" / "eng"
    readback = invoke(active, "installation-verify", "--dir", str(installed),
                      "--source-commit", revision)
    assert readback["manifest_digest"] == report["manifest_digest"]
    assert subprocess.run([str(active), "--help"], capture_output=True).returncode == 0
    print("PASS actual installed runtime/templates survive original removal")
    invoke(verifier, "distribution-install", "--from", str(source), "--into", str(installed),
           "--source-commit", revision, ok=False)
    invoke(active, "installation-verify", "--dir", str(installed),
           "--source-commit", "0" * 40, ok=False)
    for extra in ("--enable", "--migrate", "--execute", "--force"):
        invoke(verifier, "distribution-install", "--from", str(source),
               "--into", str(root / "forbidden"), "--source-commit", revision, extra, ok=False)
        assert not (root / "forbidden").exists()
    for mutation in ("template-bytes", "template-mode", "extra-file", "extra-directory",
                     "missing-template", "manifest-drift", "binary-hardlink"):
        target = root / mutation
        shutil.copytree(installed, target)
        template = target / "templates" / "control.env.example"
        if mutation == "template-bytes":
            template.chmod(0o644)
            template.write_text("tampered")
            template.chmod(0o444)
        elif mutation == "template-mode":
            template.chmod(0o644)
        elif mutation == "extra-file":
            (target / "unexpected").write_text("extra")
        elif mutation == "extra-directory":
            (target / "unexpected").mkdir()
        elif mutation == "missing-template":
            template.unlink()
        elif mutation == "manifest-drift":
            manifest = target / "installation-manifest.json"
            manifest.chmod(0o644)
            manifest.write_text("{}")
            manifest.chmod(0o444)
        elif mutation == "binary-hardlink":
            (root / "hardlink").hardlink_to(target / "bin" / "worker")
        invoke(active, "installation-verify", "--dir", str(target), "--source-commit", revision, ok=False)
        print("PASS rejected installed " + mutation)
