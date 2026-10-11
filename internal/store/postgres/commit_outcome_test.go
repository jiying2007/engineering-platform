package postgres

import (
	"context"
	"errors"
	"testing"

	corestore "github.com/jiying2007/engineering-platform/internal/store"
)

func TestCommitOutcomeClassificationNeverInfersRollback(t *testing.T) {
	if err := classifyCommitError(nil); err != nil {
		t.Fatalf("successful COMMIT was reclassified: %v", err)
	}
	for _, original := range []error{context.Canceled, context.DeadlineExceeded, errors.New("connection lost during COMMIT")} {
		classified := classifyCommitError(original)
		if !errors.Is(classified, corestore.ErrCommitOutcomeUnknown) || !errors.Is(classified, original) {
			t.Fatalf("COMMIT failure lost unknown-outcome classification: %v", classified)
		}
	}
}
