package access

import (
	"crypto/tls"
	"crypto/x509"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/testsupport"
)

const testSubject = "urn:engineering-platform:engineer:alice"

func TestPolicyRejectsAmbiguousOrOverbroadGrants(t *testing.T) {
	for _, data := range []string{
		`{"version":1,"principals":[]}`,
		`{"version":1,"Version":1,"principals":[]}`,
		`{"version":1,"principals":[{"subject":"` + testSubject + `","scope":"platform","capabilities":["*"]}]}`,
		`{"version":1,"principals":[{"subject":"` + testSubject + `","scope":"platform","capabilities":["action:execute"]}]}`,
		`{"version":1,"principals":[{"subject":"` + testSubject + `","scope":"platform","capabilities":["evidence:register"]}]}`,
		`{"version":1,"principals":[{"subject":"` + testSubject + `","scope":"tenant","capabilities":["core:read"]}]}`,
		`{"version":1,"principals":[{"subject":"` + testSubject + `","scope":"platform","capabilities":["core:read","core:read"]}]}`,
	} {
		if _, err := Decode([]byte(data)); err == nil {
			t.Fatal("invalid policy accepted")
		}
	}
	spec := PrincipalSpec{Subject: testSubject, Scope: "platform", Capabilities: []string{Read}}
	if _, err := New(Document{Version: 1, Principals: []PrincipalSpec{spec, spec}}); err == nil {
		t.Fatal("duplicate principal")
	}
}

func TestPolicyCopiesGrantsAndBindsVerifiedCertificate(t *testing.T) {
	pki := testsupport.NewPKI(t)
	cert := pki.ClientCertificate(t, testSubject)
	spec := PrincipalSpec{Subject: testSubject, Scope: "platform", Capabilities: []string{Read, ActionExecute, EvidenceRegister}, Actions: []ActionGrant{{Action: "ci.dispatch", RiskClass: "CONTROLLED_MUTATION", Capability: "ci"}}, EvidenceIssuer: "ci", EvidenceProcedures: []string{"ci.test"}}
	policy, err := New(Document{Version: 1, Principals: []PrincipalSpec{spec}})
	if err != nil {
		t.Fatal(err)
	}
	spec.Capabilities[0] = RecoveryComplete
	spec.Actions[0].RiskClass = "OBSERVE"
	spec.EvidenceProcedures[0] = "release.sign"
	r := httptest.NewRequest("GET", "/api/v1/recovery", nil)
	r.TLS = &tls.ConnectionState{Version: tls.VersionTLS13, HandshakeComplete: true, PeerCertificates: []*x509.Certificate{cert.Leaf}, VerifiedChains: [][]*x509.Certificate{{cert.Leaf, pki.CA}}}
	id, err := policy.Authenticate(r)
	if err != nil || id.Subject() != testSubject || !id.Allows(Read) || id.Allows(RecoveryComplete) || !id.AllowsAction("ci.dispatch", "CONTROLLED_MUTATION", "ci") || id.AllowsAction("ci.dispatch", "OBSERVE", "ci") || !id.AllowsEvidence("ci", "ci.test") || id.AllowsEvidence("ci", "release.sign") {
		t.Fatalf("grant binding failed: %v", err)
	}
	for _, mode := range []string{"no_tls", "unverified", "wrong_leaf", "expired", "ambiguous", "unknown", "no_uri", "old_tls", "no_client_purpose"} {
		t.Run(mode, func(t *testing.T) {
			req := r.Clone(r.Context())
			state := *r.TLS
			req.TLS = &state
			leaf := *cert.Leaf
			state.PeerCertificates = []*x509.Certificate{&leaf}
			state.VerifiedChains = [][]*x509.Certificate{{&leaf, pki.CA}}
			switch mode {
			case "no_tls":
				req.TLS = nil
			case "unverified":
				state.VerifiedChains = nil
			case "wrong_leaf":
				state.VerifiedChains = [][]*x509.Certificate{{pki.CA}}
			case "expired":
				leaf.NotAfter = time.Now().Add(-time.Second)
			case "ambiguous":
				leaf.URIs = append(leaf.URIs, leaf.URIs[0])
			case "unknown":
				other := pki.ClientCertificate(t, "urn:engineering-platform:unknown")
				state.PeerCertificates = []*x509.Certificate{other.Leaf}
				state.VerifiedChains = [][]*x509.Certificate{{other.Leaf, pki.CA}}
			case "no_uri":
				leaf.URIs = nil
			case "old_tls":
				state.Version = tls.VersionTLS12
			case "no_client_purpose":
				leaf.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
			}
			req.Header.Set("X-Forwarded-Client-Cert", testSubject)
			req.Header.Set("Authorization", "Bearer pretend-admin")
			if _, err := policy.Authenticate(req); err == nil {
				t.Fatal("unverified identity accepted")
			}
		})
	}
}

func TestTLSConfigurationRejectsUnsafeAndMissingInputs(t *testing.T) {
	pki := testsupport.NewPKI(t)
	dir := t.TempDir()
	cert, key, ca := filepath.Join(dir, "server.pem"), filepath.Join(dir, "server.key"), filepath.Join(dir, "client-ca.pem")
	for path, data := range map[string][]byte{cert: pki.ServerCertPEM, key: pki.ServerKeyPEM, ca: pki.CAPEM} {
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	config, err := LoadServerTLS(cert, key, ca)
	if err != nil || config.ClientAuth != tls.RequireAndVerifyClientCert || config.MinVersion != tls.VersionTLS13 || !config.SessionTicketsDisabled {
		t.Fatalf("unsafe TLS config: %v", err)
	}
	if err := os.Chmod(key, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadServerTLS(cert, key, ca); err == nil {
		t.Fatal("world-readable private key accepted")
	}
	if err := os.Chmod(key, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ca, []byte("not a CA"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadServerTLS(cert, key, ca); err == nil {
		t.Fatal("invalid CA accepted")
	}
	if _, err := ReadConfiguration(dir, false); err == nil {
		t.Fatal("directory accepted as policy")
	}
}
