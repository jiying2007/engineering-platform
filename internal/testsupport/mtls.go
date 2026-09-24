// Package testsupport supplies ephemeral certificate fixtures. It is imported
// only by tests; no fixture key, CA, credential or permissive policy is retained.
package testsupport

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"testing"
	"time"
)

type PKI struct {
	CA                *x509.Certificate
	key               ed25519.PrivateKey
	Roots             *x509.CertPool
	CAPEM             []byte
	ServerCertPEM     []byte
	ServerKeyPEM      []byte
	ServerCertificate tls.Certificate
}

func NewPKI(t *testing.T) *PKI {
	t.Helper()
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "ep-test-only-ca"}, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	der, err := x509.CreateCertificate(rand.Reader, template, template, pub, key)
	if err != nil {
		t.Fatal(err)
	}
	ca, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	p := &PKI{CA: ca, key: key, CAPEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), Roots: x509.NewCertPool()}
	p.Roots.AddCert(ca)
	server := &x509.Certificate{DNSNames: []string{"localhost"}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")}, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	p.ServerCertificate, p.ServerCertPEM, p.ServerKeyPEM = p.issue(t, server)
	return p
}

func (p *PKI) issue(t *testing.T, template *x509.Certificate) (tls.Certificate, []byte, []byte) {
	t.Helper()
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template.SerialNumber, err = rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		t.Fatal(err)
	}
	template.NotBefore, template.NotAfter = time.Now().Add(-time.Minute), time.Now().Add(30*time.Minute)
	template.KeyUsage = x509.KeyUsageDigitalSignature
	der, err := x509.CreateCertificate(rand.Reader, template, p.CA, pub, p.key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM, keyPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	cert.Leaf, err = x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return cert, certPEM, keyPEM
}

func (p *PKI) ClientCertificate(t *testing.T, subjects ...string) tls.Certificate {
	t.Helper()
	template := &x509.Certificate{Subject: pkix.Name{CommonName: "not-an-authority"}, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}
	for _, subject := range subjects {
		u, err := url.Parse(subject)
		if err != nil {
			t.Fatal(err)
		}
		template.URIs = append(template.URIs, u)
	}
	cert, _, _ := p.issue(t, template)
	return cert
}

func (p *PKI) ServerTLS() *tls.Config {
	return &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{p.ServerCertificate}, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: p.Roots}
}

func (p *PKI) Client(t *testing.T, subjects ...string) *http.Client {
	t.Helper()
	cert := p.ClientCertificate(t, subjects...)
	transport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: p.Roots, Certificates: []tls.Certificate{cert}}}
	t.Cleanup(transport.CloseIdleConnections)
	return &http.Client{Transport: transport, Timeout: 10 * time.Second}
}
