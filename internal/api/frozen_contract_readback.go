package api

import (
	"net/http"

	"github.com/jiying2007/engineering-platform/internal/canonical"
)

// Digest routes read immutable history. The existing ID route deliberately
// continues to return the latest task revision.
func (s *Server) handleGetFrozenTaskByDigest(w http.ResponseWriter, r *http.Request) {
	digest := r.PathValue("digest")
	if !canonical.ValidDigest(digest) {
		writePublicError(w, http.StatusBadRequest)
		return
	}
	task, err := s.getFrozenTaskForRequest(r.Context(), digest)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if err := r.Context().Err(); err != nil {
		writeStoreError(w, err)
		return
	}
	actual, err := task.Digest()
	if err != nil || actual != digest {
		writePublicError(w, http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"contract": task, "digest": digest})
}

func (s *Server) handleGetFrozenVerificationPlanByDigest(w http.ResponseWriter, r *http.Request) {
	digest := r.PathValue("digest")
	if !canonical.ValidDigest(digest) {
		writePublicError(w, http.StatusBadRequest)
		return
	}
	plan, err := s.getVerificationPlanForRequest(r.Context(), digest)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if err := r.Context().Err(); err != nil {
		writeStoreError(w, err)
		return
	}
	actual, err := plan.Digest()
	if err != nil || actual != digest {
		writePublicError(w, http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"plan": plan, "digest": digest})
}
