package githubpublish

import (
	"crypto/x509"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestPublisherRevalidatesVerifiedChainAndExactClientIdentity(t *testing.T) {
	service, _ := publisherServiceFixture(t)
	for _, tc := range []struct {
		name   string
		change func(*http.Request)
		status int
	}{
		{"valid", func(*http.Request) {}, http.StatusOK},
		{"expired-leaf", func(r *http.Request) { r.TLS.PeerCertificates[0].NotAfter = time.Now().Add(-time.Second) }, http.StatusUnauthorized},
		{"not-yet-valid-leaf", func(r *http.Request) { r.TLS.PeerCertificates[0].NotBefore = time.Now().Add(time.Hour) }, http.StatusUnauthorized},
		{"expired-issuer", func(r *http.Request) {
			r.TLS.VerifiedChains[0] = append(r.TLS.VerifiedChains[0], &x509.Certificate{NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(-time.Second)})
		}, http.StatusUnauthorized},
		{"nil-chain-certificate", func(r *http.Request) { r.TLS.VerifiedChains[0] = append(r.TLS.VerifiedChains[0], nil) }, http.StatusUnauthorized},
		{"unverified-leaf", func(r *http.Request) {
			r.TLS.VerifiedChains[0] = []*x509.Certificate{{Raw: []byte{2}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour)}}
		}, http.StatusUnauthorized},
		{"wrong-purpose", func(r *http.Request) {
			r.TLS.PeerCertificates[0].ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
		}, http.StatusUnauthorized},
		{"wrong-subject", func(r *http.Request) {
			r.TLS.PeerCertificates[0].URIs[0], _ = url.Parse("urn:engineering-platform:worker:other")
		}, http.StatusUnauthorized},
		{"multiple-subjects", func(r *http.Request) {
			r.TLS.PeerCertificates[0].URIs = append(r.TLS.PeerCertificates[0].URIs, r.TLS.PeerCertificates[0].URIs[0])
		}, http.StatusUnauthorized},
		{"valid-alternative-chain", func(r *http.Request) {
			leaf := r.TLS.PeerCertificates[0]
			r.TLS.VerifiedChains = [][]*x509.Certificate{{leaf, nil}, {leaf}}
		}, http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := authorizedPublisherRequest(t, http.MethodGet, "/healthz", nil)
			tc.change(r)
			response := httptest.NewRecorder()
			service.Handler().ServeHTTP(response, r)
			if response.Code != tc.status {
				t.Fatalf("authorization status=%d want=%d", response.Code, tc.status)
			}
		})
	}
}
