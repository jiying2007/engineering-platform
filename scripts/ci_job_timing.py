#!/usr/bin/env python3
"""Exact-SHA GitHub Actions required-job timing readback; NOT cost/SLO qualification."""
import argparse
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import re
import sys

REQUIRED = (
    "codex-app-server-qualification",
    "go",
    "offline-container-integration",
    "postgres-authority-restore-drill",
)
SOURCE_SHA = re.compile(r"[0-9a-f]{40}\Z")
UTC_INSTANT = re.compile(r"\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?Z\Z")
MAX_SOURCE_BYTES = 4 * 1024 * 1024
MAX_JOB_MS = 60 * 60 * 1000
MAX_STEPS = 128


def utc_time(raw):
    if not isinstance(raw, str) or not UTC_INSTANT.fullmatch(raw):
        raise ValueError("invalid UTC GitHub Actions timestamp")
    try:
        at = datetime.fromisoformat(raw.replace("Z", "+00:00"))
    except ValueError as exc:
        raise ValueError("invalid timestamp value") from exc
    if at.utcoffset().total_seconds() != 0:
        raise ValueError("timestamp must be UTC")
    return at


def elapsed_ms(start, end):
    delta = end - start
    value = (delta.days * 86400 + delta.seconds) * 1000 + delta.microseconds // 1000
    if value < 0 or value > MAX_JOB_MS:
        raise ValueError("negative or unbounded elapsed time")
    return value


def build_report(raw, source_sha, run_id, run_attempt):
    if not SOURCE_SHA.fullmatch(source_sha) or run_id < 1 or run_attempt < 1:
        raise ValueError("exact SHA, run ID and attempt required")
    if not raw or len(raw) > MAX_SOURCE_BYTES:
        raise ValueError("missing/oversized GitHub jobs JSON")
    try:
        document = json.loads(raw)
    except (ValueError, UnicodeDecodeError) as exc:
        raise ValueError("invalid GitHub jobs JSON") from exc
    if not isinstance(document, dict) or not isinstance(document.get("jobs"), list):
        raise ValueError("missing jobs list")
    if len(document["jobs"]) > 100 or document.get("total_count") != len(document["jobs"]):
        raise ValueError("incomplete or oversized jobs first page")
    selected = {}
    for job in document["jobs"]:
        if not isinstance(job, dict):
            raise ValueError("malformed job object")
        name = job.get("name")
        if name not in REQUIRED:
            continue
        if name in selected:
            raise ValueError("duplicate required job")
        if job.get("status") != "completed" or job.get("conclusion") != "success":
            raise ValueError("required job is not successfully completed")
        if job.get("head_sha") != source_sha or job.get("run_id") != run_id or job.get("run_attempt") != run_attempt:
            raise ValueError("required job provenance drift")
        job_id = job.get("id")
        if type(job_id) is not int or job_id < 1:
            raise ValueError("invalid job identity")
        started, completed = utc_time(job.get("started_at")), utc_time(job.get("completed_at"))
        job_ms = elapsed_ms(started, completed)
        steps = job.get("steps")
        if not isinstance(steps, list) or not 1 <= len(steps) <= MAX_STEPS:
            raise ValueError("missing or unbounded job steps")
        result_steps, previous_number, seen_steps = [], 0, set()
        for step in steps:
            if not isinstance(step, dict):
                raise ValueError("malformed step object")
            number = step.get("number")
            name_step = step.get("name")
            if type(number) is not int or number <= previous_number or not isinstance(name_step, str) or not name_step or len(name_step) > 256 or name_step in seen_steps:
                raise ValueError("duplicate/out-of-order/invalid job step")
            previous_number = number
            seen_steps.add(name_step)
            if step.get("status") != "completed" or step.get("conclusion") not in ("success", "skipped"):
                raise ValueError("required job contains unfinished/failed step")
            ss, se = utc_time(step.get("started_at")), utc_time(step.get("completed_at"))
            ms = elapsed_ms(ss, se)
            if ss < started or se > completed or ms > job_ms:
                raise ValueError("step lies outside job execution window")
            result_steps.append({"name": name_step, "number": number, "elapsed_ms": ms, "conclusion": step["conclusion"]})
        selected[name] = {
            "name": name, "job_id": job_id,
            "started_at": job["started_at"], "completed_at": job["completed_at"],
            "elapsed_ms": job_ms, "steps": result_steps,
        }
    if set(selected) != set(REQUIRED):
        raise ValueError("missing required job timing")
    rows = [selected[name] for name in REQUIRED]
    return {
        "schema_version": 1,
        "kind": "REQUIRED_CI_JOB_TIMING_CHARACTERIZATION_NOT_BILLING",
        "source_sha": source_sha,
        "run_id": run_id,
        "run_attempt": run_attempt,
        "raw_jobs_sha256": "sha256:" + hashlib.sha256(raw).hexdigest(),
        "scope": "exact-four-successful-upstream-jobs",
        "jobs": rows,
        "aggregate": {
            "job_count": len(rows),
            "total_runner_wall_ms": sum(j["elapsed_ms"] for j in rows),
            "max_job_wall_ms": max(j["elapsed_ms"] for j in rows),
        },
        "billing_qualified": False,
        "slo_qualified": False,
        "provider_live_qualified": False,
        "production_qualified": False,
    }


def main():
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--jobs", required=True, type=Path)
    ap.add_argument("--source-sha", required=True)
    ap.add_argument("--run-id", required=True, type=int)
    ap.add_argument("--attempt", required=True, type=int)
    ap.add_argument("--out", required=True, type=Path)
    args = ap.parse_args()
    raw = args.jobs.read_bytes()
    report = build_report(raw, args.source_sha, args.run_id, args.attempt)
    payload = json.dumps(report, sort_keys=True, separators=(",", ":")) + "\n"
    with args.out.open("x", encoding="utf-8") as f:
        f.write(payload)
        f.flush()
    print("CI timing: 4/4 exact upstream jobs measured; not a billable-cost or production SLO claim")


if __name__ == "__main__":
    try:
        main()
    except (ValueError, OSError) as exc:
        print("CI timing INVALID: " + str(exc), file=sys.stderr)
        sys.exit(1)
