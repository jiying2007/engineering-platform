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

type nonInvocationRecoveryStore interface {
	GetContext(context.Context, string) (action.Operation, error)
	GetRecoveryContext(context.Context) (recovery.Manager, error)
	AbandonUninvokedPublication(context.Context, string, githubpublish.PublicationRecoveryRequest, githubpublish.NonInvocationEvidence) (githubpublish.NonInvocationRecoveryReceipt, error)
	GetUninvokedPublication(context.Context, string) (githubpublish.NonInvocationRecoveryReceipt, error)
}

func (s *Server) handleAbandonUninvokedPublication(w http.ResponseWriter, r *http.Request) {
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
	target, ok := s.store.(nonInvocationRecoveryStore)
	if !ok {
		writePublicError(w, http.StatusServiceUnavailable)
		return
	}
	// Check exact saved receipt first. This is readback, never another Publisher
	// invocation; it continues to work after Recovery completes or Publisher fails.
	prior, err := target.GetUninvokedPublication(r.Context(), request.Identity.OperationID)
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
	observer, supported := s.publicationRecovery.(githubpublish.NonInvocationObserver)
	if !supported || observer == nil {
		writePublicError(w, http.StatusServiceUnavailable)
		return
	}
	original, err := target.GetContext(r.Context(), request.Identity.OperationID)
	if err != nil {
		writePublicationRecoveryError(w, err)
		return
	}
	identity, err := githubpublish.IdentityForOperation(original)
	if err != nil || identity != request.Identity || !githubpublish.RecoverablePublicationState(original.State) {
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
	evidence, err := observer.FenceUninvokedPublication(r.Context(), original)
	if err != nil {
		writePublicationRecoveryError(w, err)
		return
	}
	if evidence.Validate() != nil || evidence.Start.Identity != identity {
		writePublicError(w, http.StatusConflict)
		return
	}
	if err := r.Context().Err(); err != nil {
		writePublicationRecoveryError(w, err)
		return
	}
	// The database rechecks the current Recovery epoch and original locked row.
	// No network calls run inside its serializable transaction.
	receipt, err := target.AbandonUninvokedPublication(r.Context(), id.Subject(), request, evidence)
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
func (s *Server) handleGetUninvokedPublication(w http.ResponseWriter, r *http.Request) {
	id, ok := AuthenticatedIdentity(r.Context())
	if !ok || !id.Allows(access.Read) {
		writePublicError(w, http.StatusForbidden)
		return
	}
	target, ok := s.store.(nonInvocationRecoveryStore)
	if !ok {
		writePublicError(w, http.StatusServiceUnavailable)
		return
	}
	receipt, err := target.GetUninvokedPublication(r.Context(), r.PathValue("id"))
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
