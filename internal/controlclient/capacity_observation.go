package controlclient

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"mime"
	"net/http"
	"time"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/strictjson"
)

type CapacityRead struct {
	Body                  []byte
	PeerCertificateDigest string
	RequestedAt           time.Time
	ReceivedAt            time.Time
}

func capacityPeerDigest(state *tls.ConnectionState, expected string, now time.Time) (string, error) {
	if state == nil || !state.HandshakeComplete || state.Version < tls.VersionTLS13 ||
		len(state.PeerCertificates) == 0 || state.PeerCertificates[0] == nil || len(state.VerifiedChains) == 0 {
		return "", fmt.Errorf("verified capacity source TLS peer required")
	}
	leaf := state.PeerCertificates[0]
	digest := canonical.BytesDigest(leaf.Raw)
	if !canonical.ValidDigest(expected) || digest != expected {
		return "", fmt.Errorf("capacity source certificate differs from expected peer")
	}
	// A retained TLS connection cannot extend the life of a leaf or issuer.
	for _, chain := range state.VerifiedChains {
		if len(chain) == 0 || chain[0] == nil || !bytes.Equal(chain[0].Raw, leaf.Raw) {
			continue
		}
		valid := true
		for _, cert := range chain {
			if cert == nil || now.Before(cert.NotBefore) || !now.Before(cert.NotAfter) {
				valid = false
				break
			}
		}
		if valid {
			return digest, nil
		}
	}
	return "", fmt.Errorf("capacity source certificate chain expired or not yet valid")
}

// ReadCapacity makes exactly the existing read-capability GET through this
// direct-mTLS client. The peer digest comes from the verified TLS connection,
// never a body/header assertion. No mutation or retry policy is introduced.
func (c *Client) ReadCapacity(ctx context.Context, expectedCertificateDigest string) (CapacityRead, error) {
	var zero CapacityRead
	if c == nil || c.http == nil || !canonical.ValidDigest(expectedCertificateDigest) {
		return zero, fmt.Errorf("configured Control client and expected certificate digest required")
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint+"/api/v1/operations/capacity", nil)
	if err != nil {
		return zero, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Cache-Control", "no-cache")
	requested := time.Now().UTC()
	response, err := c.http.Do(req)
	if err != nil {
		return zero, &controlResponseError{cause: err}
	}
	if _, err := capacityPeerDigest(response.TLS, expectedCertificateDigest, requested); err != nil {
		_ = response.Body.Close()
		return zero, err
	}
	limit := int64(MaxResponse)
	if response.StatusCode != http.StatusOK {
		limit = 1024
	}
	raw, readErr := readControlResponseBody(ctx, response, limit)
	if response.StatusCode != http.StatusOK {
		return zero, classifyResponseHTTPError(http.MethodGet, response, raw, readErr)
	}
	if readErr != nil {
		return zero, readErr
	}
	contentType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || contentType != "application/json" || response.Header.Get("Content-Encoding") != "" || response.Header.Get("Cache-Control") != "no-store" {
		return zero, fmt.Errorf("uncached unencoded capacity JSON required")
	}
	if strictjson.ValidateForeignObjectLimit(raw, MaxResponse) != nil {
		return zero, fmt.Errorf("unambiguous bounded capacity response required")
	}
	received := time.Now().UTC()
	digest, err := capacityPeerDigest(response.TLS, expectedCertificateDigest, received)
	if err != nil {
		return zero, err
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	return CapacityRead{Body: raw, PeerCertificateDigest: digest, RequestedAt: requested, ReceivedAt: received}, nil
}
