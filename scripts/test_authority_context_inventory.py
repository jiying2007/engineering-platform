#!/usr/bin/env python3
"""The scanner is only a candidate extractor; it must never imply closure."""
import tempfile
import unittest
from pathlib import Path
from authority_context_inventory import inventory

class ContextInventoryTests(unittest.TestCase):
    def test_candidates_exclude_test_files_comments_and_symlinks(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / "cmd").mkdir()
            p = root / "cmd/a.go"
            p.write_text("package main\n// context.Background()\nvar c = context.WithoutCancel(parent)\nvar v = s.GetWork(id)\n")
            (root / "cmd/a_test.go").write_text("context.Background()")
            (root / "cmd/link.go").symlink_to(p)
            result = inventory(root)
            self.assertEqual([f["name"] for f in result["context_or_call_candidates"]], ["WithoutCancel", "GetWork"])
            self.assertEqual(result["status"], "STATIC_CANDIDATES_NOT_CALL_GRAPH_PROOF")
            self.assertFalse(result["issue_229_closed"])
            self.assertEqual(result, inventory(root))
            before = result["source_files_digest"]
            p.write_text(p.read_text() + "// changed source\n")
            self.assertNotEqual(before, inventory(root)["source_files_digest"])
    def test_oversized_source_rejected(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / "internal").mkdir()
            (root / "internal/a.go").write_bytes(b"x" * ((2 << 20) + 1))
            with self.assertRaises(ValueError): inventory(root)

if __name__ == "__main__": unittest.main()
