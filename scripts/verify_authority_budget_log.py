#!/usr/bin/env python3
"""Require the exact bounded PostgreSQL entry tests, including real pool wait."""
import sys
from pathlib import Path

from verify_publication_recovery_log import PACKAGE, verify

REQUIRED = {
    "TestPostgresAuthorityBudgetPreservesCaller",
    "TestPostgresAuthorityBudgetEveryPublicEntry",
    "TestPostgresAuthorityBudgetGuardRejectsDetachedOrUnreleasedTimers",
    "TestPostgresAuthorityBudgetNoDeadlinePoolWait",
    "TestPostgresAuthorityBudgetCoversWholeMutation",
    "TestPostgresAuthorityBudgetSQLContextProvenance",
}


def main(argv):
    if len(argv) != 1:
        raise ValueError("one complete PostgreSQL budget event log required")
    verify(Path(argv[0]), repeats=1, required=REQUIRED, package=PACKAGE)
    print("PostgreSQL authority entry budget: all six tests complete, zero skips")


if __name__ == "__main__":
    try:
        main(sys.argv[1:])
    except (OSError, ValueError) as exc:
        raise SystemExit(f"authority budget gate rejected: {exc}")
