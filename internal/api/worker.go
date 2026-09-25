package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/jiying2007/engineering-platform/internal/access"
	"github.com/jiying2007/engineering-platform/internal/store"
	"github.com/jiying2007/engineering-platform/internal/workerqueue"
)

var workerRouteCapabilities = map[string]string{
	"POST /api/v1/worker/prepare-claim": access.WorkerPrepare,
	"POST /api/v1/worker/prepared":      access.WorkerPrepare,
	"GET /api/v1/runs/{id}/preparation": access.Read,
	"POST /api/v1/worker/claim":         access.WorkerPoll,
	"POST /api/v1/worker/renew":         access.WorkerPoll,
	"POST /api/v1/worker/report":        access.WorkerReport,
	"GET /api/v1/runs/{id}/inbox":       access.Read,
}

// Registered only by the authenticated constructor. Bare development/test APIs
// cannot accidentally expose queue mutation to an anonymous local caller.
func (s *Server) workerRoutes() {
	s.mux.HandleFunc("POST /api/v1/worker/prepare-claim", s.handlePrepareClaim)
	s.mux.HandleFunc("POST /api/v1/worker/prepared", s.handleWorkerPrepared)
	s.mux.HandleFunc("GET /api/v1/runs/{id}/preparation", s.handleGetPreparation)
	s.mux.HandleFunc("POST /api/v1/worker/claim", s.handleWorkerClaim)
	s.mux.HandleFunc("POST /api/v1/worker/renew", s.handleWorkerRenew)
	s.mux.HandleFunc("POST /api/v1/worker/report", s.handleWorkerReport)
	s.mux.HandleFunc("GET /api/v1/runs/{id}/inbox", s.handleWorkerInbox)
}
func (s *Server) workerRepository(w http.ResponseWriter, r *http.Request, capability string) (workerqueue.Repository, access.Identity, bool) {
	id, ok := AuthenticatedIdentity(r.Context())
	if !ok || !id.Allows(capability) {
		writeError(w, http.StatusForbidden, "worker capability denied")
		return nil, id, false
	}
	repository, ok := s.store.(workerqueue.Repository)
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "durable worker inbox unavailable")
		return nil, id, false
	}
	return repository, id, true
}
func (s *Server) handleWorkerClaim(w http.ResponseWriter, r *http.Request) {
	repository, id, ok := s.workerRepository(w, r, access.WorkerPoll)
	if !ok {
		return
	}
	var req struct {
		Profile string `json:"worker_profile"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if !id.AllowsWorkerProfile(req.Profile) {
		writeError(w, http.StatusForbidden, "worker profile denied")
		return
	}
	assignment, err := repository.ClaimInput(r.Context(), id.Subject(), req.Profile)
	if err != nil {
		writeWorkerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Assignment *workerqueue.Assignment `json:"assignment"`
	}{assignment})
}
func (s *Server) handleWorkerRenew(w http.ResponseWriter, r *http.Request) {
	repository, id, ok := s.workerRepository(w, r, access.WorkerPoll)
	if !ok {
		return
	}
	var token workerqueue.Token
	if !decodeJSON(w, r, &token) {
		return
	}
	if !id.AllowsWorkerProfile(token.Profile) {
		writeError(w, http.StatusForbidden, "worker profile denied")
		return
	}
	until, err := repository.RenewInput(r.Context(), id.Subject(), token)
	if err != nil {
		writeWorkerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		LeaseUntil time.Time `json:"lease_until"`
	}{until})
}
func (s *Server) handleWorkerReport(w http.ResponseWriter, r *http.Request) {
	repository, id, ok := s.workerRepository(w, r, access.WorkerReport)
	if !ok {
		return
	}
	var report workerqueue.Report
	if !decodeJSON(w, r, &report) {
		return
	}
	if !id.AllowsWorkerProfile(report.Token.Profile) {
		writeError(w, http.StatusForbidden, "worker profile denied")
		return
	}
	receipt, err := repository.ReportInput(r.Context(), id.Subject(), report)
	if err != nil {
		writeWorkerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, receipt)
}
func (s *Server) handleWorkerInbox(w http.ResponseWriter, r *http.Request) {
	repository, _, ok := s.workerRepository(w, r, access.Read)
	if !ok {
		return
	}
	status, err := repository.GetInbox(r.Context(), r.PathValue("id"))
	if err != nil {
		writeWorkerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}
func writeWorkerError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "worker intent not found")
	case errors.Is(err, workerqueue.ErrLease), errors.Is(err, workerqueue.ErrRecovery), errors.Is(err, workerqueue.ErrInactive), errors.Is(err, workerqueue.ErrPaused):
		writeError(w, http.StatusConflict, "worker authority unavailable or superseded")
	case errors.Is(err, workerqueue.ErrIdentity):
		writeError(w, http.StatusUnprocessableEntity, "worker input identity mismatch")
	default:
		writeError(w, http.StatusServiceUnavailable, "worker store unavailable")
	}
}
