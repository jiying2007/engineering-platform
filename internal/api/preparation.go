package api

import (
	"net/http"

	"github.com/jiying2007/engineering-platform/internal/access"
	"github.com/jiying2007/engineering-platform/internal/preparation"
)

func (s *Server) handlePrepareClaim(w http.ResponseWriter, r *http.Request) {
	_, _, ok := s.workerRepository(w, r, access.WorkerPrepare)
	if !ok {
		return
	}
	repository, ok := s.store.(preparation.Repository)
	if !ok || repository.PreparationReady(r.Context()) != nil {
		writeError(w, http.StatusServiceUnavailable, "worker preparation schema unavailable")
		return
	}
	s.handleWorkerClaim(w, r)
}
func (s *Server) handleWorkerPrepared(w http.ResponseWriter, r *http.Request) {
	_, id, ok := s.workerRepository(w, r, access.WorkerPrepare)
	if !ok {
		return
	}
	repository, ok := s.store.(preparation.Repository)
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "worker preparation unavailable")
		return
	}
	var report preparation.Report
	if !decodeJSON(w, r, &report) {
		return
	}
	if !id.Allows(access.WorkerReport) || !id.AllowsWorkerProfile(report.Input.Token.Profile) {
		writeError(w, http.StatusForbidden, "worker preparation profile denied")
		return
	}
	receipt, err := repository.ReportPrepared(r.Context(), id.Subject(), report)
	if err != nil {
		writeWorkerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, receipt)
}
func (s *Server) handleGetPreparation(w http.ResponseWriter, r *http.Request) {
	_, _, ok := s.workerRepository(w, r, access.Read)
	if !ok {
		return
	}
	repository, ok := s.store.(preparation.Repository)
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "worker preparation unavailable")
		return
	}
	receipt, err := repository.GetPreparation(r.Context(), r.PathValue("id"))
	if err != nil {
		writeWorkerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, receipt)
}
