package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/jiying2007/engineering-platform/internal/access"
	"github.com/jiying2007/engineering-platform/internal/recovery"
	"github.com/jiying2007/engineering-platform/internal/store"
)

type plannedActionReconciliationStore interface {
	ReconcilePlannedActionAbandoned(context.Context, string, recovery.ActionPlannedAbandonRequest) (recovery.ActionPlannedAbandonReceipt, error)
	GetPlannedActionAbandonReceipt(context.Context, string) (recovery.ActionPlannedAbandonReceipt, error)
}

func (s *Server) handleAbandonPlannedAction(w http.ResponseWriter, r *http.Request) {
	id, ok := AuthenticatedIdentity(r.Context())
	if !ok || !id.Allows(access.RecoveryReconcile) {
		writeError(w, http.StatusForbidden, "Recovery reconciliation denied")
		return
	}
	var request recovery.ActionPlannedAbandonRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	if err := request.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, "invalid exact planned Action reconciliation request")
		return
	}
	target, ok := s.store.(plannedActionReconciliationStore)
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "durable planned Action reconciliation unavailable")
		return
	}
	receipt, err := target.ReconcilePlannedActionAbandoned(r.Context(), id.Subject(), request)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrNotFound):
			writeError(w, http.StatusNotFound, "Action reservation not found")
		case errors.Is(err, store.ErrConflict), errors.Is(err, recovery.ErrStaleEpoch), errors.Is(err, recovery.ErrReconciliationRequired):
			writeError(w, http.StatusConflict, "Action is not eligible for planned-only reconciliation")
		default:
			writeError(w, http.StatusServiceUnavailable, "planned Action reconciliation unavailable or ambiguous")
		}
		return
	}
	writeJSON(w, http.StatusCreated, receipt)
}

func (s *Server) handleGetAbandonedPlannedAction(w http.ResponseWriter, r *http.Request) {
	id, ok := AuthenticatedIdentity(r.Context())
	if !ok || !id.Allows(access.Read) {
		writeError(w, http.StatusForbidden, "read permission required")
		return
	}
	target, ok := s.store.(plannedActionReconciliationStore)
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "durable Action readback unavailable")
		return
	}
	receipt, err := target.GetPlannedActionAbandonReceipt(r.Context(), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "abandoned planned Action not found")
		} else {
			writeError(w, http.StatusServiceUnavailable, "Action readback failed")
		}
		return
	}
	writeJSON(w, http.StatusOK, receipt)
}
