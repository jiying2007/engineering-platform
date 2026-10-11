package api

import (
	"errors"
	"net/http"

	"github.com/jiying2007/engineering-platform/internal/store"
)

// COMMIT was attempted; a lost acknowledgement is not rollback evidence.
// Never leak driver/SQL diagnostics or imply permission to replay a model,
// device command, Git/PR publication, or any other external effect.
func writeCommitOutcomeUnknown(w http.ResponseWriter) {
	writeError(w, http.StatusServiceUnavailable, "commit_outcome_unknown: read back the exact resource ID, epoch, idempotency key and audit record before considering any retry")
}

func writeCommitAwareInternalError(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrCommitOutcomeUnknown) {
		writeCommitOutcomeUnknown(w)
		return
	}
	writeError(w, http.StatusInternalServerError, "internal Core authority operation failed")
}
