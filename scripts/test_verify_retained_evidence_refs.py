#!/usr/bin/env python3
import copy
from pathlib import Path
import subprocess
import tempfile
import unittest

import verify_retained_evidence_refs as guard

class RetainedEvidenceRefsTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.work = Path(self.temp.name) / "work"
        self.remote = Path(self.temp.name) / "remote.git"
        subprocess.run(["git", "init", "--bare", str(self.remote)], check=True, capture_output=True)
        subprocess.run(["git", "init", "-b", "main", str(self.work)], check=True, capture_output=True)
        self.git("config", "user.name", "test-only")
        self.git("config", "user.email", "test@example.invalid")
        self.git("remote", "add", "origin", str(self.remote))
        (self.work / "data").write_text("base\n")
        self.git("add", "data")
        self.git("commit", "-m", "base")
        self.first = self.git("rev-parse", "HEAD")
        (self.work / "data").write_text("second\n")
        self.git("add", "data")
        self.git("commit", "-m", "second")
        self.second = self.git("rev-parse", "HEAD")
        self.refs = [
            {"branch": "engineering-platform/111111111111111111111111", "expected_head": self.first},
            {"branch": "engineering-platform/222222222222222222222222", "expected_head": self.second},
        ]
        self.git("push", "origin", self.first + ":refs/heads/" + self.refs[0]["branch"], self.second + ":refs/heads/" + self.refs[1]["branch"])

    def git(self, *args):
        result = subprocess.run(["git", "-C", str(self.work), *args], capture_output=True, text=True, check=True)
        return result.stdout.strip()

    def manifest(self):
        return {"version": 1, "repository": guard.REPOSITORY, "refs": copy.deepcopy(self.refs)}

    def test_exact_inventory_passes_without_mutation(self):
        report = guard.verify(self.work, self.manifest())
        self.assertEqual(report["status"], "RETAINED_EVIDENCE_REFS_EXACT")
        self.assertFalse(report["mutation_attempted"])

    def test_changed_missing_or_extra_ref_fails(self):
        newer = self.git("commit-tree", self.git("rev-parse", "HEAD^{tree}"), "-p", self.second, "-m", "drift")
        self.git("push", "--force", "origin", newer + ":refs/heads/" + self.refs[0]["branch"])
        with self.assertRaises(ValueError):
            guard.verify(self.work, self.manifest())
        self.git("push", "--force", "origin", self.first + ":refs/heads/" + self.refs[0]["branch"])
        self.git("push", "origin", ":refs/heads/" + self.refs[1]["branch"])
        with self.assertRaises(ValueError):
            guard.verify(self.work, self.manifest())
        self.git("push", "origin", self.second + ":refs/heads/engineering-platform/333333333333333333333333")
        with self.assertRaises(ValueError):
            guard.verify(self.work, self.manifest())

    def test_manifest_is_strict_and_ordered(self):
        guard.validate(self.manifest())
        unordered = self.manifest()
        unordered["refs"].reverse()
        with self.assertRaises(ValueError):
            guard.validate(unordered)
        wrong = self.manifest()
        wrong["refs"][0]["branch"] = "feat/not-retained"
        with self.assertRaises(ValueError):
            guard.validate(wrong)
        with self.assertRaises(ValueError):
            guard.pairs([("a", 1), ("a", 2)])

if __name__ == "__main__":
    unittest.main()
