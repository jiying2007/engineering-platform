package api

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"

	"github.com/jiying2007/engineering-platform/internal/access"
	"github.com/jiying2007/engineering-platform/internal/core"
	"github.com/jiying2007/engineering-platform/internal/store"
	"github.com/jiying2007/engineering-platform/internal/strictjson"
)

// RecoveryCompletionGate must verify independently retained reconciliation facts
// for this exact epoch. A caller's reconciled=true is never the authority.
// A missing gate disables completion on the authenticated API.
type RecoveryCompletionGate interface {
	AuthorizeCompletion(context.Context, string, uint64) error
}

type AuthenticatedOptions struct{ RecoveryCompletion RecoveryCompletionGate }
type identityKey struct{}

func AuthenticatedIdentity(ctx context.Context) (access.Identity, bool) {
	id, ok := ctx.Value(identityKey{}).(access.Identity)
	return id, ok
}

// Every admitted route has an explicit capability. New routes default to denial;
// method/path prefixes, proxy headers and an unknown capability never grant access.
var routeCapabilities = map[string]string{
	"GET /api/v1/capabilities":            access.Read,
	"GET /api/v1/recovery":                access.Read,
	"POST /api/v1/recovery/begin":         access.RecoveryBegin,
	"POST /api/v1/recovery/complete":      access.RecoveryComplete,
	"POST /api/v1/recovery/proofs":        access.RecoveryReconcile,
	"GET /api/v1/recovery/proofs/{epoch}": access.Read,
	"POST /api/v1/work-items":             access.WorkCreate,
	"GET /api/v1/work-items/{id}":         access.Read,
	"POST /api/v1/task-contracts":         access.TaskCreate,
	"GET /api/v1/task-contracts/{id}":     access.Read,
	"POST /api/v1/runs":                   access.RunStart,
	"GET /api/v1/runs/{id}":               access.Read,
	"POST /api/v1/runs/{id}/steer":        access.RunControl,
	"GET /api/v1/steering/{id}":           access.Read,
	"POST /api/v1/runs/{id}/pause":        access.RunControl,
	"POST /api/v1/runs/{id}/resume":       access.RunControl,
	"POST /api/v1/runs/{id}/takeover":     access.RunControl,
	"POST /api/v1/runs/{id}/complete":     access.RunComplete,
	"POST /api/v1/runs/{id}/checkpoints":  access.CheckpointCreate,
	"GET /api/v1/checkpoints/{id}":        access.Read,
	"POST /api/v1/runs/{id}/actions":      access.ActionExecute,
	"GET /api/v1/actions/{id}":            access.Read,
	"POST /api/v1/actions/{id}/reconcile": access.ActionReconcile,
	"POST /api/v1/deliveries":             access.DeliveryCreate,
	"GET /api/v1/deliveries/{id}":         access.Read,
	"POST /api/v1/evidence":               access.EvidenceRegister,
	"GET /api/v1/evidence/{id}":           access.Read,
	"POST /api/v1/verifications":          access.VerificationCreate,
	"GET /api/v1/verifications/{id}":      access.Read,
	"POST /api/v1/reviews":                access.ReviewCreate,
	"GET /api/v1/reviews/{id}":            access.Read,
	"POST /api/v1/closures":               access.ClosureCreate,
	"GET /api/v1/closures/{id}":           access.Read,
}

// NewAuthenticatedHandler is the supported network assembly. NewServer remains
// a contract-test/explicit loopback-development fixture, not a remote endpoint.
func NewAuthenticatedHandler(backend store.Store, actions ActionGateway, policy *access.Policy, options AuthenticatedOptions) (http.Handler, error) {
	if backend == nil || policy == nil {
		return nil, fmt.Errorf("store and access policy are required")
	}
	s := NewServerWithActionGateway(backend, actions)
	s.workerRoutes()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, pattern := s.mux.Handler(r)
		if pattern == "GET /healthz" {
			s.Handler().ServeHTTP(w, r)
			return
		}
		identity, err := policy.Authenticate(r)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "verified client identity required")
			return
		}
		capability, registered := routeCapabilities[pattern]
		if !registered {
			capability, registered = workerRouteCapabilities[pattern]
		}
		if !registered || !identity.Allows(capability) {
			writeError(w, http.StatusForbidden, "capability denied")
			return
		}
		r = r.Clone(context.WithValue(r.Context(), identityKey{}, identity))
		if r.Method == http.MethodPost {
			mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if err != nil || mediaType != "application/json" || r.Header.Get("Content-Encoding") != "" {
				writeError(w, http.StatusUnsupportedMediaType, "unencoded application/json required")
				return
			}
			original := r.Body
			if original == nil {
				writeError(w, http.StatusBadRequest, "JSON body required")
				return
			}
			data, err := strictjson.ReadObject(original)
			_ = original.Close()
			if err != nil {
				writeError(w, http.StatusBadRequest, "invalid, ambiguous or oversized JSON")
				return
			}
			if status := authorizeBody(r.Context(), pattern, identity, data, options); status != 0 {
				writeError(w, status, http.StatusText(status))
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(data))
			r.ContentLength = int64(len(data))
		}
		// ServeMux, not its bare Handler lookup result, must set wildcard PathValues.
		s.Handler().ServeHTTP(w, r)
	}), nil
}

// The same checked bytes go to the existing handlers. Strict member validation
// prevents duplicate/case aliases from selecting a different identity downstream.
func authorizeBody(ctx context.Context, pattern string, id access.Identity, data []byte, options AuthenticatedOptions) int {
	switch pattern {
	case "POST /api/v1/work-items":
		var body core.WorkItem
		if strictjson.Decode(data, &body) != nil {
			return http.StatusBadRequest
		}
		if body.HumanOwner != id.Subject() {
			return http.StatusForbidden
		}
	case "POST /api/v1/task-contracts":
		var body createTaskRequest
		if strictjson.Decode(data, &body) != nil {
			return http.StatusBadRequest
		}
		if body.Material.DegradationApprovedBy != "" && (!id.Allows(access.MaterialDegrade) || body.Material.DegradationApprovedBy != id.Subject() || !strictjson.NonBlank(body.Material.DegradationReason)) {
			return http.StatusForbidden
		}
	case "POST /api/v1/runs/{id}/steer":
		var body steerRequest
		if strictjson.Decode(data, &body) != nil {
			return http.StatusBadRequest
		}
		if body.Actor != id.Subject() {
			return http.StatusForbidden
		}
	case "POST /api/v1/runs/{id}/actions":
		var body createActionRequest
		if strictjson.Decode(data, &body) != nil {
			return http.StatusBadRequest
		}
		if body.RequestedBy != id.Subject() || !id.AllowsAction(body.Action, string(body.RiskClass), body.Capability) {
			return http.StatusForbidden
		}
	case "POST /api/v1/evidence":
		var body createEvidenceRequest
		if strictjson.Decode(data, &body) != nil {
			return http.StatusBadRequest
		}
		if !id.AllowsEvidence(body.Evidence.Issuer, body.Evidence.Procedure) {
			return http.StatusForbidden
		}
	case "POST /api/v1/verifications":
		var body createVerificationRequest
		if strictjson.Decode(data, &body) != nil {
			return http.StatusBadRequest
		}
		if body.Verifier != id.Subject() {
			return http.StatusForbidden
		}
	case "POST /api/v1/reviews":
		var body createReviewRequest
		if strictjson.Decode(data, &body) != nil {
			return http.StatusBadRequest
		}
		if body.Reviewer != id.Subject() {
			return http.StatusForbidden
		}
	case "POST /api/v1/recovery/proofs":
		var body createRecoveryProofRequest
		if strictjson.Decode(data, &body) != nil || body.RecoveryEpoch == 0 || body.Reconciler == "" {
			return http.StatusBadRequest
		}
		if body.Reconciler != id.Subject() {
			return http.StatusForbidden
		}
	case "POST /api/v1/recovery/complete":
		var body completeRecoveryRequest
		if strictjson.Decode(data, &body) != nil || body.RecoveryEpoch == 0 {
			return http.StatusBadRequest
		}
		if options.RecoveryCompletion == nil {
			return http.StatusServiceUnavailable
		}
		if options.RecoveryCompletion.AuthorizeCompletion(ctx, id.Subject(), body.RecoveryEpoch) != nil {
			return http.StatusForbidden
		}
	}
	return 0
}
