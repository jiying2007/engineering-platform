package api

import (
	"net/http"
	"time"

	"github.com/jiying2007/engineering-platform/internal/access"
	"github.com/jiying2007/engineering-platform/internal/offline"
	"github.com/jiying2007/engineering-platform/internal/workerqueue"
)

func (s *Server) offlineRepository(w http.ResponseWriter, r *http.Request) (offline.Repository, access.Identity, bool) {
	id, ok := AuthenticatedIdentity(r.Context())
	repo, exists := s.store.(offline.Repository)
	if !ok || !id.Allows(access.WorkerPrepare) || !id.Allows(access.ActionExecute) {
		writeError(w, http.StatusForbidden, "offline execution grant required")
		return nil, id, false
	}
	if !exists {
		writeError(w, http.StatusServiceUnavailable, "offline execution store unavailable")
		return nil, id, false
	}
	return repo, id, true
}
func offlineGrant(id access.Identity, profile, digest string) bool {
	return id.AllowsWorkerProfile(profile) && id.AllowsAction(offline.Action, "CONTROLLED_MUTATION", digest)
}
func (s *Server) handleOfflineStart(w http.ResponseWriter, r *http.Request) {
	repo, id, ok := s.offlineRepository(w, r)
	if !ok {
		return
	}
	var request offline.Start
	if !decodeJSON(w, r, &request) {
		return
	}
	digest, err := request.Profile.Digest()
	if err != nil {
		writeWorkerError(w, workerqueue.ErrIdentity)
		return
	}
	if !offlineGrant(id, request.WorkerProfile, digest) {
		writeError(w, http.StatusForbidden, "exact offline profile not granted")
		return
	}
	permit, err := repo.StartOffline(r.Context(), id.Subject(), request)
	if err != nil {
		writeWorkerError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, permit)
}
func (s *Server) handleOfflineRenew(w http.ResponseWriter, r *http.Request) {
	repo, id, ok := s.offlineRepository(w, r)
	if !ok {
		return
	}
	var token offline.Token
	if !decodeJSON(w, r, &token) {
		return
	}
	if !offlineGrant(id, token.WorkerProfile, token.ProfileDigest) {
		writeError(w, http.StatusForbidden, "offline profile denied")
		return
	}
	until, err := repo.RenewOffline(r.Context(), id.Subject(), token)
	if err != nil {
		writeWorkerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		LeaseUntil time.Time `json:"lease_until"`
	}{until})
}
func (s *Server) handleOfflineReport(w http.ResponseWriter, r *http.Request) {
	repo, id, ok := s.offlineRepository(w, r)
	if !ok {
		return
	}
	var report offline.Report
	if !decodeJSON(w, r, &report) {
		return
	}
	if !offlineGrant(id, report.Token.WorkerProfile, report.Token.ProfileDigest) {
		writeError(w, http.StatusForbidden, "offline profile denied")
		return
	}
	receipt, err := repo.FinishOffline(r.Context(), id.Subject(), report)
	if err != nil {
		writeWorkerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, receipt)
}
func (s *Server) handleOfflineFail(w http.ResponseWriter, r *http.Request) {
	repo, id, ok := s.offlineRepository(w, r)
	if !ok {
		return
	}
	var token offline.Token
	if !decodeJSON(w, r, &token) {
		return
	}
	if !offlineGrant(id, token.WorkerProfile, token.ProfileDigest) {
		writeError(w, http.StatusForbidden, "offline profile denied")
		return
	}
	if err := repo.FailOffline(r.Context(), id.Subject(), token); err != nil {
		writeWorkerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"recorded": true})
}
func (s *Server) handleOfflineGet(w http.ResponseWriter, r *http.Request) {
	id, ok := AuthenticatedIdentity(r.Context())
	repo, exists := s.store.(offline.Repository)
	if !ok || !id.Allows(access.Read) {
		writeError(w, http.StatusForbidden, "read denied")
		return
	}
	if !exists {
		writeError(w, http.StatusServiceUnavailable, "offline store unavailable")
		return
	}
	status, err := repo.GetOffline(r.Context(), r.PathValue("id"))
	if err != nil {
		writeWorkerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}
