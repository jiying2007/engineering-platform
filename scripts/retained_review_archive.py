#!/usr/bin/env python3
"""Bounded terminal review facts. Never a runtime restore or provider admission.

Retain original UTF-8 bytes from an explicit allowlist, not a runner directory.
An externally pinned archive digest (or protected Git commit) anchors readback.
Historical state versions are preserved as evidence, not migrated or admitted.
"""
import argparse
import gzip
import io
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import re
import stat
import sys
import zipfile

REPOSITORY = "jiying2007/engineering-platform"
SCOPE = "terminal-review-facts-only;not-runtime-restore;not-production-qualification"
MAX_MEMBER = 256 * 1024
MAX_ARCHIVE = 4 * 1024 * 1024
COMMON = (
    "review-state.json", "review-response.json",
    "verification/verification-state.json", "verification/verification-response.json",
    "verification/delivery-response.json", "verification/ci-evidence.json",
    "verification/codex-evidence.json", "verification/git-evidence.json",
    "verification/codex-receipt-digest.json", "verification/git-change-manifest.json",
    "verification/engineering/engineering-state.json",
    "verification/engineering/publication-receipt.json",
)
CLOSED = ("closure-response.json", "work-closed.json")
REJECTED = ("work-after-fail-review.json",)
DIGEST = re.compile(r"sha256:[0-9a-f]{64}\Z")
COMMIT = re.compile(r"[0-9a-f]{40}\Z")
SECRET = re.compile(r"-----BEGIN [A-Z ]*PRIVATE KEY-----|\b(?:gh[pousr]_[A-Za-z0-9]{16,}|github_pat_[A-Za-z0-9_]{20,}|sk-[A-Za-z0-9_-]{20,})|\bBearer\s+\S+", re.I)
SECRET_KEYS = {"password", "private_key", "access_token", "refresh_token", "id_token", "authorization", "client_secret", "api_key"}


def require(condition, message):
    if not condition:
        raise ValueError(message)


def digest(data):
    return "sha256:" + hashlib.sha256(data).hexdigest()


def canonical(value):
    return (json.dumps(value, ensure_ascii=True, sort_keys=True, indent=2, allow_nan=False) + "\n").encode()


def pairs(items):
    value = {}
    for key, item in items:
        require(key not in value, "duplicate JSON key")
        value[key] = item
    return value


def decode(data):
    def invalid_constant(_):
        raise ValueError("non-finite JSON number")
    value = json.loads(data.decode("utf-8"), object_pairs_hook=pairs, parse_constant=invalid_constant)
    require(isinstance(value, dict), "JSON object required")
    return value


def scan(value):
    if isinstance(value, dict):
        for key, item in value.items():
            require(key.lower() not in SECRET_KEYS, "credential field in selected facts")
            scan(item)
    elif isinstance(value, list):
        for item in value:
            scan(item)
    elif isinstance(value, str):
        require(not SECRET.search(value), "credential-like material in selected facts")


def regular_bytes(path, limit):
    path = Path(path)
    require(path.absolute() == path.resolve(), "aliased input path")
    before = path.lstat()
    require(stat.S_ISREG(before.st_mode) and 0 < before.st_size <= limit, "bounded regular file required")
    fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW)
    with os.fdopen(fd, "rb") as source:
        opened = os.fstat(source.fileno())
        require((opened.st_dev, opened.st_ino) == (before.st_dev, before.st_ino), "input changed before read")
        data = source.read(limit + 1)
        after = os.fstat(source.fileno())
    current = path.lstat()
    identity = lambda info: (info.st_dev, info.st_ino, info.st_size, info.st_mtime_ns, info.st_ctime_ns)
    require(identity(before) == identity(after) == identity(current) and len(data) == before.st_size, "input changed during read")
    return data


def source_members(path, origin):
    path = Path(path).absolute()
    require(path == path.resolve(), "aliased source root")
    require(type(origin.get("review_run_id")) is int and origin["review_run_id"] > 0, "review run required")
    if path.is_dir():
        require(set(origin) == {"kind", "review_run_id"} and origin["kind"] == "workflow-directory", "invalid directory origin")
        def read(name):
            return regular_bytes(path / name, MAX_MEMBER)
        close = lambda: None
    else:
        require(set(origin) == {"kind", "review_run_id", "artifact_id", "archive_digest"}, "invalid ZIP origin")
        require(origin["kind"] == "github-actions-zip" and type(origin["artifact_id"]) is int and origin["artifact_id"] > 0, "artifact identity required")
        require(isinstance(origin["archive_digest"], str) and DIGEST.fullmatch(origin["archive_digest"]), "pinned source digest required")
        raw = regular_bytes(path, 128 * 1024 * 1024)
        require(digest(raw) == origin["archive_digest"], "source ZIP digest mismatch")
        archive = zipfile.ZipFile(io.BytesIO(raw))
        infos = archive.infolist()
        require(len(infos) <= 2048, "too many ZIP members")
        names = set()
        for info in infos:
            name = info.filename.rstrip("/")
            pure = PurePosixPath(name)
            require(name and not pure.is_absolute() and str(pure) == name and ".." not in pure.parts and "\\" not in name, "unsafe ZIP member name")
            require(name not in names, "duplicate ZIP member")
            names.add(name)
            mode = (info.external_attr >> 16) & 0o170000
            require(mode in (0, stat.S_IFREG, stat.S_IFDIR), "special ZIP member")
        def read(name):
            info = archive.getinfo(name)
            require(not info.is_dir() and 0 < info.file_size <= MAX_MEMBER, "bounded ZIP fact required")
            return archive.read(info)
        close = archive.close
    try:
        state = decode(read("review-state.json"))
        require(state.get("review_result") in ("PASS", "FAIL"), "terminal review required")
        selected = COMMON + (CLOSED if state["review_result"] == "PASS" else REJECTED)
        return {name: read(name) for name in sorted(selected)}
    finally:
        close()


def binding(members):
    docs = {}
    for name, data in members.items():
        require(name in COMMON + CLOSED + REJECTED and 0 < len(data) <= MAX_MEMBER, "unexpected or oversized fact")
        docs[name] = decode(data)
        scan(docs[name])
    r = docs["review-state.json"]
    passed = r.get("review_result") == "PASS"
    require(set(members) == set(COMMON + (CLOSED if passed else REJECTED)), "incomplete or extra fact set")
    require(r.get("version") in (1, 2) and r.get("pilot") in ("feature", "debug"), "unsupported retained subject")
    require(r.get("review_result") in ("PASS", "FAIL") and r.get("closure") == ("CLOSED" if passed else "BLOCKED"), "review/closure contradiction")
    require(isinstance(r.get("engineering_actor"), str) and r["engineering_actor"] and isinstance(r.get("reviewer_actor"), str) and r["reviewer_actor"] and r["engineering_actor"] != r["reviewer_actor"], "independent reviewer required")
    for key in ("engineering_run_id", "verification_run_id", "decision_comment_id"):
        require(type(r.get(key)) is int and r[key] > 0, "invalid review lineage")
    for key in ("base_commit", "result_commit"):
        require(isinstance(r.get(key), str) and COMMIT.fullmatch(r[key]), "exact commits required")
    v = docs["verification/verification-state.json"]
    e = docs["verification/engineering/engineering-state.json"]
    d = docs["verification/delivery-response.json"]
    vr = docs["verification/verification-response.json"]
    rr = docs["review-response.json"]
    require(v.get("version") == e.get("version") == r["version"], "retained state version mismatch")
    for key in ("pilot", "base_commit", "result_commit"):
        require(r[key] == v.get(key) == e.get(key), "cross-stage subject drift")
    require(v.get("engineering_run_id") == e.get("github_engineering_run_id") == r["engineering_run_id"], "engineering run drift")
    require(v.get("verification") == vr.get("result") == "PASS" and rr.get("result") == r["review_result"], "verification/review result drift")
    if r["version"] == 2:
        require(isinstance(r.get("provider"), dict) and r["provider"] == v.get("provider") == e.get("provider"), "historical provider binding drift")
    subject = d.get("subject_digest")
    task = d.get("task_contract_digest")
    require(isinstance(subject, str) and DIGEST.fullmatch(subject) and isinstance(task, str) and DIGEST.fullmatch(task), "subject and Task identities required")
    require(subject == vr.get("subject_digest") == rr.get("subject_digest") and task == rr.get("task_contract_digest"), "assurance subject drift")
    require(d.get("base_commit") == r["base_commit"] and d.get("result_commit") == r["result_commit"] and d.get("run_id") == e.get("run_id"), "delivery source drift")
    delivery = d.get("delivery_receipt_id")
    require(delivery and delivery == v.get("delivery_receipt_id") == vr.get("delivery_receipt_id") == rr.get("delivery_receipt_id"), "delivery identity drift")
    require(vr.get("verification_report_id") == v.get("verification_report_id") == rr.get("verification_report_id"), "verification identity drift")
    evidence_ids = []
    for name in ("ci", "codex", "git"):
        evidence = docs["verification/" + name + "-evidence.json"]
        require(evidence.get("subject_digest") == subject and evidence.get("delivery_receipt_id") == delivery and evidence.get("result") == "PASS" and evidence.get("applicable") is True, "Evidence binding drift")
        evidence_ids.append(evidence.get("evidence_id"))
    require(sorted(evidence_ids) == sorted(vr.get("evidence_ids", [])) and len(set(evidence_ids)) == 3, "Evidence set drift")
    git = docs["verification/git-change-manifest.json"]
    require(all(git.get(key) == r[key] for key in ("base_commit", "result_commit")), "Git subject drift")
    publication = docs["verification/engineering/publication-receipt.json"]
    observed = decode(publication["observed_state"].encode())
    scan(observed)
    require(publication.get("result") == "CONFIRMED" and observed.get("repository") == REPOSITORY and all(observed.get(key) == r[key] for key in ("base_commit", "result_commit")), "publication binding drift")
    require(publication.get("external_ref") == v.get("pull_request_url") == observed.get("pull_request_url"), "publication ref drift")
    require(r.get("decision_comment_url") == v["pull_request_url"] + "#issuecomment-" + str(r["decision_comment_id"]), "review comment binding drift")
    cd = docs["verification/codex-receipt-digest.json"]
    artifacts = {a["artifact_id"]: a["digest"] for a in d.get("artifacts", [])}
    require(cd.get("run_id") == e.get("run_id") and cd.get("bundle_digest") == e.get("bundle_digest") == artifacts.get("codex-result-bundle") and cd.get("artifact_digest") == artifacts.get("codex-execution-receipt"), "Codex artifact binding drift")
    work = docs["work-closed.json" if passed else "work-after-fail-review.json"]
    require(work.get("work_item_id") == d.get("work_item_id") and work.get("active_task_contract_digest") == task and work.get("active_run_id") == d.get("run_id"), "Work identity drift")
    require((work.get("state") == "CLOSED") == passed, "Work closure drift")
    if passed:
        closure = docs["closure-response.json"]
        require(closure.get("result") == "CLOSED" and closure.get("subject_digest") == subject and closure.get("task_contract_digest") == task, "closure subject drift")
        for key in ("work_item_id", "delivery_receipt_id", "run_id"):
            require(closure.get(key) == d.get(key), "closure identity drift")
        require(closure.get("verification_report_id") == vr.get("verification_report_id") and closure.get("review_report_id") == rr.get("review_report_id") and closure.get("closure_receipt_id"), "closure assurance drift")
    return {"pilot": r["pilot"], "review_result": r["review_result"], "closure": r["closure"], "result_commit": r["result_commit"], "base_commit": r["base_commit"], "subject_digest": subject, "task_contract_digest": task, "core_run_id": d["run_id"], "engineering_run_id": r["engineering_run_id"], "verification_run_id": r["verification_run_id"]}


def build(path, origin):
    members = source_members(path, origin)
    bound = binding(members)
    return canonical({"version": 1, "repository": REPOSITORY, "scope": SCOPE, "source": origin, "binding": bound, "members": [{"path": name, "digest": digest(data), "size": len(data), "utf8": data.decode("utf-8")} for name, data in sorted(members.items())]})


def payload(data):
    if data.startswith(b"\x1f\x8b"):
        try:
            with gzip.GzipFile(fileobj=io.BytesIO(data)) as source:
                data = source.read(MAX_ARCHIVE + 1)
        except (OSError, EOFError) as error:
            raise ValueError("invalid compressed archive") from error
    require(0 < len(data) <= MAX_ARCHIVE, "oversized archive payload")
    return data


def verify(data, expected_digest):
    require(isinstance(expected_digest, str) and DIGEST.fullmatch(expected_digest) and digest(data) == expected_digest, "archive anchor mismatch")
    require(0 < len(data) <= MAX_ARCHIVE, "oversized archive")
    decoded = payload(data)
    envelope = decode(decoded)
    require(canonical(envelope) == decoded, "non-canonical archive")
    require(set(envelope) == {"version", "repository", "scope", "source", "binding", "members"} and envelope["version"] == 1 and envelope["repository"] == REPOSITORY and envelope["scope"] == SCOPE, "unknown archive contract")
    origin = envelope["source"]
    require(type(origin.get("review_run_id")) is int and origin["review_run_id"] > 0, "invalid source run")
    if origin.get("kind") == "github-actions-zip":
        require(set(origin) == {"kind", "review_run_id", "artifact_id", "archive_digest"} and type(origin["artifact_id"]) is int and origin["artifact_id"] > 0 and DIGEST.fullmatch(origin["archive_digest"]), "invalid source ZIP identity")
    else:
        require(origin.get("kind") == "workflow-directory" and set(origin) == {"kind", "review_run_id"}, "invalid source identity")
    members = {}
    require(isinstance(envelope["members"], list) and len(envelope["members"]) <= len(COMMON) + len(CLOSED), "invalid member set")
    for entry in envelope["members"]:
        require(set(entry) == {"path", "digest", "size", "utf8"} and isinstance(entry["utf8"], str), "invalid member contract")
        name = entry["path"]
        require(name in COMMON + CLOSED + REJECTED and name not in members, "extra or duplicate fact")
        raw = entry["utf8"].encode("utf-8")
        require(type(entry["size"]) is int and entry["size"] == len(raw) and entry["digest"] == digest(raw), "member bytes mismatch")
        members[name] = raw
    require(list(members) == sorted(members), "non-canonical member ordering")
    require(binding(members) == envelope["binding"], "archive subject binding drift")
    return {"status": "ARCHIVE_BYTES_AND_BINDINGS_VERIFIED", "archive_digest": expected_digest, "member_count": len(members), "source": origin, "binding": envelope["binding"], "production_qualified": False, "runtime_restore_supported": False}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest="command", required=True)
    pack = sub.add_parser("pack")
    pack.add_argument("--source", required=True)
    pack.add_argument("--review-run-id", type=int, required=True)
    pack.add_argument("--artifact-id", type=int)
    pack.add_argument("--source-digest")
    pack.add_argument("--out", required=True)
    check = sub.add_parser("verify")
    check.add_argument("--archive", required=True)
    check.add_argument("--expected-digest", required=True)
    args = parser.parse_args()
    try:
        if args.command == "pack":
            origin = {"kind": "workflow-directory", "review_run_id": args.review_run_id}
            if args.artifact_id is not None or args.source_digest is not None:
                require(args.artifact_id is not None and args.source_digest is not None, "source artifact and digest must be supplied together")
                origin.update(kind="github-actions-zip", artifact_id=args.artifact_id, archive_digest=args.source_digest)
            raw = build(args.source, origin)
            if args.out.endswith(".gz"):
                raw = gzip.compress(raw, mtime=0)
            report = verify(raw, digest(raw))
            out = Path(args.out).absolute()
            require(out.parent == out.parent.resolve(), "aliased output parent")
            fd = os.open(out, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
            with os.fdopen(fd, "wb") as stream:
                stream.write(raw)
                stream.flush()
                os.fsync(stream.fileno())
            require(regular_bytes(out, MAX_ARCHIVE) == raw, "archive readback mismatch")
        else:
            report = verify(regular_bytes(Path(args.archive).absolute(), MAX_ARCHIVE), args.expected_digest)
        print(canonical(report).decode(), end="")
    except (ValueError, KeyError, TypeError, OSError, zipfile.BadZipFile) as error:
        print("retained archive rejected: " + str(error), file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
