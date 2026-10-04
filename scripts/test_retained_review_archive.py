#!/usr/bin/env python3
"""Offline regression; altered fixtures below are tests, never retained proof."""
import copy
import hashlib
import json
import os
from pathlib import Path
import stat
import subprocess
import sys
import tempfile
import unittest
import warnings
import zipfile

import retained_review_archive as archive

ROOT = Path(__file__).resolve().parents[1]
FACTS = ROOT / "docs/evidence/m1-terminal-facts"


class ArchiveTests(unittest.TestCase):
    def setUp(self):
        self.raw = (FACTS / "debug-closure.json.gz").read_bytes()
        self.good = archive.decode(archive.payload(self.raw))

    def check(self, value):
        raw = archive.canonical(value)
        return archive.verify(raw, archive.digest(raw))

    def replace(self, value, path, mutate):
        entry = next(e for e in value["members"] if e["path"] == path)
        doc = archive.decode(entry["utf8"].encode())
        mutate(doc)
        data = archive.canonical(doc)
        entry.update(utf8=data.decode(), digest=archive.digest(data), size=len(data))

    def directory(self, root, value=None):
        for entry in (value or self.good)["members"]:
            path = root / entry["path"]
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text(entry["utf8"])

    def test_retained_readback_and_pinned_manifest(self):
        lines = (FACTS / "SHA256SUMS").read_text().splitlines()
        self.assertEqual(len(lines), 2)
        names = set()
        for line in lines:
            checksum, name = line.split("  ")
            self.assertIn(name, ("feature-closure.json.gz", "debug-closure.json.gz"))
            self.assertNotIn(name, names)
            names.add(name)
            data = (FACTS / name).read_bytes()
            result = archive.verify(data, "sha256:" + checksum)
            self.assertEqual(result["member_count"], 14)
            self.assertEqual(result["binding"]["closure"], "CLOSED")
            self.assertFalse(result["production_qualified"])
            self.assertFalse(result["runtime_restore_supported"])

    def test_missing_extra_duplicate_or_noncanonical(self):
        for mutate in (lambda a: a["members"].pop(),
                       lambda a: a["members"].append(copy.deepcopy(a["members"][0])),
                       lambda a: a["members"][0].update(path="stack/pki/ca.key"),
                       lambda a: a.update(production_qualified=True),
                       lambda a: a["members"].reverse()):
            value = copy.deepcopy(self.good)
            mutate(value)
            with self.assertRaises(ValueError):
                self.check(value)
        with self.assertRaises(ValueError):
            archive.verify(self.raw + b"\n", archive.digest(self.raw + b"\n"))
        with self.assertRaises(ValueError):
            archive.verify(self.raw, "sha256:" + "0" * 64)
        with self.assertRaises(ValueError):
            archive.decode(b'{"x":1,"x":2}')

    def test_cross_bindings_reject_even_with_recomputed_member_hashes(self):
        changes = [
            ("closure-response.json", lambda d: d.update(result="OPEN")),
            ("closure-response.json", lambda d: d.update(subject_digest="sha256:" + "0" * 64)),
            ("review-state.json", lambda d: d.update(reviewer_actor=d["engineering_actor"])),
            ("review-state.json", lambda d: d.update(decision_comment_id=1)),
            ("verification/verification-state.json", lambda d: d.update(result_commit="0" * 40)),
            ("verification/verification-response.json", lambda d: d.update(evidence_ids=[])),
            ("verification/codex-receipt-digest.json", lambda d: d.update(bundle_digest="sha256:" + "0" * 64)),
            ("work-closed.json", lambda d: d.update(state="OPEN")),
        ]
        for name, mutate in changes:
            with self.subTest(name=name):
                value = copy.deepcopy(self.good)
                self.replace(value, name, mutate)
                with self.assertRaises(ValueError):
                    self.check(value)

    def test_reject_credentials_in_selected_facts(self):
        for field in ({"access_token": "example-test-only"},
                      {"known_limits": ["-----BEGIN PRIVATE KEY-----"]},
                      {"known_limits": ["Bearer example-test-only"]}):
            value = copy.deepcopy(self.good)
            self.replace(value, "review-response.json", lambda d: d.update(field))
            with self.assertRaises(ValueError):
                self.check(value)
        value = copy.deepcopy(self.good)
        def poison(doc):
            observed = json.loads(doc["observed_state"])
            observed["refresh_token"] = "test-only"
            doc["observed_state"] = json.dumps(observed)
        self.replace(value, "verification/engineering/publication-receipt.json", poison)
        with self.assertRaises(ValueError):
            self.check(value)

    def test_current_state_provider_binding_is_not_qualification(self):
        value = copy.deepcopy(self.good)
        provider = {"version": 1, "provider_id": "TEST_ONLY_NOT_ADMITTED"}
        for name in ("review-state.json", "verification/verification-state.json", "verification/engineering/engineering-state.json"):
            self.replace(value, name, lambda d: d.update(version=2, provider=provider))
        result = self.check(value)
        self.assertFalse(result["production_qualified"])
        self.replace(value, "review-state.json", lambda d: d.update(provider={"provider_id": "OTHER_TEST_ONLY"}))
        with self.assertRaises(ValueError):
            self.check(value)

    def test_failed_review_retains_no_closure(self):
        value = copy.deepcopy(self.good)
        self.replace(value, "review-state.json", lambda d: d.update(review_result="FAIL", closure="BLOCKED"))
        self.replace(value, "review-response.json", lambda d: d.update(result="FAIL"))
        self.replace(value, "work-closed.json", lambda d: d.update(state="RUNNING"))
        for entry in value["members"]:
            if entry["path"] == "work-closed.json":
                entry["path"] = "work-after-fail-review.json"
        value["members"] = sorted([e for e in value["members"] if e["path"] != "closure-response.json"], key=lambda e: e["path"])
        members = {e["path"]: e["utf8"].encode() for e in value["members"]}
        value["binding"] = archive.binding(members)
        result = self.check(value)
        self.assertEqual(result["binding"]["closure"], "BLOCKED")
        self.assertEqual(result["member_count"], 13)

    def test_directory_pack_excludes_stack_and_is_immutable(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            self.directory(root)
            (root / "stack/pki").mkdir(parents=True)
            (root / "stack/pki/ca.key").write_text("-----BEGIN PRIVATE KEY-----")
            (root / "core-review.dump").write_bytes(b"TEST_ONLY")
            out = root / "facts.json"
            cmd = [sys.executable, "-B", str(ROOT / "scripts/retained_review_archive.py"), "pack", "--source", str(root), "--review-run-id", "1", "--out", str(out)]
            completed = subprocess.run(cmd, capture_output=True, text=True)
            self.assertEqual(completed.returncode, 0, completed.stderr)
            self.assertEqual(stat.S_IMODE(out.stat().st_mode), 0o600)
            raw = out.read_bytes()
            self.assertNotIn(b"PRIVATE KEY", raw)
            self.assertNotIn(b"core-review.dump", raw)
            self.assertEqual(subprocess.run(cmd, capture_output=True).returncode, 1)
            self.assertEqual(out.read_bytes(), raw)
            self.assertTrue((root / "stack/pki/ca.key").exists())

    def test_directory_symlink_rejected(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            self.directory(root)
            original = root / "review-state.json"
            original.rename(root / "other.json")
            original.symlink_to(root / "other.json")
            with self.assertRaises(ValueError):
                archive.build(root, {"kind": "workflow-directory", "review_run_id": 1})

    def test_zip_identity_traversal_duplicates_and_symlinks(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / "input.zip"
            def build(extra):
                with warnings.catch_warnings():
                    warnings.simplefilter("ignore", UserWarning)
                    with zipfile.ZipFile(path, "w") as z:
                        for entry in self.good["members"]:
                            z.writestr(entry["path"], entry["utf8"])
                        extra(z)
                origin = {"kind": "github-actions-zip", "review_run_id": 1, "artifact_id": 1, "archive_digest": archive.digest(path.read_bytes())}
                return archive.build(path, origin)
            clean = build(lambda z: z.writestr("stack/pki/ca.key", "-----BEGIN PRIVATE KEY-----"))
            self.assertEqual(len(archive.decode(clean)["members"]), 14)
            for name in ("../escape", "/absolute", "a\\b", "review-state.json"):
                with self.subTest(name=name), self.assertRaises(ValueError):
                    build(lambda z: z.writestr(name, "{}"))
            def link(z):
                info = zipfile.ZipInfo("stack/link")
                info.create_system = 3
                info.external_attr = (stat.S_IFLNK | 0o777) << 16
                z.writestr(info, "outside")
            with self.assertRaises(ValueError):
                build(link)
            with self.assertRaises(ValueError):
                archive.build(path, {"kind": "github-actions-zip", "review_run_id": 1, "artifact_id": 1, "archive_digest": "sha256:" + "0" * 64})

    def test_terminal_workflow_has_no_raw_directory_upload(self):
        workflow = (ROOT / ".github/workflows/retained-pilot-review.yml").read_text()
        self.assertNotIn("path: ${{ runner.temp }}/retained-pilot-review\n", workflow)
        self.assertIn("path: ${{ runner.temp }}/retained-pilot-review-facts\n", workflow)
        self.assertIn("ref: ${{ github.sha }}", workflow)
        self.assertLess(workflow.index("retained_review_archive.py pack"), workflow.index("uses: actions/upload-artifact"))
        block = workflow[workflow.index("- name: Retain terminal review facts only"):workflow.index("- name: Summarize")]
        self.assertNotIn("always()", block)
        self.assertIn("if-no-files-found: error", block)
        verify = (ROOT / "examples/pilots/actions/verify-retained.sh").read_text()
        cleanup = verify[verify.index("cleanup() {"):verify.index("trap cleanup EXIT")]
        self.assertIn('rm -rf -- "$STACK_ROOT"', cleanup)
        self.assertLess(cleanup.index('wait "$CONTROL_PID"'), cleanup.index('rm -rf -- "$STACK_ROOT"'))


if __name__ == "__main__":
    unittest.main()
