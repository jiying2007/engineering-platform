package postgres

import "context"

// authorityContext supplies a finite *whole-operation* fallback only when the
// caller supplied no deadline. Statement/lock limits begin after a connection
// is acquired: they cannot bound a saturated pool or the sum of SQL phases.
// Reuse the existing 20-second statement safety ceiling as the fallback, not
// as a calibrated production SLO. A caller's explicit budget (including a
// longer, deliberately bounded maintenance operation) is retained exactly.
// Nested Store calls inherit the first deadline rather than resetting it.
//
// This is a child of the caller, never an independent maintenance context.
// Existing 5-second settlement/rollback and 30-second Outbox budgets remain
// distinct. Expiry after an attempted COMMIT still uses unknown-outcome
// classification; cancellation never implies rollback or replay authority.
func authorityContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if _, bounded := ctx.Deadline(); bounded {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, maxStatementTimeout)
}
