#!/usr/bin/env python3
"""Retire explicit integrated refs with whole-tree proof and atomic SHA leases.

This is repository maintenance, not a Core engineering action or maturity gate.
Only the CLI used by protected-main CI may mutate the fixed GitHub repository.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import sys

REPOSITORY = "jiying2007/engineering-platform"
REMOTE = "https://github.com/" + REPOSITORY
SHA = re.compile(r"[0-9a-f]{40}\Z")
BRANCH = re.compile(r"(?:chore|ci|docs|feat|fix|test)/[a-z0-9][a-z0-9/-]*\Z")


def require(value, message):
    if not value:
        raise ValueError(message)


def git(root, *args):
    result = subprocess.run(["git", "-C", str(root), *args], capture_output=True, text=True, timeout=60)
    require(result.returncode == 0, "Git operation failed: " + args[0])
    return result.stdout.strip()


def pairs(entries):
    result = {}
    for key, value in entries:
        require(key not in result, "duplicate manifest key")
        result[key] = value
    return result


def validate_manifest(value):
    require(isinstance(value, dict) and set(value) == {"version", "repository", "branches"}, "invalid manifest shape")
    require(type(value["version"]) is int and value["version"] == 1 and value["repository"] == REPOSITORY, "invalid retirement manifest")
    require(isinstance(value["branches"], list) and 0 < len(value["branches"]) <= 64, "bounded explicit branch list required")
    seen = set()
    for entry in value["branches"]:
        require(isinstance(entry, dict) and set(entry) == {"branch", "expected_head", "integrated_commit", "tree"}, "invalid branch entry")
        name = entry["branch"]
        require(isinstance(name, str) and BRANCH.fullmatch(name) and len(name) <= 200 and name not in seen, "unapproved or duplicate branch")
        require("//" not in name and not name.endswith("/"), "invalid ref name")
        for key in ("expected_head", "integrated_commit", "tree"):
            require(isinstance(entry[key], str) and SHA.fullmatch(entry[key]), "exact Git identities required")
        seen.add(name)
    return value


def inventory(root):
    result = {}
    for line in git(root, "ls-remote", "--heads", "origin").splitlines():
        sha, ref = line.split()
        require(SHA.fullmatch(sha) and ref.startswith("refs/heads/") and ref not in result, "invalid remote inventory")
        result[ref] = sha
    return result


def plan(root, value, expected_main):
    validate_manifest(value)
    require(isinstance(expected_main, str) and SHA.fullmatch(expected_main), "exact verified main required")
    heads = inventory(root)
    require(heads.get("refs/heads/main") == expected_main, "main drifted since verified CI")
    require(git(root, "rev-parse", "HEAD") == expected_main, "checkout is not the verified main")
    results = []
    for entry in value["branches"]:
        ref = "refs/heads/" + entry["branch"]
        git(root, "check-ref-format", ref)
        git(root, "merge-base", "--is-ancestor", entry["integrated_commit"], expected_main)
        require(git(root, "rev-parse", entry["integrated_commit"] + "^{tree}") == entry["tree"], "integration tree proof mismatch")
        actual = heads.get(ref)
        if actual is not None:
            require(actual == entry["expected_head"], "candidate ref drift; nothing will be deleted")
            require(git(root, "rev-parse", actual + "^{tree}") == entry["tree"], "candidate has unintegrated content")
        results.append(dict(entry, state="ABSENT" if actual is None else "ELIGIBLE"))
    return results


def push_arguments(results):
    pending = [r for r in results if r["state"] == "ELIGIBLE"]
    require(pending, "no pending refs")
    return ["push", "--atomic", "--porcelain"] + [
        "--force-with-lease=refs/heads/" + r["branch"] + ":" + r["expected_head"] for r in pending
    ] + ["origin"] + [":refs/heads/" + r["branch"] for r in pending]


def retire(root, value, expected_main, apply=False):
    results = plan(root, value, expected_main)
    attempted = apply and any(r["state"] == "ELIGIBLE" for r in results)
    if attempted:
        # One atomic deletion, each ref guarded by its exact old object ID.
        # No blind retry or non-atomic fallback is permitted.
        git(root, *push_arguments(results))
        heads = inventory(root)
        require(all("refs/heads/" + r["branch"] not in heads for r in results), "deletion readback requires reconciliation")
        for result in results:
            if result["state"] == "ELIGIBLE":
                result["state"] = "DELETED_READBACK_VERIFIED"
    return {"version": 1, "repository": REPOSITORY, "verified_main": expected_main,
            "status": "APPLIED" if apply else "DRY_RUN", "mutation_attempted": bool(attempted), "branches": results}


def merged_head_from_pull(pr, expected_main, main_tree):
    require(isinstance(pr, dict) and isinstance(expected_main, str) and SHA.fullmatch(expected_main), "valid merged PR required")
    require(isinstance(main_tree, str) and SHA.fullmatch(main_tree), "exact main tree required")
    head = pr.get("head") or {}
    base = pr.get("base") or {}
    head_repo = head.get("repo") or {}
    base_repo = base.get("repo") or {}
    name, sha = head.get("ref"), head.get("sha")
    require(pr.get("state") == "closed" and pr.get("merged_at") and pr.get("merge_commit_sha") == expected_main, "verified main must be an exact merged PR commit")
    require(base.get("ref") == "main" and base_repo.get("id") == 1383377268, "merged PR must target this repository main")
    require(head_repo.get("id") == 1383377268 and head_repo.get("full_name") == REPOSITORY, "merged head must belong to this repository")
    require(isinstance(name, str) and BRANCH.fullmatch(name) and len(name) <= 200 and "//" not in name and not name.endswith("/"), "merged head branch is not retireable")
    require(isinstance(sha, str) and SHA.fullmatch(sha), "exact merged head SHA required")
    return {"branch": name, "expected_head": sha, "integrated_commit": expected_main, "tree": main_tree}


def merged_head_entry(root, expected_main):
    result = subprocess.run(
        ["gh", "api", "repos/" + REPOSITORY + "/commits/" + expected_main + "/pulls"],
        capture_output=True, text=True, timeout=60,
    )
    require(result.returncode == 0, "cannot resolve merged PR for verified main")
    pulls = json.loads(result.stdout)
    require(isinstance(pulls, list) and 0 < len(pulls) <= 16, "bounded merged PR association required")
    matching = [
        pr for pr in pulls
        if isinstance(pr, dict)
        and pr.get("merge_commit_sha") == expected_main
        and pr.get("merged_at")
        and (pr.get("base") or {}).get("ref") == "main"
        and ((pr.get("head") or {}).get("repo") or {}).get("id") == 1383377268
    ]
    require(len(matching) == 1, "exact merged PR association required")
    main_tree = git(root, "rev-parse", expected_main + "^{tree}")
    entry = merged_head_from_pull(matching[0], expected_main, main_tree)
    heads = inventory(root)
    actual = heads.get("refs/heads/" + entry["branch"])
    if actual is not None:
        require(actual == entry["expected_head"], "merged PR head drift; nothing will be deleted")
        require(git(root, "rev-parse", actual + "^{tree}") == main_tree, "merged PR head tree differs from verified main")
    return entry


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--manifest", required=True)
    parser.add_argument("--expected-main", required=True)
    parser.add_argument("--apply", action="store_true")
    parser.add_argument("--retire-merged-head", action="store_true")
    args = parser.parse_args()
    if args.retire_merged_head and not args.apply:
        parser.error("--retire-merged-head requires --apply")
    report = {"version": 1, "status": "REJECTED", "mutation_state": "NOT_ATTEMPTED_OR_REQUIRES_RECONCILIATION"}
    try:
        root = Path.cwd().resolve()
        path = Path(args.manifest).absolute()
        require(path == path.resolve() and path.is_file() and 0 < path.stat().st_size <= 65536, "bounded non-aliased manifest required")
        raw = path.read_bytes()
        value = validate_manifest(json.loads(raw, object_pairs_hook=pairs))
        merged_head = None
        if args.apply:
            require(os.environ.get("GITHUB_REPOSITORY") == REPOSITORY and os.environ.get("GITHUB_REPOSITORY_ID") == "1383377268", "fixed GitHub repository required")
            require(os.environ.get("GITHUB_EVENT_NAME") == "workflow_run" and os.environ.get("GITHUB_REF") == "refs/heads/main" and os.environ.get("GITHUB_REF_PROTECTED") == "true", "protected-main CI completion required")
            event = json.loads(Path(os.environ["GITHUB_EVENT_PATH"]).read_text())
            run = event["workflow_run"]
            require(run["event"] == "push" and run["name"] == "CI" and run["conclusion"] == "success" and run["head_branch"] == "main" and run["head_sha"] == args.expected_main and run["head_repository"]["id"] == 1383377268, "exact same-repository successful main CI required")
            require(git(root, "remote", "get-url", "origin") in (REMOTE, REMOTE + ".git"), "unexpected mutation destination")
            if args.retire_merged_head:
                merged_head = merged_head_entry(root, args.expected_main)
                require(merged_head["branch"] not in {e["branch"] for e in value["branches"]}, "merged head already appears in manifest")
                value = validate_manifest({**value, "branches": value["branches"] + [merged_head]})
            active = subprocess.run(["gh", "api", "--paginate", "repos/" + REPOSITORY + "/pulls?state=open&per_page=100", "--jq", ".[].head.ref"], capture_output=True, text=True, timeout=60)
            require(active.returncode == 0, "cannot verify open PR inventory")
            require(not ({e["branch"] for e in value["branches"]} & set(active.stdout.splitlines())), "candidate still has an open PR")
        report = retire(root, value, args.expected_main, args.apply)
        report["manifest_digest"] = "sha256:" + hashlib.sha256(raw).hexdigest()
        if merged_head is not None:
            report["merged_head_branch"] = merged_head["branch"]
        print(json.dumps(report, sort_keys=True, indent=2))
        return 0
    except (ValueError, KeyError, TypeError, OSError, subprocess.SubprocessError) as error:
        report["reason"] = str(error)
        print(json.dumps(report, sort_keys=True, indent=2))
        return 1


if __name__ == "__main__":
    sys.exit(main())
