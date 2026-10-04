package api

import (
	"github.com/jiying2007/engineering-platform/internal/codexexec"
	"net/http"
)

func (s *Server) handleCodexSourceCheckpoint(w http.ResponseWriter, r *http.Request) {
	_, id, ok := s.codexRepository(w, r)
	if !ok {
		return
	}
	var report codexexec.SourceCheckpoint
	if !decodeJSON(w, r, &report) {
		return
	}
	if report.Validate() != nil {
		writeError(w, http.StatusBadRequest, "invalid source checkpoint")
		return
	}
	if !codexGrant(id, report.Binding.Token.WorkerProfile, report.Binding.Token.ProfileDigest) {
		writeError(w, http.StatusForbidden, "exact Codex profile denied")
		return
	}
	repo, ok := s.store.(codexexec.CheckpointRepository)
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "source checkpoint storage unavailable")
		return
	}
	receipt, err := repo.SaveCodexCheckpoint(r.Context(), id.Subject(), report)
	if err != nil {
		writeWorkerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, receipt)
}
