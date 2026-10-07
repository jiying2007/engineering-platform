#!/usr/bin/env python3
import json
from pathlib import Path
import unittest
from unittest.mock import patch
import prelive_provenance as p


class ProvenanceTests(unittest.TestCase):
    def events(self):
        events = []
        for package, tests in sorted(p.EXPECTED.items()):
            events.append({"Package": package, "Action": "start"})
            for name in sorted(tests):
                events += [{"Package": package, "Test": name, "Action": a} for a in ("run", "pass")]
            events.append({"Package": package, "Action": "pass"})
        return events

    def encode(self, events):
        return b"\n".join(json.dumps(e).encode() for e in events)

    def test_full_inventory_required(self):
        events = self.events()
        self.assertEqual(len(p.validate_events(self.encode(events))), 5)
        for bad in ([], [e for e in events if not e.get("Test")], events[:-1], events[1:], events + events):
            with self.assertRaises(ValueError):
                p.validate_events(self.encode(bad))

    def test_skip_fail_missing_duplicate_rejected(self):
        for action in ("skip", "fail", "build-fail", "unknown"):
            events = self.events()
            events[2]["Action"] = action
            with self.assertRaises(ValueError):
                p.validate_events(self.encode(events))
        events = self.events()
        events[2]["Test"] = "UnexpectedTest"
        with self.assertRaises(ValueError):
            p.validate_events(self.encode(events))
        with self.assertRaises(ValueError):
            p.validate_events(b'{"Package":"x","Package":"y","Action":"pass"}')

    def test_real_workflow_is_exact_and_keeps_events(self):
        root = Path(__file__).resolve().parents[1]
        workflow = (root / ".github/workflows/production-terminal-pre-live.yml").read_text()
        self.assertNotIn("ref: main\n", workflow)
        for required in ("ref: ${{ github.sha }}", "test-events.json", "provenance.json", "prelive_provenance.py", "--expected-sha"):
            self.assertIn(required, workflow)
        self.assertNotIn("continue-on-error", workflow)

    def test_v3_inventory_keeps_unknown_tests_fail_closed(self):
        self.assertIn("TestTerminalPlanRejectsWeakenedRolloutAndEmergencyContract",
                      p.EXPECTED[p.PREFIX + "internal/production"])
        events = self.events()
        forged = dict(events[2])
        forged["Test"] = "TestTerminalPlanFutureUnfrozenCase"
        events.insert(2, forged)
        with self.assertRaises(ValueError):
            p.validate_events(self.encode(events))

    def test_source_mismatch_and_dirty_tree_rejected_before_report(self):
        for answers in (("a"*40, "b"*40), ("c"*40, "b"*40, " M tracked-file")):
            with patch.object(p, "git", side_effect=answers):
                with self.assertRaises(ValueError):
                    p.build_report("c"*40, self.encode(self.events()), b"{}")


if __name__ == "__main__":
    unittest.main()
