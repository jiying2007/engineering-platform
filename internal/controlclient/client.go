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

// ErrCommitOutcomeUnknown never authorizes automatic retries. A control
// caller must perform exact immutable ID/epoch/idempotency/audit readback.
var ErrCommitOutcomeUnknown = errors.New("control COMMIT outcome unknown; exact readback required")

// ErrPostOutcomeUnobserved means a mutation request did not yield a usable
// success/error response. Transport failure, partial success bytes or a
// malformed success reply are not evidence that a POST was rolled back.
// Unlike ErrCommitOutcomeUnknown, the server may never have attempted COMMIT.
var ErrPostOutcomeUnobserved = errors.New("control POST outcome unobserved; exact readback required")

// ErrExternalEffectUnresolved means the Action Gateway has already called or
// observed an external Provider, but its original settlement is unresolved.
// This does not imply COMMIT failure or grant replay of the external effect.
var ErrExternalEffectUnresolved = errors.New("control external Action effect unresolved; exact readback required")

type postOutcomeUnobservedError struct{ cause error }

func (e *postOutcomeUnobservedError) Error() string {
	return "control POST outcome unobserved: read back exact original resource, epoch and audit before any retry"
}
func (e *postOutcomeUnobservedError) Unwrap() error { return e.cause }
func (e *postOutcomeUnobservedError) Is(target error) bool {
	return target == ErrPostOutcomeUnobserved
}

// This wrapper intentionally hides original transport/decoder diagnostics from
// Error(), while preserving errors.Is(context.Canceled/DeadlineExceeded) for
// Worker cancellation and cleanup. It never attempts another HTTP request.
func classifyUnobservedPost(method string, err error) error {
	if err == nil || method != http.MethodPost {
		return err
	}
	return &postOutcomeUnobservedError{cause: err}
}

type HTTPError struct {
	Status                   int
	commitOutcomeUnknown     bool
	externalEffectUnresolved bool
}

func (e *HTTPError) Error() string {
	if e != nil && e.commitOutcomeUnknown {
		return "control API returned HTTP 503 commit_outcome_unknown: read back exact resource identity, epoch and audit before any retry"
	}
	if e != nil && e.externalEffectUnresolved {
		return "control API returned HTTP 503 external_effect_unresolved: read back original Action operation and external observation before any retry"
	}
	if e == nil {
		return "nil control HTTP error"
	}
	return fmt.Sprintf("control API returned HTTP %d", e.Status)
}

// Preserve errors.As(*HTTPError) and Status while providing a typed,
// non-retrying signal. Upstream error text is never included in Error().
func (e *HTTPError) Is(target error) bool {
	return e != nil && ((e.commitOutcomeUnknown && target == ErrCommitOutcomeUnknown) ||
		(e.externalEffectUnresolved && target == ErrExternalEffectUnresolved))
}

// Only the Control API's bounded JSON error code may be surfaced. In
// particular, never copy SQL, tokens or attacker-controlled response bodies
// into caller-visible diagnostics.
func classifyControlHTTPError(response *http.Response) *HTTPError {
	result := &HTTPError{Status: response.StatusCode}
	if response.StatusCode != http.StatusServiceUnavailable ||
		response.Header.Get("Content-Encoding") != "" {
		return result
	}
	contentType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || contentType != "application/json" {
		return result
	}
	const maxErrorBytes = 1024
	data, err := io.ReadAll(io.LimitReader(response.Body, maxErrorBytes+1))
	if err != nil || len(data) > maxErrorBytes {
		return result
	}
	var payload struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(data, &payload) == nil {
		switch {
		case strings.HasPrefix(payload.Error, "commit_outcome_unknown: read back"):
			result.commitOutcomeUnknown = true
		case strings.HasPrefix(payload.Error, "external_effect_unresolved: read back"):
			result.externalEffectUnresolved = true
		}
	}
	return result
}

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
		if method == http.MethodPost {
			return nil, classifyUnobservedPost(method, err)
		}
		return nil, fmt.Errorf("control transport: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, classifyControlHTTPError(response)
	}
	contentType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || contentType != "application/json" || response.Header.Get("Content-Encoding") != "" {
		return nil, classifyUnobservedPost(method, fmt.Errorf("unencoded JSON response required"))
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, MaxResponse+1))
	if err != nil {
		return nil, classifyUnobservedPost(method, err)
	}
	if len(data) > MaxResponse {
		return nil, classifyUnobservedPost(method, fmt.Errorf("control response exceeds limit"))
	}
	if !json.Valid(data) {
		return nil, classifyUnobservedPost(method, fmt.Errorf("invalid control JSON response"))
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
		return classifyUnobservedPost(method, err)
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return classifyUnobservedPost(method, fmt.Errorf("trailing control JSON"))
	}
	return nil
}
