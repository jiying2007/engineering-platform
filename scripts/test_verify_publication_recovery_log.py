"""Exercise all existing gate modes with complete Go test event lifecycles."""
import copy
import json
import os
import re
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

import verify_publication_recovery_log as gate

MODES = (
    (gate.REQUIRED, gate.PACKAGE, 3, None),
    (gate.RESTORE_REQUIRED, gate.PACKAGE, 1, "--restore"),
    (gate.PUBLISHER_REQUIRED, gate.PUBLISHER_PACKAGE, 3, "--publisher-kernel"),
    (gate.NO_INVOCATION_REQUIRED, gate.PACKAGE, 3, "--no-invocation-db"),
    (gate.NO_INVOCATION_PUBLISHER_REQUIRED, gate.PUBLISHER_PACKAGE, 3, "--no-invocation-publisher"),
    (gate.PROOF_REQUIRED, gate.PUBLISHER_PACKAGE, 3, "--publisher-proof"),
    (gate.CONTROL_REQUIRED, gate.CONTROL_PACKAGE, 20, "--control-response"),
    (gate.CAPACITY_REQUIRED, gate.PACKAGE, 3, "--capacity-db"),
)


def event(action, name=None, package=gate.PACKAGE, **extra):
    result = {"Action": action, "Package": package, **extra}
    if name is not None:
        result["Test"] = name
    return result


def complete(required=gate.REQUIRED, package=gate.PACKAGE, repeats=3):
    events = [event("start", package=package)]
    for _ in range(repeats):
        for name in sorted(required):
            events.extend((event("run", name, package), event("pass", name, package)))
    events.append(event("pass", package=package))
    return events


class GateTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.path = Path(self.tmp.name) / "events.jsonl"

    def write(self, events):
        self.path.write_text("\n".join(json.dumps(e) for e in events) + "\n", encoding="utf-8")

    def reject(self, events, **kwargs):
        self.write(events)
        with self.assertRaises(ValueError):
            gate.verify(self.path, **kwargs)

    def test_canonical_publisher_selector_includes_every_required_test(self):
        workflow = (Path(__file__).resolve().parents[1] / ".github/workflows/ci.yml").read_text()
        commands = [line for line in workflow.splitlines()
                    if "go test -json" in line and "publisher-kernel-tests.jsonl" in line]
        self.assertEqual(len(commands), 1, "one canonical Publisher event stream required")
        self.assertIn("-count=3", commands[0], "keep original repeated gate")
        match = re.search(r"-run '([^']+)'", commands[0])
        self.assertIsNotNone(match, "explicit canonical test selector required")
        missing = sorted(name for name in gate.PUBLISHER_REQUIRED
                         if re.search(match.group(1), name) is None)
        self.assertEqual(missing, [], "required tests must actually be selected by CI")

    def test_control_gate_runs_the_full_existing_package(self):
        root = Path(__file__).resolve().parents[1]
        workflow = (root / ".github/workflows/ci.yml").read_text()
        commands = [line for line in workflow.splitlines()
                    if "go test -json" in line and "control-response-tests.jsonl" in line]
        self.assertEqual(len(commands), 1)
        self.assertIn("-race -count=20 -timeout 2m ./internal/controlclient", commands[0])
        self.assertNotIn("-run", commands[0], "do not silently reduce the previous full-package gate")
        self.assertIn('"$RUNNER_TEMP/control-response-tests.jsonl" --control-response', workflow)
        existing = {name for path in (root / "internal/controlclient").glob("*_test.go")
                    for name in re.findall(r"^func (Test\w+)\(", path.read_text(), re.M)}
        self.assertTrue(gate.CONTROL_REQUIRED <= existing)
        lifecycle = {name for name in existing if name.startswith("TestControlResponseLifecycle")}
        self.assertEqual(len(lifecycle), 7)
        self.assertTrue(lifecycle <= gate.CONTROL_REQUIRED)

    def test_restore_selector_includes_every_required_test(self):
        root = Path(__file__).resolve().parents[1]
        workflow = (root / ".github/workflows/ci.yml").read_text()
        commands = [line for line in workflow.splitlines()
                    if "go test -json" in line and "publication-restore-tests.jsonl" in line]
        self.assertEqual(len(commands), 1)
        self.assertIn("-race -count=1", commands[0])
        match = re.search(r"-run '([^']+)'", commands[0])
        self.assertIsNotNone(match)
        self.assertEqual(sorted(name for name in gate.RESTORE_REQUIRED
                                if re.search(match.group(1), name) is None), [])
        existing = {name for path in (root / "internal/store/postgres").glob("*_test.go")
                    for name in re.findall(r"^func (Test\w+)\(", path.read_text(), re.M)}
        self.assertTrue(gate.RESTORE_REQUIRED <= existing)

    def test_capacity_database_cannot_be_missing_or_skipped(self):
        root = Path(__file__).resolve().parents[1]
        workflow = (root / ".github/workflows/ci.yml").read_text()
        commands = [line for line in workflow.splitlines()
                    if "go test -json" in line and "capacity-database-tests.jsonl" in line]
        self.assertEqual(len(commands), 1)
        self.assertIn("-race -count=3 -timeout 2m ./internal/store/postgres", commands[0])
        match = re.search(r"-run '([^']+)'", commands[0])
        self.assertIsNotNone(match)
        self.assertFalse([name for name in gate.CAPACITY_REQUIRED if not re.search(match.group(1), name)])
        source = root / "internal/store/postgres/operational_capacity_test.go"
        existing = set(re.findall(r"^func (TestReadOperationalCapacity\w+)\(", source.read_text(), re.M))
        self.assertEqual(existing, gate.CAPACITY_REQUIRED)
        self.assertIn('"$RUNNER_TEMP/capacity-database-tests.jsonl" --capacity-db', workflow)
        good = complete(gate.CAPACITY_REQUIRED)
        for name in gate.CAPACITY_REQUIRED:
            self.reject([e for e in good if e.get("Test") != name], required=gate.CAPACITY_REQUIRED)
            self.reject([dict(e, Action="skip") if e.get("Test") == name and e["Action"] == "pass" else e
                         for e in good], required=gate.CAPACITY_REQUIRED)

    def test_confirmation_boundary_is_permanently_selected(self):
        import re
        root = Path(__file__).resolve().parents[1]
        source = (root / "internal/store/postgres/confirmation_boundary_test.go").read_text()
        names = set(re.findall(r"^func (TestPublicationRecoveryDatabase\w+)\(", source, re.M))
        self.assertEqual(len(names), 6)
        self.assertTrue(names <= gate.REQUIRED)
        workflow = (root / ".github/workflows/ci.yml").read_text()
        self.assertIn("-run '^TestPublicationRecoveryDatabase'", workflow)
        self.assertIn("scripts/verify_publication_recovery_log.py", workflow)

    def test_all_modes_require_complete_exact_repetitions(self):
        for required, package, repeats, option in MODES:
            with self.subTest(mode=option):
                good = complete(required, package, repeats)
                kwargs = dict(required=required, package=package, repeats=repeats)
                self.write(good)
                gate.verify(self.path, **kwargs)
                for bad in ([], good[1:], good[:-1], good[:1] + good[3:],
                            good + [good[-1]], good[:-1] + good[1:3] + good[-1:],
                            [dict(e, Package="wrong/package") for e in good],
                            [e for e in good if e["Action"] == "pass"]):
                    self.reject(bad, **kwargs)
                for action in ("skip", "fail", "build-fail"):
                    self.reject(good[:-1] + [event(action, "unknown/nested", package)] + good[-1:], **kwargs)

    def test_realistic_output_parallel_subtests_and_repeated_names(self):
        events = [event("start")]
        for _ in range(3):
            events += [event("run", "TestCase"), event("run", "TestCase/child"),
                       event("pause", "TestCase/child"),
                       event("run", "TestCase/Europe/Paris"), event("pass", "TestCase/Europe/Paris"),
                       event("run", "TestCase/child/slash-name"), event("pass", "TestCase/child/slash-name"),
                       event("run", "TestCase/sibling"),
                       event("pass", "TestCase/sibling"), event("cont", "TestCase/child"),
                       event("output", "TestCase/child", Output="=== CONT child\n"),
                       event("pass", "TestCase/child"), event("pass", "TestCase")]
        events += [event("output", Output="PASS\n"), event("pass")]
        self.write(events)
        gate.verify(self.path, required={"TestCase"})

    def test_invalid_event_order_and_unfinished_unknown_tests(self):
        start, run, passed = event("start"), event("run", "TestCase"), event("pass", "TestCase")
        cases = {
            "pass-only": [passed, event("pass")],
            "duplicate-start": [start, start, run, passed, event("pass")],
            "duplicate-run": [start, run, run, passed, event("pass")],
            "duplicate-pass": [start, run, passed, passed, event("pass")],
            "premature-package": [start, run, event("pass"), passed],
            "after-end": [start, run, passed, event("pass"), event("output", Output="late")],
            "orphan-child": [start, event("run", "TestCase/child"), event("pass")],
            "unfinished-child": [start, run, event("run", "TestCase/child"), passed, event("pass")],
            "unknown-unfinished": [start, run, passed, event("run", "TestExtra"), event("pass")],
            "cont-not-paused": [start, run, event("cont", "TestCase"), passed, event("pass")],
            "pause-twice": [start, run, event("pause", "TestCase"), event("pause", "TestCase")],
            "pass-paused": [start, run, event("pause", "TestCase"), passed, event("pass")],
            "unknown-action": [start, event("invented"), run, passed, event("pass")],
            "different-package-start": [event("start", package="other"), run, passed, event("pass")],
        }
        for name, events in cases.items():
            with self.subTest(case=name):
                self.reject(events, required={"TestCase"}, repeats=1)

    def test_invalid_fields(self):
        good = complete({"TestCase"}, repeats=1)
        for value in (None, "", False, [], {}, 3):
            bad = copy.deepcopy(good)
            bad[1]["Test"] = value
            with self.subTest(value=value):
                self.reject(bad, required={"TestCase"}, repeats=1)
        for bad_event in (event("output", Output=3), {"Action": []}, [], None):
            self.reject([good[0], bad_event] + good[1:], required={"TestCase"}, repeats=1)
        self.reject([good[0], event("run", "x" * (gate.MAX_NAME_BYTES + 1))] + good[1:], required={"TestCase"}, repeats=1)

    def test_duplicate_keys_nonfinite_invalid_utf8_and_depth(self):
        self.write(complete({"TestCase"}, repeats=1))
        good = self.path.read_bytes()
        cases = (b'{"Action":"fail","Action":"output","Package":"' + gate.PACKAGE.encode() + b'","Output":"hidden failure"}\n',
                 b'{"Action":"output","Package":"' + gate.PACKAGE.encode() + b'","Output":"x","Elapsed":NaN}\n',
                 b'{"Elapsed":1e9999}\n', b'\xff\n', b'\n', b'[]\n',
                 b'[' * 2000 + b'0' + b']' * 2000 + b'\n')
        first, rest = good.split(b'\n', 1)
        for raw in cases:
            with self.subTest(raw=raw[:50]):
                self.path.write_bytes(first + b'\n' + raw + rest)
                with self.assertRaises(ValueError):
                    gate.verify(self.path, required={"TestCase"}, repeats=1)

    def test_budget_limits(self):
        good = complete({"TestCase"}, repeats=1)
        for key, limit in (("MAX_LOG_BYTES", 8), ("MAX_LINE_BYTES", 8),
                           ("MAX_EVENTS", 2), ("MAX_TESTS", 0)):
            self.write(good)
            with self.subTest(budget=key), patch.object(gate, key, limit):
                with self.assertRaises(ValueError):
                    gate.verify(self.path, required={"TestCase"}, repeats=1)

    def test_invalid_verification_contract(self):
        self.write(complete())
        for repeats in (0, 21, True, 1.5, "3"):
            with self.assertRaises(ValueError):
                gate.verify(self.path, repeats=repeats)
        for required in (set(), [], {""}, {"TestCase/sub"}):
            with self.assertRaises(ValueError):
                gate.verify(self.path, required=required)

    @unittest.skipUnless(hasattr(os, "mkfifo"), "POSIX FIFO required")
    def test_fifo_is_rejected_without_waiting_for_writer(self):
        os.mkfifo(self.path)
        proc = subprocess.run([sys.executable, str(Path(gate.__file__).resolve()), str(self.path)],
                              capture_output=True, timeout=3)
        self.assertNotEqual(proc.returncode, 0)
        self.assertIn(b"bounded regular test log required", proc.stderr)

    def test_every_cli_mode_accepts_complete_and_rejects_summary(self):
        for required, package, repeats, option in MODES:
            args = [sys.executable, str(Path(gate.__file__).resolve()), str(self.path)]
            if option:
                args.append(option)
            events = complete(required, package, repeats)
            for good in (True, False):
                with self.subTest(mode=option, complete=good):
                    self.write(events if good else [e for e in events if e["Action"] == "pass"])
                    proc = subprocess.run(args, capture_output=True, timeout=3)
                    self.assertEqual(proc.returncode == 0, good, proc.stderr)


if __name__ == "__main__":
    unittest.main()
