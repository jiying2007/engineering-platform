package api

import (
	"github.com/jiying2007/engineering-platform/internal/access"
	"github.com/jiying2007/engineering-platform/internal/codexexec"
	"net/http"
)

func (s *Server) handleCodexContinue(w http.ResponseWriter, r *http.Request) {
	id, ok := AuthenticatedIdentity(r.Context())
	if !ok || !id.Allows(access.RunControl) || !id.Allows(access.RunStart) {
		writeError(w, http.StatusForbidden, "both RunControl and RunStart required")
		return
	}
	var q codexexec.ContinueRequest
	if !decodeJSON(w, r, &q) {
		return
	}
	if q.Validate() != nil || q.SourceRunID != r.PathValue("id") {
		writeError(w, http.StatusBadRequest, "invalid continuation identity")
		return
	}
	repo, ok := s.store.(codexexec.ContinuationRepository)
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "continuation storage unavailable")
		return
	}
	receipt, err := repo.ContinueCodex(r.Context(), id.Subject(), q)
	if err != nil {
		writeWorkerError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, receipt)
}
func (s *Server) handleCodexContinuation(w http.ResponseWriter, r *http.Request) {
	id, ok := AuthenticatedIdentity(r.Context())
	if !ok || !id.Allows(access.Read) {
		writeError(w, http.StatusForbidden, "read denied")
		return
	}
	repo, ok := s.store.(codexexec.ContinuationRepository)
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "continuation storage unavailable")
		return
	}
	receipt, err := repo.GetCodexContinuation(r.Context(), r.PathValue("id"))
	if err != nil {
		writeWorkerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, receipt)
}
