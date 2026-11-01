package api

import (
	"net/http"

	"github.com/jiying2007/engineering-platform/internal/access"
	"github.com/jiying2007/engineering-platform/internal/production"
)

// handleOperationalAttention reads the same status authority as /operations/status.
// It neither sends notifications nor writes a task/Action/recovery record.
func (s *Server) handleOperationalAttention(w http.ResponseWriter, r *http.Request) {
	identity, ok := AuthenticatedIdentity(r.Context())
	if !ok || !identity.Allows(access.Read) {
		writeError(w, http.StatusForbidden, "read capability required")
		return
	}
	reader, ok := s.store.(operationalStatusReader)
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "operational status is unavailable")
		return
	}
	status, err := reader.ReadOperationalStatus(r.Context())
	if err != nil || r.Context().Err() != nil {
		writeError(w, http.StatusServiceUnavailable, "operational status read failed")
		return
	}
	projection, err := production.ProjectOperationalAttention(status, s.now())
	if err != nil || r.Context().Err() != nil {
		writeError(w, http.StatusServiceUnavailable, "operational attention projection unavailable")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, projection)
}
