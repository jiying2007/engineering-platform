"""No database or source mutation: validate permanent lifecycle test selection."""
import json
from pathlib import Path
import re
import subprocess
import sys
import tempfile
import unittest

import verify_service_lifecycle_log as gate

ROOT = Path(__file__).resolve().parents[1]
LOGS = ("service-control-tests.jsonl", "service-publisher-tests.jsonl", "service-helper-tests.jsonl", "service-worker-tests.jsonl", "service-runtime-tests.jsonl")


def complete(package, names):
    def event(action, name=None):
        value = {"Action": action, "Package": gate.PREFIX + package}
        if name is not None:
            value["Test"] = name
        return value
    result = [event("start")]
    for _ in range(3):
        for name in sorted(names):
            result.extend((event("run", name), event("pass", name)))
    return result + [event("pass")]


class ServiceLifecycleGateTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.paths = [Path(self.tmp.name) / name for name in LOGS]
        self.events = [complete(pkg, names) for pkg, names in gate.GROUPS]
        self.write()

    def write(self):
        for path, events in zip(self.paths, self.events):
            path.write_text("".join(json.dumps(e) + "\n" for e in events))

    def test_all_five_exact_streams_required(self):
        gate.verify_logs(self.paths)
        for paths in ([], self.paths[:2], self.paths[:3], self.paths[:4], self.paths + self.paths[:1], self.paths[::-1]):
            with self.assertRaises(ValueError):
                gate.verify_logs(paths)
        for i in range(len(gate.GROUPS)):
            original = self.events[i]
            for replacement in ([], original[1:], original[:-1],
                                [e for e in original if e["Action"] == "pass"],
                                original[:-1] + original[1:3] + original[-1:]):
                self.events[i] = replacement
                self.write()
                with self.assertRaises(ValueError):
                    gate.verify_logs(self.paths)
            self.events[i] = original
        self.write()

    def test_database_skip_cannot_hide_under_package_pass(self):
        name = "TestControlLifecycleServiceStopCancelsActualPostgresWait"
        self.events[0] = [dict(e, Action="skip") if e.get("Test") == name and e["Action"] == "pass" else e
                          for e in self.events[0]]
        self.write()
        with self.assertRaises(ValueError):
            gate.verify_logs(self.paths)

    def test_missing_database_test_and_nested_failure_are_rejected(self):
        name = "TestControlLifecycleServiceStopCancelsActualPostgresWait"
        self.events[0] = [e for e in self.events[0] if e.get("Test") != name]
        self.write()
        with self.assertRaises(ValueError):
            gate.verify_logs(self.paths)
        self.events[0] = complete(*gate.GROUPS[0])
        self.events[1].insert(-1, {"Action": "fail", "Package": gate.PREFIX + gate.GROUPS[1][0],
                                   "Test": "TestPublisherLifecycleRequestStopAndOriginalSettlement/nested"})
        self.write()
        with self.assertRaises(ValueError):
            gate.verify_logs(self.paths)

    def test_worker_unknown_post_and_semantic_failures_cannot_be_skipped(self):
        for index, name in ((2, "TestOnlyStopErrorPreservesUnwrappedSemanticFailure"),
                            (3, "TestWorkerLoopPreservesAuthorityAmbiguityAfterCancel"),
                            (3, "TestWorkerCLIStoppedAfterUnobservedClaimRequiresReadback"),
                            (4, "TestScopeStartFailurePreservesUnknownProcess"),
                            (4, "TestProbeDirectoryCleanupKeepsUnconfirmedProcessInputs")):
            original = self.events[index]
            for replacement in (
                [e for e in original if e.get("Test") != name],
                [dict(e, Action="skip") if e.get("Test") == name and e["Action"] == "pass" else e for e in original],
            ):
                self.events[index] = replacement
                self.write()
                with self.assertRaises(ValueError):
                    gate.verify_logs(self.paths)
            self.events[index] = original
        self.write()

    def test_exact_required_tests_exist_and_are_selected_in_canonical_ci(self):
        workflow = (ROOT / ".github/workflows/ci.yml").read_text()
        self.assertIn('test -n "$POSTGRES_TEST_URL"', workflow)
        self.assertIn("python3 -B scripts/test_verify_service_lifecycle_log.py", workflow)
        self.assertIn("python3 -B scripts/verify_service_lifecycle_log.py", workflow)
        self.assertEqual([len(names) for _, names in gate.GROUPS], [9, 4, 2, 5, 2])
        for (pkg, names), log in zip(gate.GROUPS, LOGS):
            existing = {name for p in (ROOT / pkg).glob("*_test.go")
                        for name in re.findall(r"^func (Test\w+)\(", p.read_text(), re.M)}
            self.assertTrue(names <= existing, sorted(names - existing))
            commands = [line for line in workflow.splitlines() if "go test -json" in line and log in line]
            self.assertEqual(len(commands), 1)
            self.assertIn("-race -count=3 -timeout 2m ./" + pkg, commands[0])
            selected = re.search(r"-run '([^']+)'", commands[0])
            if selected:
                self.assertFalse([name for name in names if not re.search(selected.group(1), name)])
            else:
                self.assertEqual(pkg, "internal/servicelifecycle")
            self.assertIn('"$RUNNER_TEMP/' + log + '"', workflow)

    def test_cli_rejects_incomplete_stream_and_missing_argument(self):
        command = [sys.executable, "-B", str(Path(gate.__file__).resolve())]
        for valid in (True, False):
            if not valid:
                self.events[2] = self.events[2][:-1]
                self.write()
            result = subprocess.run(command + list(map(str, self.paths)), capture_output=True, timeout=5)
            self.assertEqual(result.returncode == 0, valid, result.stderr)
        result = subprocess.run(command + list(map(str, self.paths[:2])), capture_output=True, timeout=5)
        self.assertNotEqual(result.returncode, 0)


if __name__ == "__main__":
    unittest.main()
