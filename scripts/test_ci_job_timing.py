#!/usr/bin/env python3
"""Negative tests for provenance-bound CI job timing (stdlib only)."""
import copy
import json
import unittest

from ci_job_timing import REQUIRED, build_report, elapsed_ms, utc_time

SHA = "a" * 40
START = "2026-10-08T00:00:00Z"
STEP_END = "2026-10-08T00:00:02Z"
END = "2026-10-08T00:00:05Z"


def source():
    jobs = []
    for i, name in enumerate(REQUIRED):
        jobs.append({
            "id": i + 123, "run_id": 456, "run_attempt": 1,
            "name": name, "head_sha": SHA, "status": "completed",
            "conclusion": "success", "started_at": START, "completed_at": END,
            "steps": [
                {"name": "Set up job", "number": 1, "status": "completed",
                 "conclusion": "success", "started_at": START, "completed_at": STEP_END},
                {"name": "Verify", "number": 2, "status": "completed",
                 "conclusion": "success", "started_at": STEP_END, "completed_at": END},
            ],
        })
    jobs.append({"name": "trusted-ci-artifact-evidence", "status": "in_progress"})
    return {"total_count": len(jobs), "jobs": jobs}


def encode(data):
    return json.dumps(data, separators=(",", ":")).encode()


class CIJobTimingTests(unittest.TestCase):
    def test_successful_four_job_bound_report(self):
        report = build_report(encode(source()), SHA, 456, 1)
        self.assertEqual(report["aggregate"], {
            "job_count": 4, "total_runner_wall_ms": 20_000,
            "max_job_wall_ms": 5_000,
        })
        self.assertEqual([x["name"] for x in report["jobs"]], list(REQUIRED))
        self.assertEqual(len(report["raw_jobs_sha256"]), 71)
        self.assertEqual(report["jobs"][0]["steps"][0]["elapsed_ms"], 2_000)
        for key in ("billing_qualified", "slo_qualified",
                    "provider_live_qualified", "production_qualified"):
            self.assertIs(report[key], False)

    def test_missing_duplicate_or_failed_required_job_rejected(self):
        for mutate in (
            lambda d: d["jobs"].pop(0),
            lambda d: d["jobs"].append(copy.deepcopy(d["jobs"][0])),
            lambda d: d["jobs"][0].update(status="in_progress"),
            lambda d: d["jobs"][0].update(conclusion="failure"),
            lambda d: d["jobs"][0].update(head_sha="b" * 40),
            lambda d: d["jobs"][0].update(run_attempt=2),
            lambda d: d["jobs"][0].update(run_id=457),
            lambda d: d.update(total_count=101),
        ):
            case = source()
            mutate(case)
            case["total_count"] = len(case["jobs"]) if case["total_count"] != 101 else 101
            with self.assertRaises(ValueError):
                build_report(encode(case), SHA, 456, 1)

    def test_invalid_steps_and_times_rejected(self):
        for mutate in (
            lambda d: d["jobs"][0]["steps"][0].update(conclusion="failure"),
            lambda d: d["jobs"][0]["steps"][0].update(status="in_progress"),
            lambda d: d["jobs"][0]["steps"][0].update(number=2),
            lambda d: d["jobs"][0]["steps"][0].update(name="Verify"),
            lambda d: d["jobs"][0]["steps"][0].update(started_at="2026-10-07T23:59:59Z"),
            lambda d: d["jobs"][0]["steps"][1].update(completed_at="2026-10-08T00:00:06Z"),
            lambda d: d["jobs"][0].update(completed_at="2026-10-07T23:59:59Z"),
            lambda d: d["jobs"][0]["steps"][0].update(started_at="2026-10-08T00:00:00+00:00"),
            lambda d: d["jobs"][0]["steps"][0].update(started_at=None),
        ):
            case = source()
            mutate(case)
            with self.assertRaises(ValueError):
                build_report(encode(case), SHA, 456, 1)

    def test_invalid_payload_and_identity_rejected(self):
        for data in (b"", b"not-json", b"{}", b'{"jobs":null}',
                     b"x" * (4 * 1024 * 1024 + 1)):
            with self.assertRaises(ValueError):
                build_report(data, SHA, 456, 1)
        for sha, run, attempt in (("X" * 40, 456, 1), (SHA, 0, 1), (SHA, 456, 0)):
            with self.assertRaises(ValueError):
                build_report(encode(source()), sha, run, attempt)

    def test_millisecond_accounting(self):
        self.assertEqual(elapsed_ms(utc_time("2026-10-08T00:00:00.123Z"),
                                    utc_time("2026-10-08T00:00:01.235Z")), 1112)
        with self.assertRaises(ValueError):
            elapsed_ms(utc_time(END), utc_time(START))


if __name__ == "__main__":
    unittest.main()
