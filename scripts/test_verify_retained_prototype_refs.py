#!/usr/bin/env python3
import copy
from pathlib import Path
import subprocess
import tempfile
import unittest

import verify_retained_prototype_refs as guard

class RetainedPrototypeRefsTests(unittest.TestCase):
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
        self.base = self.git("rev-parse", "HEAD")
        self.refs = []
        for index, name in enumerate(guard.NAMES):
            (self.work / "data").write_text(f"prototype-{index}\n")
            self.git("add", "data")
            self.git("commit", "-m", f"prototype-{index}")
            sha = self.git("rev-parse", "HEAD")
            self.refs.append({"branch": name, "expected_head": sha, "disposition": guard.DISPOSITION})
            self.git("push", "origin", sha + ":refs/heads/" + name)
        self.git("reset", "--hard", self.base)

    def git(self, *args):
        result = subprocess.run(["git", "-C", str(self.work), *args], capture_output=True, text=True, check=True)
        return result.stdout.strip()

    def manifest(self):
        return {"version": 1, "repository": guard.REPOSITORY, "refs": copy.deepcopy(self.refs)}

    def test_exact_refs_pass_without_mutation_and_unrelated_refs_are_ignored(self):
        self.git("push", "origin", self.base + ":refs/heads/feat/unrelated-active")
        report = guard.verify(self.work, self.manifest())
        self.assertEqual(report["status"], "RETAINED_PROTOTYPE_REFS_EXACT")
        self.assertFalse(report["mutation_attempted"])
        self.assertEqual(len(report["refs"]), 4)

    def test_changed_or_missing_retained_ref_fails(self):
        newer = self.git("commit-tree", self.git("rev-parse", self.base + "^{tree}"), "-p", self.refs[0]["expected_head"], "-m", "drift")
        self.git("push", "--force", "origin", newer + ":refs/heads/" + self.refs[0]["branch"])
        with self.assertRaises(ValueError):
            guard.verify(self.work, self.manifest())
        self.git("push", "--force", "origin", self.refs[0]["expected_head"] + ":refs/heads/" + self.refs[0]["branch"])
        self.git("push", "origin", ":refs/heads/" + self.refs[1]["branch"])
        with self.assertRaises(ValueError):
            guard.verify(self.work, self.manifest())

    def test_manifest_requires_exact_names_order_and_disposition(self):
        guard.validate(self.manifest())
        wrong = self.manifest()
        wrong["refs"].reverse()
        with self.assertRaises(ValueError):
            guard.validate(wrong)
        wrong = self.manifest()
        wrong["refs"][0]["branch"] = "feat/other"
        with self.assertRaises(ValueError):
            guard.validate(wrong)
        wrong = self.manifest()
        wrong["refs"][0]["disposition"] = "DELETE"
        with self.assertRaises(ValueError):
            guard.validate(wrong)
        wrong = self.manifest()
        wrong["refs"][0]["expected_head"] = "bad"
        with self.assertRaises(ValueError):
            guard.validate(wrong)
        with self.assertRaises(ValueError):
            guard.pairs([("a", 1), ("a", 2)])

if __name__ == "__main__":
    unittest.main()
