package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/jiying2007/engineering-platform/internal/access"
	"github.com/jiying2007/engineering-platform/internal/recovery"
	"github.com/jiying2007/engineering-platform/internal/store"
)

type executionReconciliationStore interface {
	ReconcileExecutionAbandoned(context.Context, string, recovery.ExecutionAbandonRequest) (recovery.ExecutionAbandonReceipt, error)
}

func (s *Server) handleExecutionAbandon(w http.ResponseWriter, r *http.Request) {
	id, ok := AuthenticatedIdentity(r.Context())
	if !ok || !id.Allows(access.RecoveryReconcile) {
		writeError(w, http.StatusForbidden, "recovery reconciliation denied")
		return
	}
	var request recovery.ExecutionAbandonRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	if err := request.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	reconciler, ok := s.store.(executionReconciliationStore)
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "execution reconciliation store unavailable")
		return
	}
	receipt, err := reconciler.ReconcileExecutionAbandoned(r.Context(), id.Subject(), request)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrNotFound):
			writeError(w, http.StatusNotFound, "execution not found")
		case errors.Is(err, store.ErrConflict), errors.Is(err, recovery.ErrStaleEpoch), errors.Is(err, recovery.ErrReconciliationRequired):
			writeError(w, http.StatusConflict, "execution cannot be reconciled")
		default:
			writeError(w, http.StatusServiceUnavailable, "execution reconciliation failed")
		}
		return
	}
	writeJSON(w, http.StatusCreated, receipt)
}
