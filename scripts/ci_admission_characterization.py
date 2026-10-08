#!/usr/bin/env python3
"""Retain bounded Worker-admission load characterization; never grant an SLO."""
import argparse
import hashlib
import json
import re
import sys
from decimal import Decimal, InvalidOperation
from pathlib import Path

TEST = "TestSustainedAdmissionSecurityMatrix"
MARKER = "LOAD_CHARACTERIZATION_NOT_SLO"
SHA = re.compile(r"^[0-9a-f]{40}$")
DURATION = re.compile(r"^([0-9]+(?:\.[0-9]+)?)(ns|µs|us|ms|s)$")
FIELDS = (
    "runs", "worker_identities", "security_rounds", "denied",
    "intake_wall", "worker_wall", "process_p50", "process_p95", "process_max",
)
EXPECTED = {"runs": 96, "worker_identities": 8, "security_rounds": 3, "denied": 48}
MAX_EVENTS_BYTES = 20 * 1024 * 1024
MAX_DURATION_NS = 10 * 60 * 1_000_000_000


def duration_ns(raw):
    match = DURATION.fullmatch(raw)
    if not match:
        raise ValueError("unsupported or malformed Go duration")
    factors = {"ns": 1, "µs": 1000, "us": 1000, "ms": 1000000, "s": 1000000000}
    try:
        value = Decimal(match.group(1)) * factors[match.group(2)]
    except InvalidOperation as exc:
        raise ValueError("invalid duration decimal") from exc
    if value != value.to_integral_value() or value < 0 or value > MAX_DURATION_NS:
        raise ValueError("unbounded or sub-nanosecond duration")
    return int(value)


def parse_events(raw, expected_rounds=5):
    if not raw or len(raw) > MAX_EVENTS_BYTES:
        raise ValueError("missing or oversized CI event stream")
    samples, passes = [], 0
    for idx, line in enumerate(raw.splitlines(), start=1):
        try:
            event = json.loads(line)
        except (ValueError, UnicodeDecodeError) as exc:
            raise ValueError(f"invalid JSON event on line {idx}") from exc
        if not isinstance(event, dict):
            raise ValueError("Go event is not an object")
        if event.get("Test") != TEST:
            continue
        if event.get("Action") == "fail":
            raise ValueError("admission characterization test failed")
        if event.get("Action") == "pass":
            passes += 1
            continue
        if event.get("Action") != "output":
            continue
        output = event.get("Output", "")
        if not isinstance(output, str) or MARKER not in output:
            continue
        if output.count(MARKER) != 1:
            raise ValueError("ambiguous characterization marker")
        payload = output.split(MARKER, 1)[1].strip()
        pairs = payload.split()
        if len(pairs) != len(FIELDS):
            raise ValueError("wrong number of metric fields")
        data = {}
        for pair in pairs:
            k, sep, v = pair.partition("=")
            if not sep or k in data or k not in FIELDS or not v:
                raise ValueError("unknown/duplicate/missing characterization key")
            data[k] = v
        if set(data) != set(FIELDS):
            raise ValueError("missing characterization fields")
        sample = {}
        for key, expected in EXPECTED.items():
            if data[key] != str(expected):
                raise ValueError(f"invalid fixed workload identity: {key}")
            sample[key] = expected
        for key in FIELDS[4:]:
            sample[key + "_ns"] = duration_ns(data[key])
        if not (
            0 < sample["process_p50_ns"]
            <= sample["process_p95_ns"]
            <= sample["process_max_ns"]
            <= sample["worker_wall_ns"] + sample["intake_wall_ns"] + MAX_DURATION_NS
        ):
            raise ValueError("invalid process latency ordering")
        samples.append(sample)
    if passes != expected_rounds or len(samples) != expected_rounds:
        raise ValueError(
            f"unqualified matrix: {len(samples)} measurements, {passes} pass events"
        )
    return [{"round": i + 1, **s} for i, s in enumerate(samples)]


def build_report(raw, sha, run_id, attempt):
    if not SHA.fullmatch(sha) or run_id < 1 or attempt < 1:
        raise ValueError("exact source SHA, CI run ID and attempt required")
    samples = parse_events(raw)
    return {
        "version": 1,
        "kind": MARKER,
        "source_sha": sha,
        "ci_run_id": run_id,
        "ci_attempt": attempt,
        "input_sha256": "sha256:" + hashlib.sha256(raw).hexdigest(),
        "qualification_granted": False,
        "production_qualified": False,
        "slo_qualified": False,
        "sample_unit": "ns",
        "aggregate": {
            "runs": sum(s["runs"] for s in samples),
            "denied_probes": sum(s["denied"] for s in samples),
            "test_rounds": len(samples),
            "worker_identities_per_round": 8,
        },
        "samples": samples,
    }


def main():
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--events", required=True, type=Path)
    ap.add_argument("--source-sha", required=True)
    ap.add_argument("--run-id", required=True, type=int)
    ap.add_argument("--attempt", required=True, type=int)
    ap.add_argument("--out", required=True, type=Path)
    args = ap.parse_args()
    raw = args.events.read_bytes()
    report = build_report(raw, args.source_sha, args.run_id, args.attempt)
    serialized = json.dumps(report, sort_keys=True, separators=(",", ":"), ensure_ascii=True) + "\n"
    if args.out.exists():
        raise ValueError("output exists: refusing to overwrite characterization report")
    with args.out.open("x", encoding="utf-8") as out:
        out.write(serialized)
        out.flush()
    print("admission-characterization: 5/5 PASS, 480 Runs, 240 denied; NOT SLO")


if __name__ == "__main__":
    try:
        main()
    except (OSError, ValueError) as exc:
        print(f"admission-characterization INVALID: {exc}", file=sys.stderr)
        sys.exit(1)
