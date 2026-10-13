#!/usr/bin/env python3
"""Reject skipped/missing targeted publication recovery and kernel tests."""
import json
import sys
from collections import Counter
from pathlib import Path

PACKAGE = "github.com/jiying2007/engineering-platform/internal/store/postgres"
TRANSACTION_REQUIRED = {
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

REQUIRED = TRANSACTION_REQUIRED | {
    "TestPublicationRecoveryDatabaseRequiresOriginalGitProof",
    "TestPublicationRecoveryDatabaseHistoricalReceiptReadback",
}
PROOF_REQUIRED = {
    "TestPublisherCompletionProofLegacyCannotBePromoted",
    "TestPublisherCompletionProofRejectsTampering",
    "TestPublisherCompletionProofMissingCollectorWithholdsWitness",
    "TestPublisherCompletionProofCollectorBounds",
    "TestPublisherCompletionProofActualKernel",
    "TestPublisherWitnessIntegrityRejectsAliasedAndSpecialFiles",
    "TestPublisherWitnessIntegrityRejectsUnsafeRootModes",
    "TestPublisherWitnessIntegrityDescriptorOpenDoesNotFollowOrBlock",
}

RESTORE_REQUIRED = {"TestPostgresAuthorityRestoreDrill", "TestPublicationRecoveryRestoreDrill", "TestNoInvocationRestoreDrill"}

PUBLISHER_PACKAGE = "github.com/jiying2007/engineering-platform/internal/githubpublish"
PUBLISHER_REQUIRED = {
    "TestPublisherGitNamespaceReapsFastCommands",
    "TestPublisherGitNamespaceKillsDetachedHelper",
    "TestPublisherGitNamespaceCancellationReapsTree",
    "TestPublisherGitNamespacePrecancelledStartsNothing",
    "TestPublisherGitUnconfirmedReapQuarantinesVerifiedSource",
    "TestPublisherGitOutputReapsDetachedDescendant",
    "TestPublisherGitCredentialNeverFollowsRemoteRedirect",
    "TestGitHubPublishNeverReopensClosedDeterministicBranch",
    "TestGitHubNullOpenPullReadbackNeverAuthorizesCreate",
}

NO_INVOCATION_REQUIRED = {name.replace("TestPublicationRecoveryDatabase", "TestNoInvocationDatabase") for name in TRANSACTION_REQUIRED}
NO_INVOCATION_PUBLISHER_REQUIRED = {
 "TestPublisherNoInvocationFenceRejectsActiveAndReplays",
 "TestPublisherNoInvocationFenceRejectsHistoricalAndArmed",
 "TestPublisherInvocationBarrierPrecedesRemote",
 "TestPublisherUninvokedLeaseRejectsCancelledArming",
 "TestPublisherNoInvocationFenceAfterRealProcessExit",
 "TestPublisherNoInvocationRejectsRestoredStartSnapshot",
 "TestPublisherNoInvocationRejectsOtherBootIdentity",
 "TestPublisherNoInvocationEvidenceRealMTLS",
}

def verify(path: Path, repeats: int = 3, required=REQUIRED, package=PACKAGE) -> None:
    if repeats < 1 or repeats > 20:
        raise ValueError("invalid repetition count")
    passed = Counter()
    package_passed = False
    for line in path.read_text(encoding="utf-8").splitlines():
        event = json.loads(line)
        if not isinstance(event, dict):
            raise ValueError("invalid test event")
        if event.get("Action") in ("skip", "fail"):
            raise ValueError("required test was skipped or failed")
        if event.get("Action") == "pass":
            name = event.get("Test")
            if name in required:
                if event.get("Package") != package:
                    raise ValueError("targeted test belongs to a different package")
                passed[name] += 1
            elif not name and event.get("Package") == package:
                package_passed = True
    if not package_passed or any(passed[name] != repeats for name in required):
        raise ValueError("missing exact targeted success events")

if __name__ == "__main__":
    if len(sys.argv) not in (2, 3) or (len(sys.argv)==3 and sys.argv[2] not in ("--restore", "--publisher-kernel", "--no-invocation-db", "--no-invocation-publisher", "--publisher-proof")):
        raise SystemExit("usage: verify_publication_recovery_log.py test-events.jsonl [--restore|--publisher-kernel|--no-invocation-db|--no-invocation-publisher|--publisher-proof]")
    try:
        if len(sys.argv)==3 and sys.argv[2]=="--publisher-proof":
            verify(Path(sys.argv[1]), required=PROOF_REQUIRED, package=PUBLISHER_PACKAGE)
        elif len(sys.argv)==3 and sys.argv[2]=="--no-invocation-db":
            verify(Path(sys.argv[1]), required=NO_INVOCATION_REQUIRED)
        elif len(sys.argv)==3 and sys.argv[2]=="--no-invocation-publisher":
            verify(Path(sys.argv[1]), required=NO_INVOCATION_PUBLISHER_REQUIRED, package=PUBLISHER_PACKAGE)
        elif len(sys.argv)==3 and sys.argv[2]=="--publisher-kernel":
            verify(Path(sys.argv[1]), required=PUBLISHER_REQUIRED, package=PUBLISHER_PACKAGE)
        elif len(sys.argv)==3:
            verify(Path(sys.argv[1]), repeats=1, required=RESTORE_REQUIRED)
        else:
            verify(Path(sys.argv[1]))
    except (OSError, ValueError) as exc:
        raise SystemExit(f"publication recovery gate rejected: {exc}")
    print("Publication recovery required tests: explicit required repetitions, zero skips")
