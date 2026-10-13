package githubpublish

import (
	"context"
	"errors"
	"time"

	"github.com/jiying2007/engineering-platform/internal/canonical"
)

// prepareInvocation is called by the original service before its one Remote.Publish.
// A durable intent does not mean that Publish was entered. Only this same lease,
// with in-memory provenance of its successful intent write, can retain that fact.
// Success irrevocably marks entry: an error/panic/timeout from Publish is NEVER
// evidence of non-invocation, and no later observer may reconstruct this marker.
func (l *dispatchLease) prepareInvocation(ctx context.Context, start DispatchStart) error {
	if l == nil || l.entered {
		return ErrDispatchEvidenceUnavailable
	}
	err := l.arm(ctx, start)
	if err == nil {
		err = ctx.Err()
	}
	if err == nil {
		l.entered = true
		return nil
	}
	if l.invocation == nil {
		return err
	} // No fully synced original intent; remain blocked.
	settle, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	return errors.Join(err, l.retainArmedCancellation(settle, start))
}

func (l *dispatchLease) retainArmedCancellation(ctx context.Context, start DispatchStart) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if l == nil || l.Root == nil || l.lock == nil || l.invocation == nil || l.entered || start.Validate() != nil {
		return ErrDispatchEvidenceUnavailable
	}
	incarnation, err := witnessIncarnation(l.Root)
	if err != nil || incarnation != start.Incarnation {
		return ErrDispatchEvidenceUnavailable
	}
	var original DispatchStart
	var invoked DispatchInvocation
	if readWitness(l.Root, "started.json", &original) != nil || original != start ||
		readWitness(l.Root, "invoked.json", &invoked) != nil || invoked != *l.invocation || invoked.Validate(start) != nil ||
		!witnessAbsent(l.Root, "completed.json") || !witnessAbsent(l.Root, "not-invoked.json") {
		return ErrDispatchEvidenceUnavailable
	}
	startDigest, _ := canonical.Digest(start)
	invocationDigest, _ := canonical.Digest(invoked)
	seal := DispatchNonInvocation{Kind: "PUBLISH_NOT_INVOKED_AFTER_ARM_V2", StartDigest: startDigest,
		InvocationDigest: invocationDigest, SealedAt: time.Now().UTC()}
	evidence := NonInvocationEvidence{Version: 2, Start: start, Seal: seal, Invocation: &invoked}
	if evidence.Validate() != nil {
		return ErrDispatchEvidenceUnavailable
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// EXCL append-only. Partial write/fsync failure leaves the occupied slot intact.
	// This independent budget is cooperative; it cannot interrupt a blocked fsync.
	if err := writeWitness(l.Root, "not-invoked.json", seal); err != nil {
		return err
	}
	return ctx.Err()
}
