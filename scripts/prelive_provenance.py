#!/usr/bin/env python3
"""Bind the fixed account-free pre-live test inventory to actual Git source.
This validates test execution facts, not a model/provider or production gate.
"""
import argparse
import hashlib
import json
from pathlib import Path
import re
import subprocess
import sys

PREFIX = "github.com/jiying2007/engineering-platform/"
EXPECTED = {
    PREFIX + "internal/production": {
        "TestTerminalPlanIsDeterministicAndRequiresHumanReview",
        "TestTerminalPlanRejectsWeakenedRolloutAndEmergencyContract",
        "TestSLOReportAllowsProviderPendingButRequiresAllInternalMeasurements",
        "TestSLOReportFailsClosedOnMissingOrUnknownObservation",
    },
    PREFIX + "internal/api": {"TestProductionTerminalMaintenanceTemplatesDryRunToRunning"},
}
SHA = re.compile(r"[0-9a-f]{40}\Z")
MAX_BYTES = 16 << 20


def require(condition, message):
    if not condition:
        raise ValueError(message)


def unique_pairs(pairs):
    result = {}
    for key, value in pairs:
        require(key not in result, "duplicate JSON field")
        result[key] = value
    return result


def load(raw):
    return json.loads(raw, object_pairs_hook=unique_pairs)


def validate_events(raw):
    require(0 < len(raw) <= MAX_BYTES, "bounded nonempty test events required")
    tests, packages, elapsed = {}, {}, {}
    for line in raw.splitlines():
        event = load(line)
        package, name, action = event.get("Package"), event.get("Test"), event.get("Action")
        require(package in EXPECTED, "unexpected or missing package")
        require(action in {"start", "run", "pause", "cont", "output", "pass", "fail", "skip", "build-output", "build-fail"}, "unknown test event")
        require(action not in {"fail", "skip", "build-fail"}, "required test or package failed/skipped")
        if name:
            require(name.split("/")[0] in EXPECTED[package], "test outside frozen inventory")
            key = (package, name)
            if action == "run":
                require(key not in tests, "duplicate test execution")
                tests[key] = "run"
            elif action == "pass":
                require(tests.get(key) == "run", "test PASS without unique RUN")
                tests[key] = "pass"
                elapsed[key] = event.get("Elapsed", 0)
        elif action == "start":
            require(package not in packages, "duplicate package execution")
            packages[package] = "start"
        elif action == "pass":
            require(packages.get(package) == "start", "package PASS without START")
            packages[package] = "pass"
    require(packages == {p: "pass" for p in EXPECTED}, "missing or unfinished test package")
    require(all(tests.get((p, n)) == "pass" for p, names in EXPECTED.items() for n in names), "required top-level test did not execute")
    require(tests and all(value == "pass" for value in tests.values()), "unfinished test")
    return [{"package": p, "test": n, "result": "PASS", "elapsed_seconds": elapsed[(p, n)]} for p, n in sorted(tests)]


def git(*args):
    return subprocess.check_output(["git", *args], text=True, timeout=10).strip()


def build_report(expected_sha, events, plan_raw):
    require(SHA.fullmatch(expected_sha), "exact expected source SHA required")
    actual, tree = git("rev-parse", "HEAD"), git("rev-parse", "HEAD^{tree}")
    require(actual == expected_sha and SHA.fullmatch(tree), "event/checkout source drift")
    require(not git("status", "--porcelain", "--untracked-files=no"), "tracked source changed during validation")
    inventory = validate_events(events)
    plan = load(plan_raw)
    p = plan["plan"]
    require(p["version"] == 3 and p["human_review_required"] is True and p["provider_live_required"] is True, "terminal plan version or external gate drift")
    require(p["repository"] == "jiying2007/engineering-platform" and p["max_engineering_model_turns"] == 1, "terminal subject drift")
    require(p["deployment_profile"] == "canary-single-maintenance-fixture", "terminal canary profile drift")
    require(p["database_rollback_policy"] == "restore-authoritative-backup-and-reconcile", "terminal database rollback policy drift")
    require(p["emergency_stops"] == [
        "provider-credential-or-rule-disable",
        "publisher-credential-revoke",
        "worker-execution-stop",
    ], "terminal emergency-stop contract drift")
    require(p["no_silent_provider_fallback"] is True and p["no_automatic_database_downgrade"] is True,
            "terminal fallback/downgrade contract weakened")
    for required in (
        "canary_deployment_accepted",
        "provider_emergency_disable_proven",
        "publisher_revocation_proven",
        "database_rollback_restore_policy_accepted",
    ):
        require(required in p["required_gates"], "terminal v3 required gate missing")
    canonical_plan = json.dumps(p, separators=(",", ":"), ensure_ascii=False).encode()
    require(plan["plan_digest"] == "sha256:" + hashlib.sha256(canonical_plan).hexdigest(), "terminal plan digest mismatch")
    return {"version": 1, "source_sha": actual, "source_tree": tree, "terminal_plan_version": p["version"],
            "terminal_plan_digest": plan["plan_digest"], "test_events_digest": "sha256:" + hashlib.sha256(events).hexdigest(),
            "tests": inventory, "top_level_test_count": sum(len(names) for names in EXPECTED.values()),
            "scope": "provider-free-contract-tests", "provider_executed": False,
            "human_review_performed": False, "production_qualified": False}


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--expected-sha", required=True)
    p.add_argument("--events", required=True)
    p.add_argument("--plan", required=True)
    p.add_argument("--out", required=True)
    args = p.parse_args()
    try:
        events = Path(args.events).read_bytes()
        report = build_report(args.expected_sha, events, Path(args.plan).read_bytes())
        with Path(args.out).open("x") as stream:
            json.dump(report, stream, sort_keys=True, indent=2, allow_nan=False)
            stream.write("\n")
    except (ValueError, KeyError, TypeError, OSError, subprocess.SubprocessError) as error:
        print("pre-live provenance rejected: " + str(error), file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
