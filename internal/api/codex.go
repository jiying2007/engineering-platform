package api

import (
	"net/http"
	"time"

	"github.com/jiying2007/engineering-platform/internal/access"
	"github.com/jiying2007/engineering-platform/internal/codexexec"
	"github.com/jiying2007/engineering-platform/internal/workerqueue"
)

func (s *Server) codexRepository(w http.ResponseWriter, r *http.Request) (codexexec.Repository, access.Identity, bool) {
	id, ok := AuthenticatedIdentity(r.Context())
	repo, exists := s.store.(codexexec.Repository)
	if !ok || !id.Allows(access.WorkerPrepare) || !id.Allows(access.ActionExecute) {
		writeError(w, http.StatusForbidden, "Codex execution grant required")
		return nil, id, false
	}
	if !exists {
		writeError(w, http.StatusServiceUnavailable, "Codex execution store unavailable")
		return nil, id, false
	}
	return repo, id, true
}

func codexGrant(id access.Identity, profile, digest string) bool {
	return id.AllowsWorkerProfile(profile) && id.AllowsAction(codexexec.Action, "CONTROLLED_MUTATION", digest)
}

func (s *Server) handleCodexStart(w http.ResponseWriter, r *http.Request) {
	repo, id, ok := s.codexRepository(w, r)
	if !ok {
		return
	}
	var request codexexec.Start
	if !decodeJSON(w, r, &request) {
		return
	}
	digest, err := request.Profile.Digest()
	if err != nil {
		writeWorkerError(w, workerqueue.ErrIdentity)
		return
	}
	if !codexGrant(id, request.WorkerProfile, digest) {
		writeError(w, http.StatusForbidden, "exact Codex profile not granted")
		return
	}
	permit, err := repo.StartCodex(r.Context(), id.Subject(), request)
	if err != nil {
		writeWorkerError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, permit)
}

func (s *Server) handleCodexRenew(w http.ResponseWriter, r *http.Request) {
	repo, id, ok := s.codexRepository(w, r)
	if !ok {
		return
	}
	var token codexexec.Token
	if !decodeJSON(w, r, &token) {
		return
	}
	if !codexGrant(id, token.WorkerProfile, token.ProfileDigest) {
		writeError(w, http.StatusForbidden, "Codex profile denied")
		return
	}
	until, err := repo.RenewCodex(r.Context(), id.Subject(), token)
	if err != nil {
		writeWorkerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		LeaseUntil time.Time `json:"lease_until"`
	}{until})
}

func (s *Server) handleCodexReport(w http.ResponseWriter, r *http.Request) {
	repo, id, ok := s.codexRepository(w, r)
	if !ok {
		return
	}
	var report codexexec.Report
	if !decodeJSON(w, r, &report) {
		return
	}
	if !codexGrant(id, report.Token.WorkerProfile, report.Token.ProfileDigest) {
		writeError(w, http.StatusForbidden, "Codex profile denied")
		return
	}
	receipt, err := repo.FinishCodex(r.Context(), id.Subject(), report)
	if err != nil {
		writeWorkerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, receipt)
}

func (s *Server) handleCodexFail(w http.ResponseWriter, r *http.Request) {
	repo, id, ok := s.codexRepository(w, r)
	if !ok {
		return
	}
	var token codexexec.Token
	if !decodeJSON(w, r, &token) {
		return
	}
	if !codexGrant(id, token.WorkerProfile, token.ProfileDigest) {
		writeError(w, http.StatusForbidden, "Codex profile denied")
		return
	}
	if err := repo.FailCodex(r.Context(), id.Subject(), token); err != nil {
		writeWorkerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"recorded": true})
}

func (s *Server) handleCodexGet(w http.ResponseWriter, r *http.Request) {
	id, ok := AuthenticatedIdentity(r.Context())
	repo, exists := s.store.(codexexec.Repository)
	if !ok || !id.Allows(access.Read) {
		writeError(w, http.StatusForbidden, "read denied")
		return
	}
	if !exists {
		writeError(w, http.StatusServiceUnavailable, "Codex execution store unavailable")
		return
	}
	status, err := repo.GetCodex(r.Context(), r.PathValue("id"))
	if err != nil {
		writeWorkerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}
