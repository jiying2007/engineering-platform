package controlclient

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/canonical"
)

func TestCapacityClientReadsActualPinnedTLSCertificate(t *testing.T) {
	pki := newPKI(t)
	pin := canonical.BytesDigest(pki.server.Certificate[0])
	body := []byte("{\"retained\":true}\n")
	var calls atomic.Int64
	server := testServer(t, pki, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/operations/capacity" || r.Header.Get("Accept") != "application/json" {
			t.Error("unexpected capacity request")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Peer-Certificate-Digest", canonical.BytesDigest([]byte("forged header")))
		w.Write(body)
	}))
	client, err := New(server.URL, pki.clientTLS())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	read, err := client.ReadCapacity(context.Background(), pin)
	if err != nil || !bytes.Equal(read.Body, body) || read.PeerCertificateDigest != pin || read.RequestedAt.IsZero() || read.ReceivedAt.Before(read.RequestedAt) {
		t.Fatalf("pinned read failed: %#v %v", read, err)
	}
	if got, err := client.ReadCapacity(context.Background(), canonical.BytesDigest([]byte("wrong expected peer"))); err == nil || !reflect.DeepEqual(got, CapacityRead{}) {
		t.Fatal("body/headers substituted for actual TLS certificate")
	}
	if calls.Load() != 2 {
		t.Fatal("capacity GET or rejected peer retried")
	}
	if _, err := client.ReadCapacity(context.Background(), ""); err == nil || calls.Load() != 2 {
		t.Fatal("missing external pin caused a request")
	}
}

func TestCapacityClientRejectsPartialEncodedAmbiguousAndRedirectedReads(t *testing.T) {
	for _, mode := range []string{"partial", "encoded", "missing-no-store", "duplicate", "redirect", "denied", "oversize"} {
		t.Run(mode, func(t *testing.T) {
			pki := newPKI(t)
			var calls atomic.Int64
			server := testServer(t, pki, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Cache-Control", "no-store")
				switch mode {
				case "partial":
					w.Header().Set("Content-Length", "100")
					io.WriteString(w, `{"partial":true}`)
				case "encoded":
					w.Header().Set("Content-Encoding", "gzip")
					io.WriteString(w, `{}`)
				case "missing-no-store":
					w.Header().Del("Cache-Control")
					io.WriteString(w, `{}`)
				case "duplicate":
					io.WriteString(w, `{"count":0,"count":1}`)
				case "redirect":
					w.Header().Set("Location", "/api/v1/other")
					w.WriteHeader(http.StatusTemporaryRedirect)
					io.WriteString(w, `{}`)
				case "denied":
					w.WriteHeader(http.StatusForbidden)
					io.WriteString(w, `{"error":"denied"}`)
				case "oversize":
					io.WriteString(w, `{"data":"`+strings.Repeat("x", MaxResponse)+`"}`)
				}
			}))
			client, err := New(server.URL, pki.clientTLS())
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			read, err := client.ReadCapacity(context.Background(), canonical.BytesDigest(pki.server.Certificate[0]))
			if err == nil || !reflect.DeepEqual(read, CapacityRead{}) || calls.Load() != 1 {
				t.Fatalf("incomplete capacity read accepted or retried: %#v %v / %d", read, err, calls.Load())
			}
		})
	}
}

func TestCapacityPeerRechecksCurrentEntireChainOnReusedConnection(t *testing.T) {
	pki := newPKI(t)
	leaf, err := x509.ParseCertificate(pki.server.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	issuer := *leaf
	issuer.Raw = []byte("test issuer")
	now := time.Now()
	state := &tls.ConnectionState{Version: tls.VersionTLS13, HandshakeComplete: true, PeerCertificates: []*x509.Certificate{leaf}, VerifiedChains: [][]*x509.Certificate{{leaf, &issuer}}}
	pin := canonical.BytesDigest(leaf.Raw)
	if _, err := capacityPeerDigest(state, pin, now); err != nil {
		t.Fatal(err)
	}
	issuer.NotAfter = now
	if _, err := capacityPeerDigest(state, pin, now); err == nil {
		t.Fatal("expired issuer extended reused-connection authority")
	}
	issuer.NotAfter = now.Add(time.Hour)
	leaf.NotAfter = now
	if _, err := capacityPeerDigest(state, pin, now); err == nil {
		t.Fatal("expired leaf extended reused-connection authority")
	}
}

func TestCapacityClientCancellationRetainsNoPartialObservation(t *testing.T) {
	pki := newPKI(t)
	entered := make(chan struct{})
	var calls atomic.Int64
	server := testServer(t, pki, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		io.WriteString(w, `{"partial":`)
		w.(http.Flusher).Flush()
		close(entered)
		<-r.Context().Done()
	}))
	client, err := New(server.URL, pki.clientTLS())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		got, err := client.ReadCapacity(ctx, canonical.BytesDigest(pki.server.Certificate[0]))
		if !reflect.DeepEqual(got, CapacityRead{}) {
			t.Error("partial observation retained")
		}
		done <- err
	}()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("request did not arrive")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal("cancellation lost", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("body read ignored cancellation")
	}
	if calls.Load() != 1 {
		t.Fatal("cancelled GET retried")
	}
}
