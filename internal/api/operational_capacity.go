package api

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"slices"

	"github.com/jiying2007/engineering-platform/internal/access"
	"github.com/jiying2007/engineering-platform/internal/production"
)

type OperationalCapacityOptions struct {
	Source  production.CapacitySource
	Workers []production.WorkerCapacityGrant
}

func copyOperationalCapacity(input *OperationalCapacityOptions, policy *access.Policy) (*OperationalCapacityOptions, error) {
	if input == nil {
		return nil, nil
	}
	if err := input.Source.Validate(); err != nil {
		return nil, err
	}
	if _, err := production.CapacityProfiles(input.Workers); err != nil {
		return nil, err
	}
	expected := []production.WorkerCapacityGrant{}
	for _, grant := range policy.WorkerClaimGrants() {
		lane := "ADMISSION"
		if grant.Prepares {
			lane = "PREPARATION"
		}
		expected = append(expected, production.WorkerCapacityGrant{Subject: grant.Subject, Profiles: grant.Profiles, Lane: lane})
	}
	if !reflect.DeepEqual(input.Workers, expected) {
		return nil, fmt.Errorf("capacity workers must match the current loaded access policy")
	}
	result := &OperationalCapacityOptions{Source: input.Source, Workers: make([]production.WorkerCapacityGrant, len(input.Workers))}
	for i, worker := range input.Workers {
		result.Workers[i] = worker
		result.Workers[i].Profiles = slices.Clone(worker.Profiles)
	}
	return result, nil
}

func (s *Server) handleOperationalCapacity(w http.ResponseWriter, r *http.Request) {
	id, ok := AuthenticatedIdentity(r.Context())
	if !ok || !id.Allows(access.Read) {
		writeError(w, http.StatusForbidden, "read capability required")
		return
	}
	reader, available := s.store.(interface {
		ReadOperationalCapacity(context.Context, []production.WorkerCapacityGrant) (production.CapacityFacts, error)
	})
	if !available || s.operationalCapacity == nil {
		writeError(w, http.StatusServiceUnavailable, "identified Core capacity source unavailable")
		return
	}
	facts, err := reader.ReadOperationalCapacity(r.Context(), s.operationalCapacity.Workers)
	if err != nil || facts.Validate(s.operationalCapacity.Workers) != nil || r.Context().Err() != nil {
		writeError(w, http.StatusServiceUnavailable, "Core capacity facts unavailable")
		return
	}
	response := production.CapacityResponse{
		Version: production.CapacityVersion, Scope: production.CapacityScope,
		Source: s.operationalCapacity.Source, GeneratedAt: s.now(), Facts: facts,
	}
	if response.ValidateAt(s.now()) != nil {
		writeError(w, http.StatusServiceUnavailable, "Core capacity facts stale or inconsistent")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, response)
}
