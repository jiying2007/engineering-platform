package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/jiying2007/engineering-platform/internal/access"
	"github.com/jiying2007/engineering-platform/internal/action"
	"github.com/jiying2007/engineering-platform/internal/githubpublish"
	"github.com/jiying2007/engineering-platform/internal/recovery"
	"github.com/jiying2007/engineering-platform/internal/store"
)

type publicationRecoveryStore interface {
	GetContext(context.Context, string) (action.Operation, error)
	GetRecoveryContext(context.Context) (recovery.Manager, error)
	ConfirmRecoveredPublication(context.Context, string, githubpublish.PublicationRecoveryRequest, githubpublish.PublicationEvidence) (githubpublish.PublicationRecoveryReceipt, error)
	GetRecoveredPublication(context.Context, string) (githubpublish.PublicationRecoveryReceipt, error)
}

func (s *Server) handleConfirmRecoveredPublication(w http.ResponseWriter, r *http.Request) {
	id, ok := AuthenticatedIdentity(r.Context())
	if !ok || !id.Allows(access.RecoveryReconcile) {
		writePublicError(w, http.StatusForbidden)
		return
	}
	var request githubpublish.PublicationRecoveryRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	if request.Validate() != nil {
		writePublicError(w, http.StatusBadRequest)
		return
	}
	target, ok := s.store.(publicationRecoveryStore)
	if !ok {
		writePublicError(w, http.StatusServiceUnavailable)
		return
	}
	// Check exact saved receipt first. This is readback, never another Publisher
	// invocation; it continues to work after Recovery completes or Publisher fails.
	prior, err := target.GetRecoveredPublication(r.Context(), request.Identity.OperationID)
	if err == nil {
		digest, _ := request.Digest()
		if prior.Validate() != nil || prior.RequestDigest != digest || prior.Reconciler != id.Subject() {
			writePublicError(w, http.StatusConflict)
			return
		}
		writeJSON(w, http.StatusOK, prior)
		return
	}
	if !errors.Is(err, store.ErrNotFound) {
		writePublicationRecoveryError(w, err)
		return
	}
	if s.publicationRecovery == nil {
		writePublicError(w, http.StatusServiceUnavailable)
		return
	}
	original, err := target.GetContext(r.Context(), request.Identity.OperationID)
	if err != nil {
		writePublicationRecoveryError(w, err)
		return
	}
	identity, err := githubpublish.IdentityForOperation(original)
	if err != nil || identity != request.Identity || !githubpublish.PositiveRecoveryState(original.State) {
		writePublicError(w, http.StatusConflict)
		return
	}
	state, err := target.GetRecoveryContext(r.Context())
	if err != nil {
		writePublicationRecoveryError(w, err)
		return
	}
	if state.Epoch != request.RecoveryEpoch || state.Mode != recovery.RecoveryReconciliation {
		writePublicError(w, http.StatusConflict)
		return
	}
	evidence, err := s.publicationRecovery.ObserveOriginalPublication(r.Context(), original)
	if err != nil {
		writePublicationRecoveryError(w, err)
		return
	}
	if evidence.ValidateForRecovery() != nil || evidence.Start.Identity != identity {
		writePublicError(w, http.StatusConflict)
		return
	}
	if err := r.Context().Err(); err != nil {
		writePublicationRecoveryError(w, err)
		return
	}
	// The database rechecks the current Recovery epoch and original locked row.
	// No network calls run inside its serializable transaction.
	receipt, err := target.ConfirmRecoveredPublication(r.Context(), id.Subject(), request, evidence)
	if err != nil {
		writePublicationRecoveryError(w, err)
		return
	}
	digest, _ := request.Digest()
	if receipt.Validate() != nil || receipt.RequestDigest != digest || receipt.Reconciler != id.Subject() {
		writePublicError(w, http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, http.StatusCreated, receipt)
}
func (s *Server) handleGetRecoveredPublication(w http.ResponseWriter, r *http.Request) {
	id, ok := AuthenticatedIdentity(r.Context())
	if !ok || !id.Allows(access.Read) {
		writePublicError(w, http.StatusForbidden)
		return
	}
	target, ok := s.store.(publicationRecoveryStore)
	if !ok {
		writePublicError(w, http.StatusServiceUnavailable)
		return
	}
	receipt, err := target.GetRecoveredPublication(r.Context(), r.PathValue("id"))
	if err != nil {
		writePublicationRecoveryError(w, err)
		return
	}
	if receipt.Validate() != nil || receipt.Request.Identity.OperationID != r.PathValue("id") {
		writePublicError(w, http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, http.StatusOK, receipt)
}
func writePublicationRecoveryError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrCommitOutcomeUnknown):
		writeCommitOutcomeUnknown(w)
	case errors.Is(err, store.ErrNotFound), errors.Is(err, action.ErrOperationAbsent):
		writePublicError(w, http.StatusNotFound)
	case errors.Is(err, store.ErrConflict), errors.Is(err, recovery.ErrStaleEpoch), errors.Is(err, recovery.ErrReconciliationRequired), errors.Is(err, githubpublish.ErrDispatchEvidenceUnavailable):
		writePublicError(w, http.StatusConflict)
	default:
		writePublicError(w, http.StatusServiceUnavailable)
	}
}
