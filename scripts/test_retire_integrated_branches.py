#!/usr/bin/env python3
"""Real local Git fixtures only: no GitHub mutation or model calls."""
import copy
import json
from pathlib import Path
import subprocess
import tempfile
import unittest

import retire_integrated_branches as retire

ROOT = Path(__file__).resolve().parents[1]


class RetirementTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name) / "work"
        self.remote = Path(self.temp.name) / "remote.git"
        subprocess.run(["git", "init", "--bare", str(self.remote)], check=True, capture_output=True)
        subprocess.run(["git", "init", "-b", "main", str(self.root)], check=True, capture_output=True)
        self.git("config", "user.name", "test-only")
        self.git("config", "user.email", "test@example.invalid")
        self.git("remote", "add", "origin", str(self.remote))
        (self.root / "data").write_text("base\n")
        self.git("add", "data")
        self.git("commit", "-m", "base")
        self.base = self.git("rev-parse", "HEAD")
        (self.root / "data").write_text("integrated\n")
        self.git("add", "data")
        self.tree = self.git("write-tree")
        self.topic = self.git("commit-tree", self.tree, "-p", self.base, "-m", "topic")
        self.main = self.git("commit-tree", self.tree, "-p", self.base, "-m", "squashed integration")
        self.git("reset", "--hard", self.main)
        self.git("branch", "feat/one", self.topic)
        self.git("branch", "fix/two", self.topic)
        self.git("push", "origin", "main", "feat/one", "fix/two")
        self.manifest = {"version": 1, "repository": retire.REPOSITORY, "branches": [
            {"branch": name, "expected_head": self.topic, "integrated_commit": self.main, "tree": self.tree}
            for name in ("feat/one", "fix/two")]}

    def git(self, *args):
        return retire.git(self.root, *args)

    def test_dry_run_then_atomic_delete_and_idempotent_readback(self):
        result = retire.retire(self.root, self.manifest, self.main)
        self.assertFalse(result["mutation_attempted"])
        self.assertIn("refs/heads/feat/one", retire.inventory(self.root))
        result = retire.retire(self.root, self.manifest, self.main, apply=True)
        self.assertTrue(result["mutation_attempted"])
        self.assertTrue(all(e["state"] == "DELETED_READBACK_VERIFIED" for e in result["branches"]))
        self.assertEqual(retire.inventory(self.root), {"refs/heads/main": self.main})
        result = retire.retire(self.root, self.manifest, self.main, apply=True)
        self.assertFalse(result["mutation_attempted"])
        self.assertTrue(all(e["state"] == "ABSENT" for e in result["branches"]))

    def test_atomic_lease_blocks_parallel_update_without_partial_deletion(self):
        plan = retire.plan(self.root, self.manifest, self.main)
        newer = self.git("commit-tree", self.tree, "-p", self.topic, "-m", "parallel update")
        self.git("push", "origin", newer + ":refs/heads/fix/two")
        with self.assertRaises(ValueError):
            self.git(*retire.push_arguments(plan))
        heads = retire.inventory(self.root)
        self.assertEqual(heads["refs/heads/feat/one"], self.topic)
        self.assertEqual(heads["refs/heads/fix/two"], newer)

    def test_stale_main_or_candidate_is_rejected_before_deleting(self):
        with self.assertRaises(ValueError):
            retire.retire(self.root, self.manifest, self.base, apply=True)
        bad = copy.deepcopy(self.manifest)
        bad["branches"][1]["expected_head"] = self.base
        with self.assertRaises(ValueError):
            retire.retire(self.root, bad, self.main, apply=True)
        self.assertEqual(len(retire.inventory(self.root)), 3)

    def test_unintegrated_tree_or_proof_is_rejected(self):
        for change in ({"tree": self.git("rev-parse", self.base + "^{tree}")},
                       {"integrated_commit": self.topic}):
            bad = copy.deepcopy(self.manifest)
            bad["branches"][0].update(change)
            with self.assertRaises(ValueError):
                retire.retire(self.root, bad, self.main, apply=True)
        self.assertEqual(len(retire.inventory(self.root)), 3)

    def test_protected_names_invalid_manifests_and_duplicates_rejected(self):
        retire.validate_manifest({"version": 1, "repository": retire.REPOSITORY, "branches": [
            {"branch": "test/verified", "expected_head": self.topic, "integrated_commit": self.main, "tree": self.tree}
        ]})
        for name in ("main", "engineering-platform/retained", "release/stable", "feat/../main", "feat//bad", "--force", "feat/trailing/"):
            bad = copy.deepcopy(self.manifest)
            bad["branches"][0]["branch"] = name
            with self.assertRaises(ValueError):
                retire.validate_manifest(bad)
        for change in ({"version": True}, {"repository": "other/repo"}, {"branches": []}, {"force": True}):
            bad = copy.deepcopy(self.manifest)
            bad.update(change)
            with self.assertRaises(ValueError):
                retire.validate_manifest(bad)
        bad = copy.deepcopy(self.manifest)
        bad["branches"].append(bad["branches"][0])
        with self.assertRaises(ValueError):
            retire.validate_manifest(bad)
        with self.assertRaises(ValueError):
            retire.pairs([("a", 1), ("a", 2)])

    def test_cli_cannot_apply_outside_protected_main_workflow(self):
        manifest = self.root / "manifest.json"
        manifest.write_text(json.dumps(self.manifest))
        result = subprocess.run(["python3", "-B", str(ROOT / "scripts/retire_integrated_branches.py"), "--manifest", str(manifest), "--expected-main", self.main, "--apply"], cwd=self.root, capture_output=True, text=True, env={"PATH": "/usr/local/bin:/usr/bin:/bin"})
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(json.loads(result.stdout)["status"], "REJECTED")
        self.assertEqual(len(retire.inventory(self.root)), 3)

    def test_checked_in_manifest_and_workflow_contract(self):
        value = json.loads((ROOT / ".github/retired-branches.json").read_text())
        retire.validate_manifest(value)
        workflow = (ROOT / ".github/workflows/retire-integrated-branches.yml").read_text()
        for fragment in ("workflow_run:", "head_repository.id == 1383377268", "workflow_run.event == 'push'", "workflow_run.conclusion == 'success'", "ref: ${{ github.event.workflow_run.head_sha }}", "cancel-in-progress: false", "--apply", "branch-retirement.json"):
            self.assertIn(fragment, workflow)
        self.assertNotIn("pull_request_target", workflow)


if __name__ == "__main__":
    unittest.main()
