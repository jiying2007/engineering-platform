import tempfile
import unittest
from pathlib import Path
from authority_context_inventory import inventory
from verify_context_inventory import CATEGORIES, verify

class ContextInventoryBindingTests(unittest.TestCase):
    def test_exact_current_source_and_reject_drift(self):
        with tempfile.TemporaryDirectory() as tmp:
            root=Path(tmp);(root/"cmd").mkdir()
            src=root/"cmd/main.go";src.write_text("package main\nvar c = context.Background()\n")
            digest=inventory(root)["source_files_digest"]
            doc=root/"inventory.md"
            good=f"`{digest}`\n"+"".join(f"| {c} | {int(c=='Explicitly timed command root')} |\n" for c in sorted(CATEGORIES))+"| `cmd/main.go:2` | `Background` | Explicitly timed command root |\n"
            doc.write_text(good)
            result=verify(root,doc)
            self.assertEqual(result["direct_sites"],1)
            self.assertFalse(result["interprocedural_proof"])
            self.assertFalse(result["issue_229_closed"])
            for bad in (good.replace(digest,"sha256:"+"0"*64),good.replace(":2`",":3`"),good.replace("| `cmd/main.go:2`","| `cmd/other.go:2`"),good+"| `cmd/main.go:2` | `Background` | Explicitly timed command root |\n",good.replace("| Explicitly timed command root | 1 |","| Explicitly timed command root | 2 |"),good.replace("` | Explicitly timed command root |","` | Assumed safe |")):
                doc.write_text(bad)
                with self.assertRaises(ValueError):verify(root,doc)
            doc.write_text(good)
            src.write_text(src.read_text()+"// changed source\n")
            with self.assertRaises(ValueError):verify(root,doc)
if __name__=="__main__":unittest.main()
