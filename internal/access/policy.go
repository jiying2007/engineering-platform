// Package access binds directly verified mTLS identities to explicit Core grants.
// It is a single-trust-domain policy, not a tenant or row-level ACL system.
package access

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"time"

	"github.com/jiying2007/engineering-platform/internal/strictjson"
)

var ErrUnauthenticated = errors.New("verified client identity required")

const (
	Read               = "core:read"
	WorkCreate         = "work:create"
	TaskCreate         = "task:create"
	RunStart           = "run:start"
	RunControl         = "run:control"
	RunComplete        = "run:complete"
	CheckpointCreate   = "checkpoint:create"
	ActionExecute      = "action:execute"
	ActionReconcile    = "action:reconcile"
	DeliveryCreate     = "delivery:create"
	EvidenceRegister   = "evidence:register"
	VerificationCreate = "verification:create"
	ReviewCreate       = "review:create"
	ClosureCreate      = "closure:create"
	RecoveryBegin      = "recovery:begin"
	RecoveryComplete   = "recovery:complete"
	MaterialDegrade    = "material:degrade"
)

var capabilities = map[string]bool{
	WorkerPoll: true, WorkerReport: true,
	Read: true, WorkCreate: true, TaskCreate: true, RunStart: true, RunControl: true,
	RunComplete: true, CheckpointCreate: true, ActionExecute: true, ActionReconcile: true,
	DeliveryCreate: true, EvidenceRegister: true, VerificationCreate: true, ReviewCreate: true,
	ClosureCreate: true, RecoveryBegin: true, RecoveryComplete: true, MaterialDegrade: true,
}
var subjectPattern = regexp.MustCompile(`^urn:engineering-platform:[A-Za-z0-9][A-Za-z0-9._:-]{0,191}$`)
var namePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/-]{0,191}$`)

type ActionGrant struct {
	Action     string `json:"action"`
	RiskClass  string `json:"risk_class"`
	Capability string `json:"capability"`
}

type PrincipalSpec struct {
	WorkerProfiles     []string      `json:"worker_profiles,omitempty"`
	Subject            string        `json:"subject"`
	Scope              string        `json:"scope"`
	Capabilities       []string      `json:"capabilities"`
	Actions            []ActionGrant `json:"actions,omitempty"`
	EvidenceIssuer     string        `json:"evidence_issuer,omitempty"`
	EvidenceProcedures []string      `json:"evidence_procedures,omitempty"`
}

type Document struct {
	Version    int             `json:"version"`
	Principals []PrincipalSpec `json:"principals"`
}

// Identity's grants are copied into private maps and cannot be changed by a
// request, the caller's source slices, or a returned slice.
type Identity struct {
	workerProfiles map[string]bool
	subject        string
	capabilities   map[string]bool
	actions        map[string]ActionGrant
	issuer         string
	procedures     map[string]bool
}

func (p Identity) Subject() string               { return p.subject }
func (p Identity) Allows(capability string) bool { return p.capabilities[capability] }
func (p Identity) AllowsAction(action, risk, capability string) bool {
	grant, ok := p.actions[action]
	return p.Allows(ActionExecute) && ok && grant.RiskClass == risk && grant.Capability == capability
}
func (p Identity) AllowsEvidence(issuer, procedure string) bool {
	return p.Allows(EvidenceRegister) && issuer == p.issuer && p.procedures[procedure]
}

type Policy struct{ principals map[string]Identity }

func Decode(data []byte) (*Policy, error) {
	var doc Document
	if err := strictjson.Decode(data, &doc); err != nil {
		return nil, fmt.Errorf("decode access policy: %w", err)
	}
	return New(doc)
}

func New(doc Document) (*Policy, error) {
	if doc.Version != 1 || len(doc.Principals) == 0 || len(doc.Principals) > 256 {
		return nil, fmt.Errorf("access policy requires version 1 and 1..256 principals")
	}
	policy := &Policy{principals: map[string]Identity{}}
	for _, spec := range doc.Principals {
		if !subjectPattern.MatchString(spec.Subject) || spec.Scope != "platform" {
			return nil, fmt.Errorf("explicit platform scope and canonical URI subject required")
		}
		if _, exists := policy.principals[spec.Subject]; exists {
			return nil, fmt.Errorf("duplicate principal")
		}
		if len(spec.Capabilities) == 0 || len(spec.Capabilities) > len(capabilities) || len(spec.Actions) > 64 || len(spec.EvidenceProcedures) > 64 {
			return nil, fmt.Errorf("invalid grant count")
		}
		id := Identity{subject: spec.Subject, capabilities: map[string]bool{}, actions: map[string]ActionGrant{}, issuer: spec.EvidenceIssuer, procedures: map[string]bool{}}
		for _, capability := range spec.Capabilities {
			if !capabilities[capability] || id.capabilities[capability] {
				return nil, fmt.Errorf("unknown or duplicate capability")
			}
			id.capabilities[capability] = true
		}
		for _, grant := range spec.Actions {
			if !namePattern.MatchString(grant.Action) || !namePattern.MatchString(grant.Capability) || !validRisk(grant.RiskClass) {
				return nil, fmt.Errorf("invalid action grant")
			}
			if _, exists := id.actions[grant.Action]; exists {
				return nil, fmt.Errorf("duplicate action grant")
			}
			id.actions[grant.Action] = grant
		}
		if id.Allows(ActionExecute) != (len(id.actions) > 0) {
			return nil, fmt.Errorf("action execution requires exact action grants")
		}
		for _, procedure := range spec.EvidenceProcedures {
			if !namePattern.MatchString(procedure) || id.procedures[procedure] {
				return nil, fmt.Errorf("invalid or duplicate evidence procedure")
			}
			id.procedures[procedure] = true
		}
		if id.Allows(EvidenceRegister) {
			if !namePattern.MatchString(id.issuer) || len(id.procedures) == 0 {
				return nil, fmt.Errorf("evidence grant requires issuer and procedures")
			}
		} else if id.issuer != "" || len(id.procedures) > 0 {
			return nil, fmt.Errorf("evidence binding without evidence grant")
		}
		if err := configureWorkerProfiles(spec, &id); err != nil {
			return nil, err
		}
		policy.principals[id.subject] = id
	}
	return policy, nil
}

func validRisk(risk string) bool {
	return risk == "OBSERVE" || risk == "CONTROLLED_MUTATION" || risk == "HIGH_RISK"
}

// Authenticate trusts only a directly verified TLS client certificate. Common
// Name, HTTP forwarding headers and bearer strings are not identity authorities.
// Recheck chain time validity for long-lived HTTP connections after handshake.
func (p *Policy) Authenticate(r *http.Request) (Identity, error) {
	if p == nil || r == nil || r.TLS == nil || !r.TLS.HandshakeComplete || r.TLS.Version < tls.VersionTLS13 || len(r.TLS.PeerCertificates) == 0 || len(r.TLS.VerifiedChains) == 0 {
		return Identity{}, ErrUnauthenticated
	}
	leaf := r.TLS.PeerCertificates[0]
	if leaf == nil || len(leaf.URIs) != 1 || leaf.URIs[0] == nil {
		return Identity{}, ErrUnauthenticated
	}
	clientPurpose := false
	for _, usage := range leaf.ExtKeyUsage {
		if usage == x509.ExtKeyUsageClientAuth {
			clientPurpose = true
		}
	}
	if !clientPurpose {
		return Identity{}, ErrUnauthenticated
	}
	now, valid := time.Now(), false
	for _, chain := range r.TLS.VerifiedChains {
		if len(chain) == 0 || chain[0] == nil || !bytes.Equal(chain[0].Raw, leaf.Raw) {
			continue
		}
		valid = true
		for _, cert := range chain {
			if cert == nil || now.Before(cert.NotBefore) || !now.Before(cert.NotAfter) {
				valid = false
				break
			}
		}
		if valid {
			break
		}
	}
	if !valid {
		return Identity{}, ErrUnauthenticated
	}
	id, exists := p.principals[leaf.URIs[0].String()]
	if !exists {
		return Identity{}, ErrUnauthenticated
	}
	return id, nil
}
