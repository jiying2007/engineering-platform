#!/usr/bin/env python3
"""AST-backed origin inventory stays read-only and does not infer closure."""
import json
import os
import subprocess
import tempfile
import unittest
from pathlib import Path
from authority_context_inventory import inventory, _read_source

class ContextInventoryTests(unittest.TestCase):
    def test_candidates_exclude_test_files_comments_and_symlinks(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / "cmd").mkdir()
            p = root / "cmd/a.go"
            p.write_text('package main\nimport "context"\n// context.Background()\nvar c = context.WithoutCancel(parent)\nvar v = s.GetWork(id)\n')
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

    def test_alias_and_function_value_origins_are_visible(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / "internal").mkdir()
            (root / "internal/a.go").write_text('package p\nimport c "context"\nvar makeRoot=c.Background\nfunc f(){_=c.\nTODO()}\n')
            result = inventory(root)
            self.assertEqual([(f["line"],f["name"],f["reference_form"]) for f in result["context_or_call_candidates"]],
                             [(3,"Background","function_value"),(4,"TODO","direct_call")])

    def test_oversized_and_invalid_source_rejected(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / "internal").mkdir()
            p = root / "internal/a.go"
            for raw in [b"x" * ((2 << 20) + 1), b"package p;func {", b'package p;import . "context";var x=Background()']:
                p.write_bytes(raw)
                with self.assertRaises(ValueError):
                    inventory(root)

    def test_bounded_source_descriptor_rejects_alias_and_fifo(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            source = root / "real.go"
            source.write_text("package p\n")
            alias = root / "alias.go"
            alias.symlink_to(source)
            with self.assertRaises(ValueError):
                _read_source(alias)
            if hasattr(os, "mkfifo"):
                special = root / "special.go"
                os.mkfifo(special)
                # A regression to blocking open must terminate as a test failure.
                import sys
                code = "from pathlib import Path; from authority_context_inventory import _read_source; _read_source(Path(__import__('sys').argv[1]))"
                proc = subprocess.run([sys.executable,"-B","-c",code,str(special)],cwd=Path(__file__).resolve().parent,
                                      capture_output=True,timeout=3)
                self.assertNotEqual(proc.returncode,0)
                self.assertIn(b"bounded regular Go source required",proc.stderr)

    def test_native_frontend_scope_and_position_regressions(self):
        folder = Path(__file__).resolve().parent
        env = dict(os.environ,GO111MODULE="off",GOWORK="off",GOTOOLCHAIN="local",GOFLAGS="")
        proc = subprocess.run(["go","test","-json",str(folder/"context_inventory_ast.go"),str(folder/"context_inventory_ast_test.go")],
                              capture_output=True,env=env,timeout=120)
        self.assertEqual(proc.returncode,0,proc.stderr.decode()+proc.stdout.decode())
        events = [json.loads(line) for line in proc.stdout.splitlines()]
        self.assertFalse(any(e["Action"] in ("fail","skip") for e in events))
        self.assertEqual(sum(e["Action"]=="pass" and e.get("Test","").startswith("TestAST") and "/" not in e["Test"] for e in events),3)

if __name__ == "__main__":
    unittest.main()
