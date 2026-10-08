#!/usr/bin/env python3
"""Self-contained negative tests for bounded CI admission characterization."""
import json
import unittest

from ci_admission_characterization import (
    MARKER, TEST, build_report, duration_ns, parse_events,
)

SHA = "a" * 40
METRIC = (
    "runs=96 worker_identities=8 security_rounds=3 denied=48 "
    "intake_wall=428.046606ms worker_wall=940.175342ms "
    "process_p50=60.520729ms process_p95=195.287577ms "
    "process_max=220.284471ms"
)


def event_stream(metric=METRIC, passes=5, reports=5):
    events = []
    for idx in range(max(passes, reports)):
        if idx < reports:
            events.append({"Action": "output", "Test": TEST,
                           "Output": "load_security_integration_test.go:357: "
                                     + MARKER + " " + metric + "\n"})
        if idx < passes:
            events.append({"Action": "pass", "Test": TEST, "Elapsed": 8.0})
    return ("\n".join(json.dumps(x) for x in events) + "\n").encode()


class CharacterizationTests(unittest.TestCase):
    def test_exact_five_rounds_and_source(self):
        raw = event_stream()
        report = build_report(raw, SHA, 42, 1)
        self.assertEqual(report["aggregate"], {
            "runs": 480, "denied_probes": 240,
            "test_rounds": 5, "worker_identities_per_round": 8,
        })
        self.assertEqual(report["samples"][0]["process_p95_ns"], 195287577)
        self.assertEqual(report["samples"][4]["round"], 5)
        self.assertFalse(report["qualification_granted"])
        self.assertFalse(report["production_qualified"])
        self.assertFalse(report["slo_qualified"])
        self.assertEqual(len(report["input_sha256"]), 71)

    def test_reject_missing_or_extra_measurement(self):
        for raw in (event_stream(passes=4), event_stream(reports=4),
                    event_stream(passes=6, reports=6)):
            with self.assertRaises(ValueError):
                parse_events(raw)

    def test_reject_changed_workload_or_forged_performance(self):
        for changed in (METRIC.replace("denied=48", "denied=47"),
                        METRIC.replace("process_p95=195.287577ms", "process_p95=2ms"),
                        METRIC + " allowed=999",
                        METRIC.replace("runs=96", "runs=96 runs=96"),
                        METRIC.replace("process_max=220.284471ms", "process_max=nan"),
                        METRIC.replace("worker_wall=940.175342ms", "worker_wall=-2ms")):
            with self.assertRaises(ValueError):
                parse_events(event_stream(metric=changed))

    def test_reject_malformed_failing_and_oversize_events(self):
        samples = [b"", b"not-json\n", event_stream() + b'{"garbled":',
                   b"x" * (20 * 1024 * 1024 + 1)]
        for raw in samples:
            with self.assertRaises(ValueError):
                parse_events(raw)
        failed = json.dumps({"Action": "fail", "Test": TEST}).encode()
        with self.assertRaises(ValueError):
            parse_events(event_stream() + failed + b"\n")

    def test_reject_missing_run_identity(self):
        raw = event_stream()
        for sha, run, attempt in (("z" * 40, 42, 1), (SHA, 0, 1), (SHA, 42, 0)):
            with self.assertRaises(ValueError):
                build_report(raw, sha, run, attempt)

    def test_duration_precision_and_bounds(self):
        self.assertEqual(duration_ns("0.000001ms"), 1)
        self.assertEqual(duration_ns("0.123456ms"), 123456)
        for invalid in ("-1ms", "nanms", "1.5ns", "1h", "601s", "1ms2us"):
            with self.assertRaises(ValueError):
                duration_ns(invalid)


if __name__ == "__main__":
    unittest.main()
