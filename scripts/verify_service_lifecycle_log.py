#!/usr/bin/env python3
"""Require complete repeated service-stop tests using the existing event parser.

This binds test identities/completeness, not production-host qualification.
Repository, run and artifact provenance remain independent requirements.
"""
import sys
from pathlib import Path

from verify_publication_recovery_log import verify

PREFIX = "github.com/jiying2007/engineering-platform/"
GROUPS = (
    ("cmd/control-plane", frozenset({
        "TestControlLifecycleMixedRelayFailure",
        "TestControlLifecycleRelayCleanupFailureAfterCancel",
        "TestControlLifecycleUnexpectedRelayCancellation",
        "TestControlLifecycleMixedServerFailure",
        "TestControlLifecycleWrappedCooperativeCancel",
        "TestControlLifecycleGracefulRequestDrain",
        "TestControlStopErrorClassification",
        "TestControlLifecycleServiceStopCancelsActualPostgresWait",
        "TestControlLifecycleRequestCancellationBeforeDrain",
    })),
    ("cmd/publisher-service", frozenset({
        "TestPublisherLifecycleRequestStopAndOriginalSettlement",
        "TestPublisherLifecyclePreservesMixedListenerFailure",
        "TestPublisherLifecyclePreCancelledStartsNothing",
        "TestPublisherLifecycleListenerFailureCancelsAndDrainsActiveRequest",
    })),
    ("internal/servicelifecycle", frozenset({
        "TestBindHTTPPreservesRootAndRejectsDetachedHooks",
    })),
)


def verify_logs(paths, repeats=3):
    if len(paths) != len(GROUPS):
        raise ValueError("all three service lifecycle event streams required")
    for path, (package, names) in zip(paths, GROUPS):
        verify(Path(path), repeats=repeats, required=names, package=PREFIX + package)


def main(argv=None):
    args = sys.argv[1:] if argv is None else argv
    if len(args) != 3:
        raise ValueError("usage: verify_service_lifecycle_log.py control.jsonl publisher.jsonl helper.jsonl")
    verify_logs(args)
    print("Service lifecycle: 14 required tests x3, complete event streams, zero skips")


if __name__ == "__main__":
    try:
        main()
    except (OSError, ValueError) as exc:
        raise SystemExit(f"service lifecycle gate rejected: {exc}")
