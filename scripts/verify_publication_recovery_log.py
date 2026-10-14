#!/usr/bin/env python3
"""Reject skipped/missing targeted publication recovery and kernel tests."""
import json
import math
import os
import stat
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
    "TestPublisherWitnessIntegrityContextPreCancelled",
    "TestPublisherWitnessIntegrityContextWriteMarshallingCancellation",
    "TestPublisherWitnessIntegrityContextReadDecodeCancellation",
    "TestPublisherWitnessIntegrityContextReadCheckpoints",
    "TestPublisherWitnessIntegrityContextReadNoDescriptorLeak",
    "TestPublisherWitnessIntegrityContextOriginalCompletionSettlement",
}

RESTORE_REQUIRED = {"TestPostgresAuthorityRestoreDrill", "TestPublicationRecoveryRestoreDrill", "TestNoInvocationRestoreDrill"}

PUBLISHER_PACKAGE = "github.com/jiying2007/engineering-platform/internal/githubpublish"
PUBLISHER_REQUIRED = {
    "TestPublisherRemoteResponseRejectsIncompleteBody",
    "TestPublisherRemoteResponseAtomicDecode",
    "TestPublisherRemoteResponseCancelledBeforeExchange",
    "TestPublisherRemoteHealthCompleteObservation",
    "TestPublisherRemoteResponseTransportFailure",
    "TestPublisherRemoteResponsePublicErrorsHaveNoReceipt",
    "TestPublisherRemoteResponseRealMTLS",
    "TestPublisherGitHubResponseIncomplete404",
    "TestPublisherGitHubResponseCompleteStatus",
    "TestPublisherGitHubResponseAtomicDecode",
    "TestPublisherGitHubResponseCancelledBeforeExchange",
    "TestPublisherGitHubResponseTruncated404Observation",
    "TestPublisherGitHubResponseTruncated404Publication",
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
 "TestPublisherNoInvocationAfterArmPreservesLegacyBytes",
 "TestPublisherNoInvocationAfterArmCancellation",
 "TestPublisherNoInvocationAfterArmCannotCertifyEnteredCall",
 "TestPublisherNoInvocationAfterArmRequiresOriginalWriter",
 "TestPublisherNoInvocationAfterArmRejectsCorruptEvidence",
 "TestPublisherNoInvocationAfterArmEvidenceRealMTLS",
 "TestPublisherNoInvocationAfterArmFailedRetentionStaysBlocked",

 "TestPublisherNoInvocationFenceRejectsActiveAndReplays",
 "TestPublisherNoInvocationFenceRejectsHistoricalAndArmed",
 "TestPublisherInvocationBarrierPrecedesRemote",
 "TestPublisherUninvokedLeaseRejectsCancelledArming",
 "TestPublisherNoInvocationFenceAfterRealProcessExit",
 "TestPublisherNoInvocationRejectsRestoredStartSnapshot",
 "TestPublisherNoInvocationRejectsOtherBootIdentity",
 "TestPublisherNoInvocationEvidenceRealMTLS",
}

CONTROL_PACKAGE = "github.com/jiying2007/engineering-platform/internal/controlclient"
CONTROL_REQUIRED = {
    "TestControlResponseLifecycleOnlyCompleteConflictIsIgnorable",
    "TestBroken503RetainsHTTPStatusUncertaintyAndReadCause",
    "TestControlResponseLifecycleCancelledBeforeExchange",
    "TestControlResponseLifecycleCancelledDecode",
    "TestControlResponseLifecycleCompleteStatuses",
    "TestControlResponseLifecycleIncompleteBody",
    "TestControlResponseLifecycleReadBudgets",
    "TestControlResponseLifecycleRealMTLSTruncatedError",
    "TestExternalEffectUnresolvedPreservesExactHTTPStatusAndNoReplay",
    "TestPostCancelledBeforeSendPreservesContextCauseWithoutReplay",
    "TestPostTransportCutAfterHandlerNeverClaimsRollbackOrRetries",
    "TestRawNullRejectedEvenWithoutTypedDestination",
    "TestRealMTLSAndNoAmbientProxy",
    "TestRedirectsErrorsAndOversizeResponses",
    "TestResponseClaimDistinguishesMissingAndExplicitNull",
    "TestResponseDestinationRejectedBeforePOST",
    "TestResponseObjectContractPreservesOutputAndNeverRetries",
    "TestTLSConstructionDoesNotInheritOverridesOrDERAliases",
    "TestTLSKeyFileBounds",
    "TestUnknownCommitTransportErrorRequiresReadbackWithoutSQLLeakOrReplay",
    "TestUnsafeClientConfigurationAndPathsFailClosed",
    "TestUnusablePostSuccessCannotBeTreatedAsCompletedOrRetried",
}

# Evidence is read as a bounded event stream, not a list of PASS summaries.
# This validates structure/completeness only. CI/run/source/Artifact provenance
# remains an independent requirement; a fabricated complete stream is not proof.
MAX_LOG_BYTES = 64 << 20
MAX_LINE_BYTES = 1 << 20
MAX_EVENTS = 250_000
MAX_TESTS = 10_000
MAX_NAME_BYTES = 4096


def _unique_object(pairs):
    result = {}
    for key, value in pairs:
        if key in result:
            raise ValueError("duplicate test event field")
        result[key] = value
    return result


def _invalid_constant(_):
    raise ValueError("non-finite test event number")


def _finite_float(value):
    number = float(value)
    if not math.isfinite(number):
        raise ValueError("non-finite test event number")
    return number


def _events(path):
    flags = os.O_RDONLY | getattr(os, "O_CLOEXEC", 0)
    flags |= getattr(os, "O_NOFOLLOW", 0) | getattr(os, "O_NONBLOCK", 0)
    # A FIFO/device is not a finished evidence file. Never block waiting for it.
    with os.fdopen(os.open(path, flags), "rb") as stream:
        info = os.fstat(stream.fileno())
        if not stat.S_ISREG(info.st_mode) or not 0 < info.st_size <= MAX_LOG_BYTES:
            raise ValueError("bounded regular test log required")
        total = count = 0
        while True:
            raw = stream.readline(MAX_LINE_BYTES + 1)
            if not raw:
                break
            total += len(raw)
            count += 1
            if len(raw) > MAX_LINE_BYTES or total > MAX_LOG_BYTES or count > MAX_EVENTS:
                raise ValueError("test log resource budget exceeded")
            try:
                event = json.loads(raw.decode("utf-8"), object_pairs_hook=_unique_object,
                                   parse_constant=_invalid_constant, parse_float=_finite_float)
            except (UnicodeError, RecursionError) as exc:
                raise ValueError("invalid test event encoding or depth") from exc
            if not isinstance(event, dict):
                raise ValueError("invalid test event")
            yield event


def verify(path: Path, repeats: int = 3, required=REQUIRED, package=PACKAGE) -> None:
    if type(repeats) is not int or not 1 <= repeats <= 20:
        raise ValueError("invalid repetition count")
    if (not isinstance(required, (set, frozenset)) or not required or
            any(not isinstance(name, str) or not name or "/" in name for name in required) or
            not isinstance(package, str) or not package):
        raise ValueError("explicit package and top-level test set required")
    passed = Counter()
    active = {}
    children = Counter()
    seen = set()
    started = ended = False
    for event in _events(path):
        action = event.get("Action")
        name = event.get("Test")
        if (not isinstance(action, str) or event.get("Package") != package or
                ("Test" in event and (not isinstance(name, str) or not name or
                                     len(name.encode("utf-8")) > MAX_NAME_BYTES))):
            raise ValueError("invalid test event identity")
        if ended:
            raise ValueError("event after terminal package result")
        if action in ("skip", "fail", "build-fail"):
            raise ValueError("required test was skipped or failed")
        if action == "start":
            if started or name is not None:
                raise ValueError("duplicate or invalid package start")
            started = True
            continue
        if not started:
            raise ValueError("missing package start")
        if action == "output":
            if not isinstance(event.get("Output"), str):
                raise ValueError("invalid test output event")
            continue
        if action == "run":
            if name is None or name in active:
                raise ValueError("test started twice or without identity")
            root = name.split("/", 1)[0]
            # A slash in t.Run names does not imply an intermediate run event.
            # test2json exposes full names, not parent IDs; bind to the known
            # top-level invocation instead of inventing nested parent events.
            if "/" in name and root not in active:
                raise ValueError("subtest has no active top-level test")
            seen.add(name)
            if len(seen) > MAX_TESTS:
                raise ValueError("test identity budget exceeded")
            active[name] = "running"
            if "/" in name:
                children[root] += 1
        elif action in ("pause", "cont"):
            expected = "running" if action == "pause" else "paused"
            if name is None or active.get(name) != expected:
                raise ValueError("invalid parallel test transition")
            active[name] = "paused" if action == "pause" else "running"
        elif action == "pass":
            if name is None:
                if active:
                    raise ValueError("package passed with unfinished tests")
                ended = True
            else:
                if active.get(name) != "running":
                    raise ValueError("test passed without a running invocation")
                if "/" not in name and children[name]:
                    raise ValueError("test passed with unfinished subtests")
                del active[name]
                del children[name]
                if "/" in name:
                    children[name.split("/", 1)[0]] -= 1
                if name in required:
                    passed[name] += 1
                    if passed[name] > repeats:
                        raise ValueError("excess targeted test repetition")
        else:
            raise ValueError("unsupported test event action")
    if not ended or active or any(passed[name] != repeats for name in required):
        raise ValueError("missing complete targeted success events")

if __name__ == "__main__":
    if len(sys.argv) not in (2, 3) or (len(sys.argv)==3 and sys.argv[2] not in ("--restore", "--publisher-kernel", "--no-invocation-db", "--no-invocation-publisher", "--publisher-proof", "--control-response")):
        raise SystemExit("usage: verify_publication_recovery_log.py test-events.jsonl [--restore|--publisher-kernel|--no-invocation-db|--no-invocation-publisher|--publisher-proof|--control-response]")
    try:
        if len(sys.argv)==3 and sys.argv[2]=="--control-response":
            verify(Path(sys.argv[1]), repeats=20, required=CONTROL_REQUIRED, package=CONTROL_PACKAGE)
        elif len(sys.argv)==3 and sys.argv[2]=="--publisher-proof":
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
    print("Publication recovery required tests: complete bounded event stream, exact repetitions, zero skips")
