package api

import (
	"net/http"

	"github.com/jiying2007/engineering-platform/internal/access"
	"github.com/jiying2007/engineering-platform/internal/codexexec"
)

func (s *Server) handleInterrupt(w http.ResponseWriter, r *http.Request) {
	s.handleLiveControl(w, r, codexexec.ControlInterrupt)
}
func (s *Server) handleLiveControl(w http.ResponseWriter, r *http.Request, kind string) {
	id, ok := AuthenticatedIdentity(r.Context())
	if !ok || !id.Allows(access.RunControl) {
		writeError(w, http.StatusForbidden, "authenticated RunControl required")
		return
	}
	var input codexexec.ControlInput
	if !decodeJSON(w, r, &input) {
		return
	}
	if input.Actor != id.Subject() {
		writeError(w, http.StatusForbidden, "control actor mismatch")
		return
	}
	if input.Validate(kind) != nil {
		writeError(w, http.StatusBadRequest, "exact live turn and bounded control input required")
		return
	}
	repo, ok := s.store.(codexexec.ControlRepository)
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "live control delivery unavailable")
		return
	}
	receipt, err := repo.QueueCodexControl(r.Context(), r.PathValue("id"), kind, input)
	if err != nil {
		writeWorkerError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, receipt)
}
func (s *Server) handleControlDelivery(w http.ResponseWriter, r *http.Request) {
	id, ok := AuthenticatedIdentity(r.Context())
	if !ok || !id.Allows(access.Read) {
		writeError(w, http.StatusForbidden, "read denied")
		return
	}
	repo, ok := s.store.(codexexec.ControlRepository)
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "live control delivery unavailable")
		return
	}
	receipt, err := repo.GetCodexControl(r.Context(), r.PathValue("id"))
	if err != nil {
		writeWorkerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, receipt)
}
func (s *Server) handleWorkerControl(w http.ResponseWriter, r *http.Request, operation string) {
	_, id, ok := s.codexRepository(w, r)
	if !ok {
		return
	}
	repo, ok := s.store.(codexexec.ControlRepository)
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "live control delivery unavailable")
		return
	}
	var binding codexexec.ControlBinding
	var settlement codexexec.ControlSettlement
	var closeRequest codexexec.ControlClose
	switch operation {
	case "bind", "claim":
		if !decodeJSON(w, r, &binding) {
			return
		}
	case "report":
		if !decodeJSON(w, r, &settlement) {
			return
		}
		binding = settlement.Binding
	case "close":
		if !decodeJSON(w, r, &closeRequest) {
			return
		}
		binding = closeRequest.Binding
	default:
		writeError(w, http.StatusNotFound, "unknown control operation")
		return
	}
	if binding.Validate() != nil {
		writeError(w, http.StatusBadRequest, "invalid live binding")
		return
	}
	if !codexGrant(id, binding.Token.WorkerProfile, binding.Token.ProfileDigest) {
		writeError(w, http.StatusForbidden, "exact Codex profile denied")
		return
	}
	var response any
	var err error
	switch operation {
	case "bind":
		err = repo.BindCodexControl(r.Context(), id.Subject(), binding)
		response = map[string]bool{"bound": err == nil}
	case "claim":
		var delivery *codexexec.ControlDelivery
		delivery, err = repo.ClaimCodexControl(r.Context(), id.Subject(), binding)
		response = struct {
			Delivery *codexexec.ControlDelivery `json:"delivery"`
		}{delivery}
	case "report":
		response, err = repo.SettleCodexControl(r.Context(), id.Subject(), settlement)
	case "close":
		response, err = repo.CloseCodexControl(r.Context(), id.Subject(), closeRequest)
	}
	if err != nil {
		writeWorkerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleControlBind(w http.ResponseWriter, r *http.Request) {
	s.handleWorkerControl(w, r, "bind")
}
func (s *Server) handleControlClaim(w http.ResponseWriter, r *http.Request) {
	s.handleWorkerControl(w, r, "claim")
}
func (s *Server) handleControlReport(w http.ResponseWriter, r *http.Request) {
	s.handleWorkerControl(w, r, "report")
}
func (s *Server) handleControlClose(w http.ResponseWriter, r *http.Request) {
	s.handleWorkerControl(w, r, "close")
}
