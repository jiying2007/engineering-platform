// Package controlclient is the bounded, direct-mTLS front door shared by CLI
// and Worker. No proxy inheritance, redirects, insecure TLS or automatic POST
// retries are allowed. It contains no provider or database credentials.
package controlclient

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"
)

const MaxRequest = 1 << 20
const MaxResponse = 4 << 20

type HTTPError struct{ Status int }

func (e *HTTPError) Error() string { return fmt.Sprintf("control API returned HTTP %d", e.Status) }

type Client struct {
	endpoint, subject string
	http              *http.Client
}

func (c *Client) Subject() string { return c.subject }
func (c *Client) Close()          { c.http.CloseIdleConnections() }
func New(endpoint string, config *tls.Config) (*Client, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.RawPath != "" || (u.Path != "" && u.Path != "/") || u.Opaque != "" {
		return nil, fmt.Errorf("explicit root HTTPS endpoint required")
	}
	if config == nil || config.InsecureSkipVerify || config.RootCAs == nil || len(config.Certificates) != 1 || len(config.Certificates[0].Certificate) == 0 || config.GetClientCertificate != nil || config.VerifyConnection != nil || config.VerifyPeerCertificate != nil {
		return nil, fmt.Errorf("explicit CA and one fixed client certificate required")
	}
	if config.MaxVersion != 0 && config.MaxVersion < tls.VersionTLS13 {
		return nil, fmt.Errorf("TLS 1.3 required")
	}
	cert, err := x509.ParseCertificate(config.Certificates[0].Certificate[0])
	if err != nil {
		return nil, err
	}
	now := time.Now()
	if now.Before(cert.NotBefore) || !now.Before(cert.NotAfter) || len(cert.URIs) != 1 || !strings.HasPrefix(cert.URIs[0].String(), "urn:engineering-platform:") {
		return nil, fmt.Errorf("current client URI identity required")
	}
	purpose := false
	for _, usage := range cert.ExtKeyUsage {
		if usage == x509.ExtKeyUsageClientAuth {
			purpose = true
		}
	}
	if !purpose {
		return nil, fmt.Errorf("explicit clientAuth usage required")
	}
	// Rebuild rather than Clone: do not inherit custom Time, key logging,
	// callbacks, renegotiation or a ServerName overriding the endpoint identity.
	pair := config.Certificates[0]
	pair.Certificate = make([][]byte, len(config.Certificates[0].Certificate))
	for i, der := range config.Certificates[0].Certificate {
		pair.Certificate[i] = bytes.Clone(der)
	}
	pair.Leaf = nil
	pair.OCSPStaple = bytes.Clone(pair.OCSPStaple)
	pair.SignedCertificateTimestamps = nil
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS13, ServerName: u.Hostname(), RootCAs: config.RootCAs.Clone(), Certificates: []tls.Certificate{pair}}
	transport := &http.Transport{TLSClientConfig: tlsConfig, Proxy: nil, DialContext: (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext, TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: 10 * time.Second, IdleConnTimeout: 30 * time.Second, MaxIdleConns: 4, MaxConnsPerHost: 4, MaxResponseHeaderBytes: 32 << 10, DisableCompression: true}
	client := &http.Client{Transport: transport, Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return &Client{endpoint: strings.TrimSuffix(endpoint, "/"), subject: cert.URIs[0].String(), http: client}, nil
}
func FromEnvironment(env func(string) string) (*Client, error) {
	endpoint, certFile, keyFile, caFile := env("CONTROL_ENDPOINT"), env("CONTROL_CLIENT_CERT_FILE"), env("CONTROL_CLIENT_KEY_FILE"), env("CONTROL_SERVER_CA_FILE")
	if endpoint == "" || certFile == "" || keyFile == "" || caFile == "" {
		return nil, fmt.Errorf("CONTROL_ENDPOINT/client certificate/key/server CA must be explicit")
	}
	certPEM, err := readTLSFile(certFile, false)
	if err != nil {
		return nil, err
	}
	keyPEM, err := readTLSFile(keyFile, true)
	if err != nil {
		return nil, err
	}
	pair, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("invalid client certificate/key")
	}
	caPEM, err := readTLSFile(caFile, false)
	if err != nil {
		return nil, err
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(caPEM) {
		return nil, fmt.Errorf("invalid server CA")
	}
	return New(endpoint, &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: roots, Certificates: []tls.Certificate{pair}})
}
func readTLSFile(name string, secret bool) ([]byte, error) {
	before, err := os.Lstat(name)
	if err != nil {
		return nil, fmt.Errorf("TLS input unavailable")
	}
	if !before.Mode().IsRegular() || before.Mode().Perm()&0o022 != 0 || (secret && before.Mode().Perm()&0o077 != 0) {
		return nil, fmt.Errorf("unsafe TLS input permissions")
	}
	f, err := os.Open(name)
	if err != nil {
		return nil, fmt.Errorf("TLS input unavailable")
	}
	defer f.Close()
	after, err := f.Stat()
	if err != nil || !os.SameFile(before, after) {
		return nil, fmt.Errorf("TLS input changed")
	}
	data, err := io.ReadAll(io.LimitReader(f, MaxRequest+1))
	if err != nil || len(data) > MaxRequest {
		return nil, fmt.Errorf("TLS input read failed or exceeds limit")
	}
	return data, nil
}
func validPath(p string) bool {
	return (p == "/healthz" || strings.HasPrefix(p, "/api/v1/")) && path.Clean(p) == p && !strings.ContainsAny(p, "%?#\\\r\n") && !strings.Contains(p, "//")
}
func (c *Client) Raw(ctx context.Context, method, p string, body []byte) ([]byte, error) {
	if c == nil || c.http == nil || !validPath(p) || (method != http.MethodGet && method != http.MethodPost) {
		return nil, fmt.Errorf("invalid control request")
	}
	if len(body) > MaxRequest || (method == http.MethodGet && len(body) != 0) || (method == http.MethodPost && (!json.Valid(body) || len(bytes.TrimSpace(body)) == 0 || bytes.TrimSpace(body)[0] != '{')) {
		return nil, fmt.Errorf("bounded JSON object required")
	}
	req, err := http.NewRequestWithContext(ctx, method, c.endpoint+p, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/json")
	}
	response, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("control transport: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, &HTTPError{Status: response.StatusCode}
	}
	contentType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || contentType != "application/json" || response.Header.Get("Content-Encoding") != "" {
		return nil, fmt.Errorf("unencoded JSON response required")
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, MaxResponse+1))
	if err != nil {
		return nil, err
	}
	if len(data) > MaxResponse {
		return nil, fmt.Errorf("control response exceeds limit")
	}
	if !json.Valid(data) {
		return nil, fmt.Errorf("invalid control JSON response")
	}
	return data, nil
}
func (c *Client) Call(ctx context.Context, method, p string, in, out any) error {
	var body []byte
	var err error
	if in != nil {
		body, err = json.Marshal(in)
		if err != nil {
			return err
		}
	}
	data, err := c.Raw(ctx, method, p, body)
	if err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return fmt.Errorf("trailing control JSON")
	}
	return nil
}
