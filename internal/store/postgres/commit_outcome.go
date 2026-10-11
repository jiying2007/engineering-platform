package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	corestore "github.com/jiying2007/engineering-platform/internal/store"
)

// classifyCommitError is conservative: even a cancelled/disconnected caller
// cannot infer rollback after COMMIT was attempted. Only authoritative
// readback of the original immutable resource and audit can resolve it.
func classifyCommitError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: %w", corestore.ErrCommitOutcomeUnknown, err)
}

func commitAuthority(ctx context.Context, tx pgx.Tx) error {
	return classifyCommitError(tx.Commit(ctx))
}
