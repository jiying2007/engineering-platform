#!/usr/bin/env python3
"""Deterministic CI visibility regressions; no network, credentials or real sleep."""
import copy
import hashlib
import json
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

from ci_facts_readback import (FetchFailure, MAX_SOURCE_BYTES, PREFIXES, ReadbackTimeout,
                               collect, curl_fetch)
from ci_job_timing import build_report
from test_ci_job_timing import SHA, encode, source


def expected():
    return {"repository": "example/engineering-platform", "source_sha": SHA, "run_id": 456,
            "run_attempt": 1, "event": "pull_request",
            "producer_artifact_ids": {prefix + SHA: i + 201 for i, prefix in enumerate(PREFIXES)}}


def facts():
    return {
        "run.json": {"id": 456, "run_attempt": 1, "head_sha": SHA, "name": "CI",
                     "path": ".github/workflows/ci.yml", "event": "pull_request",
                     "repository": {"full_name": "example/engineering-platform", "id": 10},
                     "head_repository": {"full_name": "fork/engineering-platform", "id": 20},
                     "head_branch": "main", "status": "in_progress", "conclusion": None},
        "jobs.json": source(),
        "artifacts.json": {"total_count": 2, "artifacts": [
            {"id": i + 201, "name": prefix + SHA, "digest": "sha256:" + str(i + 1) * 64,
             "size_in_bytes": 100 + i, "expired": False, "workflow_run": {"id": 456, "head_sha": SHA}}
            for i, prefix in enumerate(PREFIXES)]},
    }


class FakeClock:
    def __init__(self):
        self.now = 0
        self.sleeps = []

    def monotonic(self):
        return self.now

    def sleep(self, seconds):
        self.sleeps.append(seconds)
        self.now += seconds


class Replay:
    def __init__(self, frames):
        self.frames, self.poll, self.calls = frames, -1, []

    def __call__(self, path, timeout):
        self.calls.append((path, timeout))
        if path == "/actions/runs/456/attempts/1":
            name = "run.json"
            self.poll += 1
        elif path == "/actions/runs/456/attempts/1/jobs?per_page=100":
            name = "jobs.json"
        elif path == "/actions/runs/456/artifacts?per_page=100":
            name = "artifacts.json"
        else:
            raise AssertionError("unexpected or unbound facts endpoint: " + path)
        value = self.frames[min(self.poll, len(self.frames) - 1)][name]
        if isinstance(value, Exception):
            raise value
        return value if isinstance(value, bytes) else encode(value)


class CIFactsReadbackTests(unittest.TestCase):
    def attempt(self, frames, identity=None, **options):
        self.output = Path(self.enterContext(tempfile.TemporaryDirectory()))
        self.clock, self.fetch = FakeClock(), Replay(frames)
        return collect(identity or expected(), self.output, self.fetch, clock=self.clock.monotonic,
                       sleep=self.clock.sleep, **options)

    def saved_report(self):
        return json.loads((self.output / "ci-facts-readback.json").read_bytes())

    def rejected(self, frame, message=None):
        with self.assertRaisesRegex(ValueError, message or ".+"):
            self.attempt([frame])
        report = self.saved_report()
        self.assertEqual(report["status"], "REJECTED")
        self.assertEqual(report["poll_count"], 1)
        self.assertEqual(self.clock.sleeps, [])
        self.assertIs(report["qualification_granted"], False)
        self.assertIs(report["production_qualified"], False)

    def test_unfinished_job_becomes_success_and_retains_exact_final_bytes(self):
        pending, ready = facts(), facts()
        pending["jobs.json"]["jobs"][0].update(status="in_progress", conclusion=None)
        ready["jobs.json"] = json.dumps(ready["jobs.json"], indent=3).encode() + b"\n"
        report = self.attempt([pending, ready])
        self.assertEqual(report["status"], "READY")
        self.assertEqual(report["poll_count"], 2)
        self.assertEqual(self.clock.sleeps, [5])
        self.assertEqual(self.saved_report(), report)
        self.assertEqual(set(report["raw_sha256"]), {"run.json", "jobs.json", "artifacts.json"})
        for name, digest in report["raw_sha256"].items():
            raw = self.output.joinpath(name).read_bytes()
            self.assertEqual(digest, "sha256:" + hashlib.sha256(raw).hexdigest())
        raw_jobs = self.output.joinpath("jobs.json").read_bytes()
        self.assertEqual(raw_jobs, ready["jobs.json"])
        self.assertEqual(build_report(raw_jobs, SHA, 456, 1)["raw_jobs_sha256"], report["raw_sha256"]["jobs.json"])
        self.assertIs(report["qualification_granted"], False)
        self.assertIs(report["production_qualified"], False)

    def test_completed_job_with_unfinished_step_waits_for_actual_step(self):
        pending = facts()
        pending["jobs.json"]["jobs"][0]["steps"][1].update(status="in_progress", conclusion=None)
        self.assertEqual(self.attempt([pending, facts()])["poll_count"], 2)
        self.assertEqual(self.clock.sleeps, [5])

    def test_missing_job_artifact_or_metadata_can_become_visible(self):
        for kind in ("job", "artifact", "digest", "size"):
            with self.subTest(kind=kind):
                pending = facts()
                if kind == "job":
                    pending["jobs.json"]["jobs"].pop(0)
                    pending["jobs.json"]["total_count"] -= 1
                elif kind == "artifact":
                    pending["artifacts.json"]["artifacts"].pop(0)
                    pending["artifacts.json"]["total_count"] -= 1
                else:
                    pending["artifacts.json"]["artifacts"][0].update(
                        {"digest": None} if kind == "digest" else {"size_in_bytes": 0})
                self.assertEqual(self.attempt([pending, facts()])["poll_count"], 2)

    def test_failed_job_or_step_is_immediately_rejected(self):
        for conclusion in ("failure", "cancelled", "timed_out", "skipped", "neutral", "stale", "action_required"):
            with self.subTest(conclusion=conclusion):
                frame = facts()
                frame["jobs.json"]["jobs"][0]["conclusion"] = conclusion
                self.rejected(frame, "unsuccessful conclusion")
        frame = facts()
        frame["jobs.json"]["jobs"][0].update(status="in_progress", conclusion=None)
        frame["jobs.json"]["jobs"][0]["steps"][0]["conclusion"] = "failure"
        self.rejected(frame, "unsuccessful step")

    def test_actual_run_failure_is_immediately_rejected(self):
        frame = facts()
        frame["run.json"].update(status="completed", conclusion="failure")
        self.rejected(frame, "unsuccessful conclusion")
        self.assertEqual(len(self.fetch.calls), 1)

    def test_persistent_pending_or_missing_facts_exhaust_total_budget(self):
        for kind in ("pending", "missing_job", "missing_artifact"):
            with self.subTest(kind=kind):
                frame = facts()
                if kind == "pending":
                    frame["jobs.json"]["jobs"][0].update(status="in_progress", conclusion=None)
                else:
                    name, rows = ("jobs.json", "jobs") if kind == "missing_job" else ("artifacts.json", "artifacts")
                    frame[name][rows].pop(0)
                    frame[name]["total_count"] -= 1
                with self.assertRaises(ReadbackTimeout):
                    self.attempt([frame], budget=10)
                self.assertEqual(self.saved_report()["status"], "TIMED_OUT")
                self.assertEqual(self.saved_report()["poll_count"], 2)
                self.assertEqual(self.clock.now, 10)
                self.assertEqual(self.clock.sleeps, [5, 5])
                self.assertEqual(len(self.fetch.calls), 6)

    def test_each_request_receives_only_remaining_budget(self):
        frame = facts()
        frame["jobs.json"]["jobs"][0].update(status="queued", conclusion=None)
        self.attempt([frame, facts()], budget=6)
        self.assertEqual([timeout for _, timeout in self.fetch.calls], [6, 6, 6, 1, 1, 1])

    def test_late_response_is_retained_but_cannot_be_accepted(self):
        clock = FakeClock()
        output = Path(self.enterContext(tempfile.TemporaryDirectory()))
        raw = encode(facts()["run.json"])

        def late_fetch(path, timeout):
            self.assertEqual(timeout, 6)
            clock.now += 6
            return raw

        with self.assertRaises(ReadbackTimeout):
            collect(expected(), output, late_fetch, budget=6, clock=clock.monotonic, sleep=clock.sleep)
        self.assertEqual(output.joinpath("run.json").read_bytes(), raw)
        self.assertEqual(json.loads(output.joinpath("ci-facts-readback.json").read_bytes())["status"], "TIMED_OUT")
        self.assertEqual(clock.sleeps, [])

    def test_identity_drift_is_never_retried(self):
        cases = (
            ("run.json", "id", 457), ("run.json", "run_attempt", 2),
            ("run.json", "head_sha", "b" * 40), ("run.json", "name", "Other"),
            ("run.json", "path", ".github/workflows/other.yml"), ("run.json", "event", "push"),
            ("run.json", "repository", {"full_name": "other/repository"}),
            ("job", "id", True), ("job", "run_id", 457), ("job", "run_attempt", 2),
            ("job", "head_sha", "b" * 40), ("artifact", "id", 999),
            ("artifact", "name", PREFIXES[0] + "b" * 40),
            ("artifact", "workflow_run", {"id": 457, "head_sha": SHA}),
            ("artifact", "workflow_run", {"id": 456, "head_sha": "b" * 40}),
        )
        for target, key, value in cases:
            with self.subTest(target=target, key=key, value=value):
                frame = facts()
                obj = frame["jobs.json"]["jobs"][0] if target == "job" else (
                    frame["artifacts.json"]["artifacts"][0] if target == "artifact" else frame[target])
                obj[key] = value
                self.rejected(frame)

    def test_job_id_and_visible_artifact_metadata_cannot_change_during_polling(self):
        for key in ("job_id", "digest", "size_in_bytes"):
            with self.subTest(key=key):
                pending, changed = facts(), facts()
                pending["jobs.json"]["jobs"][0].update(status="in_progress", conclusion=None)
                if key == "job_id":
                    changed["jobs.json"]["jobs"][0]["id"] += 10
                else:
                    changed["artifacts.json"]["artifacts"][0][key] = "sha256:" + "f" * 64 if key == "digest" else 999
                with self.assertRaisesRegex(ValueError, "changed during readback"):
                    self.attempt([pending, changed])
                self.assertEqual(self.saved_report()["status"], "REJECTED")
                self.assertEqual(self.saved_report()["poll_count"], 2)

    def test_duplicate_jobs_artifacts_and_truncated_pages_are_rejected(self):
        for filename, key in (("jobs.json", "jobs"), ("artifacts.json", "artifacts")):
            with self.subTest(filename=filename):
                frame = facts()
                frame[filename][key].append(copy.deepcopy(frame[filename][key][0]))
                frame[filename]["total_count"] += 1
                self.rejected(frame, "duplicate required")
                frame = facts()
                frame[filename]["total_count"] = 101
                self.rejected(frame, "incomplete/oversized")
        frame = facts()
        frame["jobs.json"]["jobs"][1]["id"] = frame["jobs.json"]["jobs"][0]["id"]
        self.rejected(frame, "duplicate required job ID")

    def test_malformed_artifact_fields_are_not_mistaken_for_visibility_delay(self):
        for digest, size in (("bad", 0), (None, -1), (None, False), ("", "100"),
                             (123, 0), ([], 0), ("sha256:" + "a" * 64, None)):
            with self.subTest(digest=digest, size=size):
                frame = facts()
                frame["artifacts.json"]["artifacts"][0].update(digest=digest, size_in_bytes=size)
                self.rejected(frame, "invalid required artifact")
        frame = facts()
        frame["artifacts.json"]["artifacts"][0]["expired"] = True
        self.rejected(frame, "expired")

    def test_invalid_json_is_preserved_and_rejected(self):
        for raw in (b"", b"null", b"not JSON", b"\xff", b'{"id":1,"id":2}',
                    b'{"number":NaN}', b'{"number":Infinity}', b'{"number":-Infinity}',
                    b" " * (MAX_SOURCE_BYTES + 1)):
            with self.subTest(length=len(raw)):
                frame = facts()
                frame["run.json"] = raw
                self.rejected(frame)
                self.assertEqual(self.output.joinpath("run.json").read_bytes(), raw)

    def test_existing_complete_timing_validation_still_controls_ready(self):
        for mutate in (
            lambda job: job.update(steps=[]),
            lambda job: job.update(started_at="invalid"),
            lambda job: job["steps"][1].update(number=1),
            lambda job: job["steps"][0].update(started_at="2026-10-07T23:59:59Z"),
        ):
            with self.subTest(mutate=mutate):
                frame = facts()
                mutate(frame["jobs.json"]["jobs"][0])
                self.rejected(frame)

    def test_fork_pr_is_supported_and_push_still_requires_main(self):
        self.assertEqual(self.attempt([facts()])["status"], "READY")
        identity, frame = expected(), facts()
        identity["event"] = frame["run.json"]["event"] = "push"
        self.assertEqual(self.attempt([frame], identity)["status"], "READY")
        frame["run.json"]["head_branch"] = "feature"
        with self.assertRaisesRegex(ValueError, "must bind main"):
            self.attempt([frame], identity)

    def test_fetch_failure_preserves_last_body_and_does_not_retry(self):
        frame = facts()
        raw = b'{"message":"synthetic unavailable"}'
        frame["jobs.json"] = FetchFailure("synthetic HTTP failure", raw)
        self.rejected(frame, "synthetic HTTP failure")
        self.assertEqual(self.output.joinpath("jobs.json").read_bytes(), raw)
        self.assertEqual(len(self.fetch.calls), 2)
        self.assertNotIn("artifacts.json", self.saved_report()["raw_sha256"])

    def test_curl_rejects_unsupported_streaming_limits_before_network(self):
        for version in (b"curl 8.3.0", b"curl 7.88.1", b"not curl"):
            with self.subTest(version=version), patch("ci_facts_readback.subprocess.run", return_value=
                    subprocess.CompletedProcess([], 0, version)) as run:
                with self.assertRaisesRegex(ValueError, "curl >= 8.4.0"):
                    curl_fetch("https://api.github.com/repos/example/repository", "synthetic-token", "/facts", 10)
                self.assertEqual(run.call_count, 1)

    def test_curl_bounds_request_and_keeps_auth_off_command_line(self):
        raw, token = b'{"synthetic":true}', "synthetic-token"
        responses = [subprocess.CompletedProcess([], 0, b"curl 8.4.0"),
                     subprocess.CompletedProcess([], 0, raw)]
        with patch("ci_facts_readback.subprocess.run", side_effect=responses) as run, \
                patch("ci_facts_readback.time.monotonic", side_effect=[10, 11]):
            self.assertEqual(curl_fetch("https://api.github.com/repos/example/repository", token, "/facts", 6), raw)
        command = run.call_args.args[0]
        self.assertNotIn(token, " ".join(command))
        self.assertEqual(command[:2], ["curl", "--disable"])
        for flag, value in (("--proto", "=https"), ("--noproxy", "*"), ("--max-time", "5"),
                            ("--max-filesize", str(MAX_SOURCE_BYTES)), ("--header", "@-")):
            self.assertEqual(command[command.index(flag) + 1], value)
        self.assertNotIn("-L", command)
        self.assertNotIn("--location", command)
        self.assertEqual(run.call_args.kwargs["timeout"], 5)
        self.assertIn(("Authorization: Bearer " + token).encode(), run.call_args.kwargs["input"])
        self.assertIs(run.call_args.kwargs["stderr"], subprocess.DEVNULL)

    def test_curl_failures_are_bounded_and_do_not_expose_auth(self):
        token, raw = "synthetic-token", b'{"message":"synthetic failure"}'
        for outcome in (subprocess.CompletedProcess([], 22, raw),
                        subprocess.TimeoutExpired(["curl", token], 1, output=raw),
                        subprocess.CompletedProcess([], 0, b"x" * (MAX_SOURCE_BYTES + 1))):
            with self.subTest(kind=type(outcome).__name__), patch("ci_facts_readback.subprocess.run",
                    side_effect=[subprocess.CompletedProcess([], 0, b"curl 8.5.0"), outcome]):
                with self.assertRaises(FetchFailure) as caught:
                    curl_fetch("https://api.github.com/repos/example/repository", token, "/facts", 1)
                self.assertNotIn(token, str(caught.exception))
                self.assertLessEqual(len(caught.exception.raw), MAX_SOURCE_BYTES)
                if not isinstance(outcome, subprocess.CompletedProcess) or outcome.returncode:
                    self.assertEqual(caught.exception.raw, raw)


if __name__ == "__main__":
    unittest.main(verbosity=2)
