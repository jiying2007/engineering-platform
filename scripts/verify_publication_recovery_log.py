#!/usr/bin/env python3
"""Reject skipped/missing targeted PostgreSQL recovery tests in canonical CI."""
import json
import sys
from collections import Counter
from pathlib import Path

PACKAGE = "github.com/jiying2007/engineering-platform/internal/store/postgres"
REQUIRED = {
    "TestPublicationRecoveryDatabaseTerminalReadbackAndNoReplay",
    "TestPublicationRecoveryDatabaseRejectsInsufficientEvidence",
    "TestPublicationRecoveryDatabaseCorruptionBlocksProofAndReadback",
    "TestPublicationRecoveryDatabaseWaitCancellationIsAtomic",
    "TestPublicationRecoveryDatabaseAuditFailureRollsBack",
    "TestPublicationRecoveryDatabaseConcurrentSettlementIsSingle",
    "TestPublicationRecoveryDatabaseRealCommitAckLoss",
    "TestPublicationRecoveryDatabaseCompletionRechecksEvidence",
    "TestPublicationRecoveryDatabaseMTLSIndependentCompletion",
}

RESTORE_REQUIRED = {"TestPostgresAuthorityRestoreDrill", "TestPublicationRecoveryRestoreDrill"}

def verify(path: Path, repeats: int = 3, required=REQUIRED) -> None:
    if repeats < 1 or repeats > 20:
        raise ValueError("invalid repetition count")
    passed = Counter()
    package_passed = False
    for line in path.read_text(encoding="utf-8").splitlines():
        event = json.loads(line)
        if not isinstance(event, dict):
            raise ValueError("invalid test event")
        if event.get("Action") in ("skip", "fail"):
            raise ValueError("database test was skipped or failed")
        if event.get("Action") == "pass":
            name = event.get("Test")
            if name in required:
                if event.get("Package") != PACKAGE:
                    raise ValueError("targeted test belongs to a different package")
                passed[name] += 1
            elif not name and event.get("Package") == PACKAGE:
                package_passed = True
    if not package_passed or any(passed[name] != repeats for name in required):
        raise ValueError("missing exact targeted database success events")

if __name__ == "__main__":
    if len(sys.argv) not in (2, 3) or (len(sys.argv)==3 and sys.argv[2]!="--restore"):
        raise SystemExit("usage: verify_publication_recovery_log.py test-events.jsonl [--restore]")
    try:
        if len(sys.argv)==3:
            verify(Path(sys.argv[1]), repeats=1, required=RESTORE_REQUIRED)
        else:
            verify(Path(sys.argv[1]))
    except (OSError, ValueError) as exc:
        raise SystemExit(f"database recovery gate rejected: {exc}")
    print("Publication recovery PostgreSQL tests: explicit required repetitions, zero skips")
