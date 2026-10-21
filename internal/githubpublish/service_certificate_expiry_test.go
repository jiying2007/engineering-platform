package githubpublish

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"net/url"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/testsupport"
)

func TestPublisherRejectsNewPublishAfterPeerCertExpiresOnReusedConnection(t *testing.T) {
	service, plan := publisherServiceFixture(t)
	serverPKI := testsupport.NewPKI(t)
	now := time.Now()
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	caTemplate := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "expiry-test-client-ca"}, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	der, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, pub, key)
	if err != nil {
		t.Fatal(err)
	}
	ca, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	leafPub, leafKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	subject, _ := url.Parse(service.controlSubject)
	leafTemplate := &x509.Certificate{SerialNumber: big.NewInt(2), NotBefore: now.Add(-time.Minute), NotAfter: now.Add(3 * time.Second), URIs: []*url.URL{subject}, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTemplate, ca, leafPub, key)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(leafDER)
	if err != nil {
		t.Fatal(err)
	}
	clientCertificate := tls.Certificate{Certificate: [][]byte{leafDER, der}, PrivateKey: leafKey, Leaf: leaf}
	roots := x509.NewCertPool()
	roots.AddCert(ca)
	server := httptest.NewUnstartedServer(service.Handler())
	server.TLS = serverPKI.ServerTLS()
	server.TLS.ClientCAs = roots
	server.StartTLS()
	defer server.Close()
	cfg := &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: serverPKI.Roots, Certificates: []tls.Certificate{clientCertificate}}
	transport := &http.Transport{TLSClientConfig: cfg}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
	first, err := client.Get(server.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, first.Body)
	first.Body.Close()
	if first.StatusCode != http.StatusOK {
		t.Fatal(first.StatusCode)
	}
	time.Sleep(time.Until(leaf.NotAfter) + 100*time.Millisecond)
	if time.Now().Before(leaf.NotAfter) {
		t.Fatal("certificate not expired")
	}
	raw, _ := json.Marshal(publishRequest{Plan: plan, Identity: testDispatchIdentity(plan)})
	req, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/publish", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	reused := false
	req = req.WithContext(httptrace.WithClientTrace(req.Context(), &httptrace.ClientTrace{GotConn: func(info httptrace.GotConnInfo) { reused = info.Reused }}))
	response, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if !reused {
		t.Fatal("test did not reuse the previously authenticated connection")
	}
	if response.StatusCode != http.StatusUnauthorized || service.remote.(*serviceRemote).publishCalls != 0 {
		t.Fatalf("expired connection authorized publication: HTTP %d %s", response.StatusCode, body)
	}
	freshTransport := &http.Transport{TLSClientConfig: cfg.Clone()}
	defer freshTransport.CloseIdleConnections()
	fresh := &http.Client{Transport: freshTransport, Timeout: 5 * time.Second}
	rejected, err := fresh.Get(server.URL + "/healthz")
	if rejected != nil {
		rejected.Body.Close()
	}
	if err == nil {
		t.Fatal("new TLS connection incorrectly accepted expired certificate")
	}
}
