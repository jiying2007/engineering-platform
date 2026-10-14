"""Do not accept skipped SQL tests or a selector which misses the budget tests."""
import json
from pathlib import Path
import re
import subprocess
import sys
import tempfile
import unittest

import verify_authority_budget_log as gate

ROOT = Path(__file__).resolve().parents[1]


def events():
    values = [{"Action": "start", "Package": gate.PACKAGE}]
    for name in sorted(gate.REQUIRED):
        values += [{"Action": action, "Package": gate.PACKAGE, "Test": name}
                   for action in ("run", "pass")]
    values += [{"Action": "pass", "Package": gate.PACKAGE}]
    return values


class AuthorityBudgetGateTests(unittest.TestCase):
    def test_exact_source_and_ci_selection(self):
        names = set()
        for name in ("authority_context_test.go", "authority_budget_integration_test.go"):
            source = (ROOT / "internal/store/postgres" / name).read_text()
            names.update(re.findall(r"^func (TestPostgresAuthorityBudget\w+)\(", source, re.M))
        self.assertEqual(names, gate.REQUIRED)
        workflow = (ROOT / ".github/workflows/ci.yml").read_text()
        step = workflow.split("      - name: PostgreSQL no-deadline authority budget regression\n", 1)[1].split("      - name:", 1)[0]
        self.assertIn('test -n "$POSTGRES_TEST_URL"', step)
        self.assertIn("go test -json -race -count=1 -timeout 2m ./internal/store/postgres -run '^TestPostgresAuthorityBudget'", step)
        self.assertIn("python3 -B scripts/verify_authority_budget_log.py", step)
        self.assertTrue(all(re.search(r"^TestPostgresAuthorityBudget", n) for n in names))

    def test_complete_and_missing_skipped_or_unfinished_db(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "events.jsonl"
            def write(values):
                path.write_text("\n".join(json.dumps(v) for v in values) + "\n")
            good = events()
            write(good)
            gate.main([str(path)])
            required_db = "TestPostgresAuthorityBudgetNoDeadlinePoolWait"
            missing = [v for v in good if v.get("Test") != required_db]
            skipped = [dict(v, Action="skip") if v.get("Test") == required_db and v["Action"] == "pass" else v for v in good]
            for bad in (missing, skipped, good[:-1], [v for v in good if v["Action"] == "pass"]):
                write(bad)
                with self.assertRaises(ValueError):
                    gate.main([str(path)])

    def test_cli_and_invalid_arity(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "events.jsonl"
            path.write_text("\n".join(json.dumps(v) for v in events()) + "\n")
            cli = [sys.executable, "-B", str(Path(gate.__file__).resolve())]
            for args, accepted in (([str(path)], True), ([], False), ([str(path), "extra"], False)):
                result = subprocess.run(cli + args, capture_output=True, timeout=3)
                self.assertEqual(result.returncode == 0, accepted, result.stderr)


if __name__ == "__main__":
    unittest.main()
