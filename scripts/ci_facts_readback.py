#!/usr/bin/env python3
"""Bounded readback of exact CI facts; never infer API success from needs."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import time

from ci_job_timing import MAX_SOURCE_BYTES, REQUIRED, SOURCE_SHA, build_report

MAX_SECONDS = 120
POLL_SECONDS = 5
REQUEST_SECONDS = 10
PREFIXES = ("engineering-binaries-", "codex-compatibility-qualification-")
PENDING = {"queued", "in_progress", "waiting", "pending", "requested"}
DIGEST = re.compile(r"sha256:[0-9a-f]{64}\Z")


class ReadbackTimeout(TimeoutError):
    pass


class FetchFailure(ValueError):
    def __init__(self, message, raw=b""):
        super().__init__(message)
        self.raw = raw


def require(condition, message):
    if not condition:
        raise ValueError(message)


def positive(value):
    return type(value) is int and value > 0


def unique_pairs(pairs):
    result = {}
    for key, value in pairs:
        require(key not in result, "duplicate JSON field")
        result[key] = value
    return result


def invalid_constant(_):
    raise ValueError("non-finite JSON number")


def document(raw):
    require(0 < len(raw) <= MAX_SOURCE_BYTES, "missing/oversized GitHub facts")
    try:
        value = json.loads(raw, object_pairs_hook=unique_pairs, parse_constant=invalid_constant)
    except (ValueError, UnicodeDecodeError, RecursionError) as exc:
        raise ValueError("invalid GitHub facts JSON") from exc
    require(isinstance(value, dict), "GitHub facts object required")
    return value


def page(raw, key):
    value = document(raw)
    rows = value.get(key)
    require(isinstance(rows, list) and type(value.get("total_count")) is int and
            value["total_count"] == len(rows) <= 100, "incomplete/oversized GitHub facts page")
    require(all(isinstance(row, dict) for row in rows), "malformed GitHub facts row")
    return rows


def verify_run(raw, expected):
    run = document(raw)
    require(positive(run.get("id")) and run["id"] == expected["run_id"] and
            positive(run.get("run_attempt")) and run["run_attempt"] == expected["run_attempt"] and
            run.get("head_sha") == expected["source_sha"] and run.get("name") == "CI" and
            run.get("path") == ".github/workflows/ci.yml" and run.get("event") == expected["event"] and
            isinstance(run.get("repository"), dict) and
            run["repository"].get("full_name") == expected["repository"], "workflow run identity drift")
    if expected["event"] == "push":
        require(run.get("head_branch") == "main", "push facts must bind main")
    # The collecting job is part of this run, so the run itself can be unfinished.
    require(isinstance(run.get("status"), str) and run["status"] in PENDING | {"completed"}, "invalid workflow run status")
    require(run.get("conclusion") in (None, "success"), "workflow run has an unsuccessful conclusion")


def jobs_ready(raw, expected, seen_ids):
    selected = {}
    ready = True
    for job in page(raw, "jobs"):
        name = job.get("name")
        if name not in REQUIRED:
            continue
        require(name not in selected, "duplicate required job")
        require(positive(job.get("id")) and positive(job.get("run_id")) and
                positive(job.get("run_attempt")) and job["run_id"] == expected["run_id"] and
                job["run_attempt"] == expected["run_attempt"] and
                job.get("head_sha") == expected["source_sha"], "required job identity drift")
        require(name not in seen_ids or seen_ids[name] == job["id"], "required job ID changed during readback")
        seen_ids[name] = job["id"]
        require(job["id"] not in selected.values(), "duplicate required job ID")
        selected[name] = job["id"]
        require(isinstance(job.get("status"), str) and job["status"] in PENDING | {"completed"}, "invalid required job status")
        require(job.get("conclusion") in (None, "success"), "required job has an unsuccessful conclusion")
        steps = job.get("steps", [])
        require(isinstance(steps, list), "malformed required job steps")
        steps_done = True
        for step in steps:
            require(isinstance(step, dict), "malformed required job step")
            require(isinstance(step.get("status"), str) and step["status"] in PENDING | {"completed"}, "invalid required step status")
            require(step.get("conclusion") in (None, "success", "skipped"), "required job has an unsuccessful step")
            steps_done = steps_done and step["status"] == "completed" and step.get("conclusion") in ("success", "skipped")
        ready = ready and job["status"] == "completed" and job.get("conclusion") == "success" and steps_done
    return set(selected) == set(REQUIRED) and ready


def artifacts_ready(raw, expected, seen_metadata):
    selected = set()
    ready = True
    for artifact in page(raw, "artifacts"):
        name = artifact.get("name")
        require(isinstance(name, str), "invalid artifact name")
        if not name.startswith(PREFIXES):
            continue
        require(name in expected["producer_artifact_ids"], "artifact source-name drift")
        require(name not in selected, "duplicate required artifact")
        selected.add(name)
        require(positive(artifact.get("id")) and artifact["id"] == expected["producer_artifact_ids"][name],
                "artifact differs from producer upload ID")
        run = artifact.get("workflow_run")
        require(isinstance(run, dict) and positive(run.get("id")) and run["id"] == expected["run_id"] and
                run.get("head_sha") == expected["source_sha"], "artifact run/source drift")
        require(artifact.get("expired") is False, "required artifact expired or expiration unknown")
        digest, size = artifact.get("digest"), artifact.get("size_in_bytes")
        require(digest is None or isinstance(digest, str) and (not digest or DIGEST.fullmatch(digest)),
                "invalid required artifact digest")
        require(type(size) is int and size >= 0, "invalid required artifact size")
        if digest in (None, "") or size == 0:
            ready = False
        else:
            require(name not in seen_metadata or seen_metadata[name] == (digest, size),
                    "artifact digest/size changed during readback")
            seen_metadata[name] = (digest, size)
    return selected == set(expected["producer_artifact_ids"]) and ready


def curl_fetch(api, token, path, timeout):
    # Fixed HTTPS origin, no curlrc/proxy/redirects; auth travels on stdin, never
    # in an exception's command line. Both response bytes and process life bound.
    started = time.monotonic()
    try:
        version = subprocess.run(["curl", "--disable", "--version"], stdout=subprocess.PIPE,
                                 stderr=subprocess.DEVNULL, timeout=min(2, timeout))
    except subprocess.TimeoutExpired:
        raise FetchFailure("curl version check exceeded its request budget") from None
    match = re.match(rb"curl ([0-9]+)\.([0-9]+)\.([0-9]+)\b", version.stdout)
    require(version.returncode == 0 and match and tuple(map(int, match.groups())) >= (8, 4, 0),
            "curl >= 8.4.0 required for a streaming response byte limit")
    timeout -= time.monotonic() - started
    if timeout <= 0:
        raise FetchFailure("GitHub read exceeded its request budget")
    command = ["curl", "--disable", "--silent", "--show-error", "--fail-with-body",
               "--proto", "=https", "--noproxy", "*", "--connect-timeout", str(min(5, timeout)),
               "--max-time", str(timeout), "--max-filesize", str(MAX_SOURCE_BYTES), "--header", "@-", api + path]
    headers = ("Authorization: Bearer " + token + "\nAccept: application/vnd.github+json\n"
               "X-GitHub-Api-Version: 2022-11-28\n").encode()
    try:
        result = subprocess.run(command, input=headers, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, timeout=timeout)
    except subprocess.TimeoutExpired as exc:
        raise FetchFailure("GitHub read exceeded its request budget", (exc.output or b"")[:MAX_SOURCE_BYTES]) from None
    if result.returncode != 0:
        raise FetchFailure("GitHub read failed (curl exit %d)" % result.returncode, result.stdout[:MAX_SOURCE_BYTES])
    if len(result.stdout) > MAX_SOURCE_BYTES:
        raise FetchFailure("GitHub response exceeds byte limit", result.stdout[:MAX_SOURCE_BYTES])
    return result.stdout


def collect(expected, output, fetch, *, budget=MAX_SECONDS, interval=POLL_SECONDS, clock=time.monotonic, sleep=time.sleep):
    require(0 < budget <= MAX_SECONDS and 0 < interval <= POLL_SECONDS, "bounded readback budget required")
    require(SOURCE_SHA.fullmatch(expected["source_sha"]) and positive(expected["run_id"]) and
            positive(expected["run_attempt"]) and expected["event"] in ("push", "pull_request") and
            re.fullmatch(r"[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+", expected["repository"]), "exact CI identity required")
    ids = expected["producer_artifact_ids"]
    require(set(ids) == {prefix + expected["source_sha"] for prefix in PREFIXES} and
            all(positive(value) for value in ids.values()) and len(set(ids.values())) == 2, "distinct producer upload IDs required")
    output.mkdir(parents=True, exist_ok=True)
    started = clock()
    deadline = started + budget
    report = {"schema_version": 1, "kind": "BOUNDED_CI_FACT_READBACK", "workflow": "CI", **expected,
              "status": "REJECTED", "poll_count": 0, "budget_seconds": budget, "raw_sha256": {},
              "qualification_granted": False, "production_qualified": False}
    seen_ids, seen_metadata = {}, {}

    def remaining():
        value = deadline - clock()
        if value <= 0:
            raise ReadbackTimeout("required CI facts did not become ready within the total budget")
        return value

    def save(name, raw):
        (output / name).write_bytes(raw)
        report["raw_sha256"][name] = "sha256:" + hashlib.sha256(raw).hexdigest()

    def read(name, path):
        try:
            raw = fetch(path, min(REQUEST_SECONDS, remaining()))
        except FetchFailure as exc:
            save(name, exc.raw)
            raise
        save(name, raw)  # Retain the exact response before any parser can reject it.
        remaining()
        return raw

    run_path = "/actions/runs/%d/attempts/%d" % (expected["run_id"], expected["run_attempt"])
    try:
        # The attempt count is also bounded if a substituted clock does not advance.
        for poll in range(1, int(budget / interval) + 2):
            remaining()
            report["poll_count"] = poll
            verify_run(read("run.json", run_path), expected)
            jobs = read("jobs.json", run_path + "/jobs?per_page=100")
            jobs_done = jobs_ready(jobs, expected, seen_ids)
            artifacts_done = artifacts_ready(read("artifacts.json", "/actions/runs/%d/artifacts?per_page=100" % expected["run_id"]), expected, seen_metadata)
            if jobs_done and artifacts_done:
                # Keep the existing complete timing/provenance validator as the
                # final success gate, and preserve its exact input bytes on disk.
                build_report(jobs, expected["source_sha"], expected["run_id"], expected["run_attempt"])
                remaining()
                report["status"] = "READY"
                report["detail"] = "exact upstream jobs and producer artifacts read back"
                return report
            report["detail"] = "waiting for completed required jobs or visible producer artifacts"
            sleep(min(interval, remaining()))
        raise ReadbackTimeout("required CI facts exhausted the bounded poll count")
    except ReadbackTimeout as exc:
        report["status"], report["detail"] = "TIMED_OUT", str(exc)
        raise
    except (ValueError, OSError) as exc:
        report["detail"] = str(exc)
        raise
    finally:
        report["elapsed_ms"] = max(0, int((clock() - started) * 1000))
        (output / "ci-facts-readback.json").write_text(json.dumps(report, sort_keys=True, indent=2) + "\n")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--repository", required=True)
    parser.add_argument("--source-sha", required=True)
    parser.add_argument("--run-id", required=True, type=int)
    parser.add_argument("--attempt", required=True, type=int)
    parser.add_argument("--event", required=True)
    parser.add_argument("--binary-artifact-id", required=True, type=int)
    parser.add_argument("--codex-artifact-id", required=True, type=int)
    parser.add_argument("--out", required=True, type=Path)
    args = parser.parse_args()
    token = os.environ.get("GH_TOKEN", "")
    require(token and not any(character.isspace() for character in token), "GitHub read token required")
    expected = {"repository": args.repository, "source_sha": args.source_sha, "run_id": args.run_id,
                "run_attempt": args.attempt, "event": args.event,
                "producer_artifact_ids": dict(zip((prefix + args.source_sha for prefix in PREFIXES),
                                                  (args.binary_artifact_id, args.codex_artifact_id)))}
    collect(expected, args.out, lambda path, timeout: curl_fetch("https://api.github.com/repos/" + args.repository, token, path, timeout))
    print("CI facts READY: exact API jobs and producer artifacts; no qualification granted")


if __name__ == "__main__":
    try:
        main()
    except (ValueError, OSError) as exc:
        print("CI facts REJECTED: " + str(exc), file=sys.stderr)
        sys.exit(1)
