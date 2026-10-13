#!/usr/bin/env python3
"""Reject closing syntax for gates still declared unresolved in source readiness.

Read-only guard for PR descriptions, candidate commits and a proposed final
merge message. This never edits issues or changes readiness. CI cannot inspect
an arbitrary squash message chosen only AFTER checks have run; pass that exact
proposed text with --message-file immediately before the merge operation.
"""
from __future__ import annotations
import argparse
from bisect import bisect_right
import json
import math
import os
from pathlib import Path
import re
import stat

MAX_STATUS_BYTES = 1 << 20
MAX_EVENT_BYTES = 4 << 20
MAX_MESSAGE_BYTES = 256 << 10
MAX_MESSAGES = 2048
MAX_TOTAL_MESSAGE_BYTES = 4 << 20
MAX_FINDINGS = 4096
START = "<!-- engineering-platform-readiness\n"
END = "\n-->"
KEYWORD = re.compile(r"(?i)\b(?:close|closes|closed|fix|fixes|fixed|resolve|resolves|resolved)(?:\s*:\s*|\s+)")
REFERENCE = re.compile(r"(?:https://github\.com/(?P<urlrepo>[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+)/issues/(?P<urlid>[1-9][0-9]*)|(?:(?P<repo>[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+))?\#(?P<id>[1-9][0-9]*))(?![0-9A-Za-z_])",re.I)
REPOSITORY = re.compile(r"[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+\Z")
FIELDS = {"version","internal_readiness","internal_blockers","external_qualification_gates","deferred_credential_gates"}


def unique_object(pairs):
    obj = {}
    for key, value in pairs:
        if key in obj:
            raise ValueError("duplicate JSON field")
        obj[key] = value
    return obj


def strict_json(text):
    def invalid(_):
        raise ValueError("non-finite JSON value")
    def finite_float(value):
        number = float(value)
        if not math.isfinite(number):
            raise ValueError("non-finite JSON value")
        return number
    return json.loads(text, object_pairs_hook=unique_object, parse_constant=invalid,
                      parse_float=finite_float)


def read_bounded(path: Path, maximum: int) -> str:
    flags = os.O_RDONLY | getattr(os,"O_CLOEXEC",0) | getattr(os,"O_NOFOLLOW",0) | getattr(os,"O_NONBLOCK",0)
    with os.fdopen(os.open(path,flags),"rb") as stream:
        before = os.fstat(stream.fileno())
        if not stat.S_ISREG(before.st_mode) or not 0 < before.st_size <= maximum:
            raise ValueError("bounded nonempty regular input required")
        raw = stream.read(maximum+1)
        after = os.fstat(stream.fileno())
        if len(raw)>maximum or len(raw)!=before.st_size or before.st_size!=after.st_size or before.st_mtime_ns!=after.st_mtime_ns:
            raise ValueError("input changed or exceeded limit")
        return raw.decode("utf-8")


def unresolved_issues(status: str) -> set[int]:
    if not status.startswith("# Implementation Status\n") or len(status.encode())>MAX_STATUS_BYTES:
        raise ValueError("bounded Implementation Status required")
    if status.count("<!-- engineering-platform-readiness")!=1 or status.count(START)!=1:
        raise ValueError("one readiness authority required")
    body = status.split(START,1)[1]
    if END not in body:
        raise ValueError("unterminated readiness authority")
    raw = body.split(END,1)[0]
    if len(raw.encode())>8192:
        raise ValueError("readiness authority too large")
    data = strict_json(raw)
    if not isinstance(data,dict) or set(data)!=FIELDS or type(data["version"]) is not int or data["version"]!=1:
        raise ValueError("unsupported readiness authority")
    seen=set()
    for field in ("internal_blockers","external_qualification_gates","deferred_credential_gates"):
        values=data[field]
        if not isinstance(values,list) or any(type(n) is not int or n<=0 for n in values) or values!=sorted(set(values)) or seen.intersection(values):
            raise ValueError("invalid or duplicate readiness issue IDs")
        seen.update(values)
    if not data["external_qualification_gates"]:
        raise ValueError("explicit external qualification required")
    if not ((data["internal_readiness"]=="BLOCKED" and data["internal_blockers"]) or
            (data["internal_readiness"]=="CANDIDATE_ONLY" and not data["internal_blockers"])):
        raise ValueError("inconsistent readiness")
    return seen


def closing_references(text: str, repository: str) -> list[tuple[int,int]]:
    """Conservative syntax guard: negation and Markdown are NOT exemptions.

    Follow documented keyword/reference syntax, including a colon and case.
    Also reject slash/comma/and lists conservatively; GitHub need not interpret
    every extension for unsafe metadata to be refused by this repository.
    """
    if not isinstance(repository,str) or not REPOSITORY.fullmatch(repository) or not isinstance(text,str) or len(text.encode())>MAX_MESSAGE_BYTES:
        raise ValueError("bounded message and exact repository required")
    found=[]
    newlines=[m.start() for m in re.finditer("\n", text)]
    for keyword in KEYWORD.finditer(text):
        offset=keyword.end()
        # Formatting wrappers cannot be used to hide an unresolved gate in a
        # proposed message. Do not print the source text into CI logs.
        while offset<len(text) and text[offset] in "`*_[( ":
            offset+=1
        while True:
            match=REFERENCE.match(text,offset)
            if not match:
                break
            repo=match.group("urlrepo") or match.group("repo") or repository
            number=int(match.group("urlid") or match.group("id"))
            if repo.casefold()==repository.casefold():
                found.append((number,bisect_right(newlines,keyword.start())+1))
                if len(found)>MAX_FINDINGS:
                    raise ValueError("closing-reference budget exceeded")
            offset=match.end()
            # One unambiguous reference is enough for the documented syntax.
            # Lists are additionally refused as a conservative local policy.
            link = re.match(r"(?:\s*(?:,|/|and)\s*)",text[offset:],re.I)
            if not link:
                break
            offset+=link.end()
    return found


def event_messages(event: dict, repository: str) -> list[tuple[str,str]]:
    if not isinstance(event,dict):
        raise ValueError("object event required")
    rep=event.get("repository")
    if (not isinstance(rep,dict) or not isinstance(rep.get("full_name"),str) or
            not isinstance(repository,str) or not REPOSITORY.fullmatch(repository) or
            rep["full_name"].casefold()!=repository.casefold()):
        raise ValueError("event repository mismatch")
    messages=[]
    total=0
    def add(label,value,nullable=False):
        nonlocal total
        if nullable and value is None:
            value=""
        if not isinstance(value,str) or len(value.encode())>MAX_MESSAGE_BYTES:
            raise ValueError("invalid event metadata string")
        total+=len(value.encode())
        if total>MAX_TOTAL_MESSAGE_BYTES:
            raise ValueError("aggregate metadata budget exceeded")
        messages.append((label,value))
    if "pull_request" in event:
        pr=event["pull_request"]
        if not isinstance(pr,dict):
            raise ValueError("invalid pull request event")
        add("pull_request.title",pr.get("title"))
        add("pull_request.body",pr.get("body"),nullable=True)
    if "head_commit" in event and event["head_commit"] is not None:
        if not isinstance(event["head_commit"],dict):
            raise ValueError("invalid head commit")
        add("head_commit.message",event["head_commit"].get("message"))
    commits=event.get("commits",[])
    if not isinstance(commits,list) or len(commits)>MAX_MESSAGES:
        raise ValueError("invalid commit metadata set")
    for i,c in enumerate(commits):
        if not isinstance(c,dict):
            raise ValueError("invalid commit metadata")
        add(f"commits[{i}].message",c.get("message"))
    if not messages:
        raise ValueError("event contains no PR or commit metadata")
    return messages


def verify(status: str, repository: str, messages: list[tuple[str,str]]) -> dict:
    unresolved=unresolved_issues(status)
    if not isinstance(repository,str) or not REPOSITORY.fullmatch(repository) or not messages or len(messages)>MAX_MESSAGES+4:
        raise ValueError("explicit bounded metadata set required")
    rejected=[]
    total=0
    for label,text in messages:
        if not isinstance(label,str) or not re.fullmatch(r"[A-Za-z0-9_.\[\]-]{1,128}",label):
            raise ValueError("invalid metadata source label")
        if not isinstance(text,str):
            raise ValueError("invalid metadata source text")
        total+=len(text.encode())
        if total>MAX_TOTAL_MESSAGE_BYTES:
            raise ValueError("aggregate metadata budget exceeded")
        for issue,line in closing_references(text,repository):
            if issue in unresolved:
                rejected.append({"source":label,"line":line,"issue":issue})
                if len(rejected)>MAX_FINDINGS:
                    raise ValueError("closing-reference budget exceeded")
    if rejected:
        raise ValueError("unresolved-gate closing syntax: "+json.dumps(rejected,sort_keys=True))
    return {"version":1,"status":"DECLARED_BLOCKER_METADATA_CHECKED","repository":repository,
            "unresolved_issues":sorted(unresolved),"messages_checked":len(messages),
            "issue_changes_performed":False,"readiness_promoted":False,
            "arbitrary_later_merge_message_covered":False}


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--status",type=Path,required=True)
    parser.add_argument("--repository",required=True)
    parser.add_argument("--event",type=Path)
    parser.add_argument("--message-file",type=Path,action="append",default=[])
    args=parser.parse_args()
    try:
        if len(args.message_file)>MAX_MESSAGES:
            raise ValueError("too many metadata files")
        messages=[]
        if args.event:
            messages+=event_messages(strict_json(read_bounded(args.event,MAX_EVENT_BYTES)),args.repository)
        total=sum(len(text.encode()) for _,text in messages)
        for i,path in enumerate(args.message_file):
            text=read_bounded(path,MAX_MESSAGE_BYTES)
            total+=len(text.encode())
            if total>MAX_TOTAL_MESSAGE_BYTES:
                raise ValueError("aggregate metadata budget exceeded")
            messages.append((f"proposed_message[{i}]",text))
        report=verify(read_bounded(args.status,MAX_STATUS_BYTES),args.repository,messages)
    except (OSError,ValueError,UnicodeError,RecursionError,TypeError) as exc:
        raise SystemExit("blocker metadata rejected: "+str(exc))
    print(json.dumps(report,indent=2))

if __name__=="__main__":
    main()
