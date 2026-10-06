package githubpublish

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jiying2007/engineering-platform/internal/access"
	"github.com/jiying2007/engineering-platform/internal/strictjson"
)

type RemoteConfiguration struct {
	Version        int    `json:"version"`
	Endpoint       string `json:"endpoint"`
	ClientCertFile string `json:"client_cert_file"`
	ClientKeyFile  string `json:"client_key_file"`
	ServerCAFile   string `json:"server_ca_file"`
}

type remoteClient struct {
	endpoint string
	client   *http.Client
}

type PublisherHealthObservation struct {
	Version               int       `json:"version"`
	Status                string    `json:"status"`
	Service               string    `json:"service"`
	ObservedAt            time.Time `json:"observed_at"`
	EndpointObserved      bool      `json:"endpoint_observed"`
	UpstreamObserved      bool      `json:"upstream_observed"`
	CapacityObserved      bool      `json:"capacity_observed"`
	PublicationAuthorized bool      `json:"publication_authorized"`
	ExecutionAuthorized   bool      `json:"execution_authorized"`
	ProductionQualified   bool      `json:"production_qualified"`
}

type publisherHealthPayload struct {
	Status  string `json:"status"`
	Service string `json:"service"`
}

func readRemoteConfiguration(path string) (RemoteConfiguration, error) {
	var config RemoteConfiguration
	data, err := access.ReadConfiguration(path, false)
	if err != nil {
		return config, err
	}
	if err := strictjson.Decode(data, &config); err != nil {
		return config, err
	}
	return config, nil
}

func LoadRemoteClient(path string) (Remote, error) {
	config, err := readRemoteConfiguration(path)
	if err != nil {
		return nil, err
	}
	return NewRemoteClient(config)
}

func LoadRemoteHealth(ctx context.Context, path string) (PublisherHealthObservation, error) {
	config, err := readRemoteConfiguration(path)
	if err != nil {
		return PublisherHealthObservation{}, err
	}
	return ProbeRemoteHealth(ctx, config)
}

func NewRemoteClient(config RemoteConfiguration) (Remote, error) {
	return newRemoteClient(config)
}

func newRemoteClient(config RemoteConfiguration) (*remoteClient, error) {
	if config.Version != 1 {
		return nil, fmt.Errorf("publisher remote configuration version 1 required")
	}
	u, err := url.Parse(config.Endpoint)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil ||
		u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" || (u.Path != "" && u.Path != "/") {
		return nil, fmt.Errorf("publisher remote requires explicit root HTTPS endpoint")
	}
	certPEM, err := access.ReadConfiguration(config.ClientCertFile, false)
	if err != nil {
		return nil, err
	}
	keyPEM, err := access.ReadConfiguration(config.ClientKeyFile, true)
	if err != nil {
		return nil, err
	}
	pair, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("invalid publisher remote client certificate/key")
	}
	caPEM, err := access.ReadConfiguration(config.ServerCAFile, false)
	if err != nil {
		return nil, err
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(caPEM) {
		return nil, fmt.Errorf("invalid publisher remote server CA")
	}
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS13, ServerName: u.Hostname(),
		RootCAs: roots, Certificates: []tls.Certificate{pair},
	}
	transport := &http.Transport{
		TLSClientConfig: tlsConfig, Proxy: nil,
		DialContext:         (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: 15 * time.Second,
		IdleConnTimeout: 30 * time.Second, MaxIdleConns: 2, MaxConnsPerHost: 2,
		MaxResponseHeaderBytes: 32 << 10, DisableCompression: true,
	}
	client := &http.Client{
		Transport: transport, Timeout: 30 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	return &remoteClient{endpoint: strings.TrimSuffix(config.Endpoint, "/"), client: client}, nil
}

func ProbeRemoteHealth(ctx context.Context, config RemoteConfiguration) (PublisherHealthObservation, error) {
	client, err := newRemoteClient(config)
	if err != nil {
		return PublisherHealthObservation{}, err
	}
	defer client.client.CloseIdleConnections()
	return client.health(ctx)
}

func (c *remoteClient) health(ctx context.Context) (PublisherHealthObservation, error) {
	var zero PublisherHealthObservation
	if c == nil || c.client == nil || c.endpoint == "" {
		return zero, fmt.Errorf("configured publisher remote required")
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint+"/healthz", nil)
	if err != nil {
		return zero, err
	}
	req.Header.Set("Accept", "application/json")
	response, err := c.client.Do(req)
	if err != nil {
		return zero, fmt.Errorf("publisher health transport: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return zero, fmt.Errorf("publisher health returned HTTP %d", response.StatusCode)
	}
	mediaType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" || response.Header.Get("Content-Encoding") != "" {
		return zero, fmt.Errorf("publisher health requires unencoded JSON")
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, (4<<10)+1))
	if err != nil || len(body) == 0 || len(body) > 4<<10 {
		return zero, fmt.Errorf("publisher health response outside size limit")
	}
	var payload publisherHealthPayload
	if err := strictjson.Decode(body, &payload); err != nil ||
		payload.Status != "ok" || payload.Service != "engineering-github-publisher" {
		return zero, fmt.Errorf("publisher health identity mismatch")
	}
	return PublisherHealthObservation{
		Version: 1, Status: "PUBLISHER_ENDPOINT_OBSERVED",
		Service: payload.Service, ObservedAt: time.Now().UTC(), EndpointObserved: true,
	}, nil
}

func (c *remoteClient) Publish(ctx context.Context, plan Plan, bundlePath string) (PublicationReceipt, error) {
	var receipt PublicationReceipt
	if c == nil || c.client == nil || plan.Validate() != nil || bundlePath == "" {
		return receipt, fmt.Errorf("valid publisher remote client request required")
	}
	if err := c.call(ctx, "/v1/publish", publishRequest{Plan: plan}, &receipt); err != nil {
		return receipt, err
	}
	if err := receipt.Validate(plan); err != nil {
		return PublicationReceipt{}, err
	}
	return receipt, nil
}

func (c *remoteClient) Observe(ctx context.Context, plan Plan) (ObserveResult, error) {
	var result ObserveResult
	if c == nil || c.client == nil || plan.Validate() != nil {
		return result, fmt.Errorf("valid publisher remote client request required")
	}
	if err := c.call(ctx, "/v1/observe", observeRequest{Plan: plan}, &result); err != nil {
		return result, err
	}
	switch result.Outcome {
	case ObservedAbsent, ObservedConfirmed, ObservedPartial, ObservedConflict:
	default:
		return ObserveResult{}, fmt.Errorf("publisher remote returned invalid observation")
	}
	if result.Outcome == ObservedConfirmed && result.Receipt.Validate(plan) != nil {
		return ObserveResult{}, fmt.Errorf("publisher remote returned invalid receipt")
	}
	return result, nil
}

func (c *remoteClient) call(ctx context.Context, path string, input, output any) error {
	data, err := json.Marshal(input)
	if err != nil || len(data) == 0 || len(data) > 1<<20 {
		return fmt.Errorf("publisher remote request outside size limit")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	response, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("publisher remote transport: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("publisher remote returned HTTP %d", response.StatusCode)
	}
	mediaType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" || response.Header.Get("Content-Encoding") != "" {
		return fmt.Errorf("publisher remote requires unencoded JSON response")
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil || len(body) == 0 || len(body) > 1<<20 {
		return fmt.Errorf("publisher remote response outside size limit")
	}
	if err := strictjson.Decode(body, output); err != nil {
		return fmt.Errorf("publisher remote returned invalid JSON")
	}
	return nil
}
