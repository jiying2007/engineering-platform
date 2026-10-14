package githubpublish

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jiying2007/engineering-platform/internal/testsupport"
)

type remoteResponseRecord struct {
	Value string `json:"value"`
	Count int    `json:"count"`
}

func remoteResponseProbe(body io.ReadCloser, status int, header http.Header, calls *int) *remoteClient {
	return &remoteClient{endpoint: "https://publisher.invalid", client: &http.Client{
		Transport: responseProbeTransport(func(*http.Request) (*http.Response, error) {
			(*calls)++
			return &http.Response{StatusCode: status, Body: body, Header: header}, nil
		}),
	}}
}

func remoteResponseHeader() http.Header {
	return http.Header{"Content-Type": []string{"application/json"}}
}

func TestPublisherRemoteResponseRejectsIncompleteBody(t *testing.T) {
	for _, path := range []string{"publish", "observe", "dispatch-evidence", "dispatch-no-invocation"} {
		for _, kind := range []string{"read", "close", "read-and-close", "cancel-read", "cancel-close", "oversize"} {
			t.Run(path+"-"+kind, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				cause := errors.New("private-body-diagnostic")
				closeCause := errors.New("private-close-diagnostic")
				body := &responseProbeBody{reader: strings.NewReader(`{"value":"new","count":2}`)}
				switch kind {
				case "read":
					body.readErr = cause
				case "close":
					body.closeErr = cause
				case "read-and-close":
					body.readErr, body.closeErr = cause, closeCause
				case "cancel-read":
					body.onRead, cause = cancel, context.Canceled
				case "cancel-close":
					body.onClose, cause = cancel, context.Canceled
				case "oversize":
					body.reader = strings.NewReader(strings.Repeat("x", (1<<20)+1))
				}
				calls := 0
				client := remoteResponseProbe(body, http.StatusOK, remoteResponseHeader(), &calls)
				before := remoteResponseRecord{Value: "retained", Count: 1}
				out := before
				err := client.call(ctx, "/v1/"+path, struct{}{}, &out)
				if err == nil || out != before {
					t.Fatalf("incomplete Publisher response accepted or mutated output: %+v %v", out, err)
				}
				if kind != "oversize" && !errors.Is(err, cause) {
					t.Fatalf("original Publisher response cause lost: %v", err)
				}
				if kind == "read-and-close" && !errors.Is(err, closeCause) {
					t.Fatal("simultaneous close failure lost")
				}
				if strings.Contains(err.Error(), "private-") || calls != 1 || body.closed != 1 {
					t.Fatalf("response leaked diagnostics, retried or failed close: %v %d/%d", err, calls, body.closed)
				}
			})
		}
	}
}

func TestPublisherRemoteResponseAtomicDecode(t *testing.T) {
	for _, kind := range []string{"valid", "type-error", "unknown-field", "duplicate", "trailing", "null", "empty", "status", "media", "encoding"} {
		t.Run(kind, func(t *testing.T) {
			raw := `{"value":"new","count":2}`
			status, header := http.StatusCreated, remoteResponseHeader()
			switch kind {
			case "type-error":
				raw = `{"value":"new","count":"wrong"}`
			case "unknown-field":
				raw = `{"value":"new","count":2,"unknown":true}`
			case "duplicate":
				raw = `{"value":"new","value":"other","count":2}`
			case "trailing":
				raw += `{}`
			case "null":
				raw = `null`
			case "empty":
				raw = ""
			case "status":
				status = http.StatusServiceUnavailable
			case "media":
				header.Set("Content-Type", "text/plain")
			case "encoding":
				header.Set("Content-Encoding", "gzip")
			}
			calls := 0
			body := &responseProbeBody{reader: strings.NewReader(raw)}
			client := remoteResponseProbe(body, status, header, &calls)
			before := remoteResponseRecord{Value: "retained", Count: 1}
			out := before
			err := client.call(context.Background(), "/v1/observe", struct{}{}, &out)
			if kind == "valid" {
				if err != nil || out != (remoteResponseRecord{Value: "new", Count: 2}) {
					t.Fatalf("valid internal JSON response rejected: %+v %v", out, err)
				}
			} else if err == nil || out != before {
				t.Fatalf("failed Publisher decode replaced retained output: %+v %v", out, err)
			}
			if calls != 1 || body.closed != 1 {
				t.Fatal("decode failure repeated exchange or lost body close")
			}
		})
	}
}

type remoteResponseMarshal struct {
	cancel context.CancelFunc
	calls  *int
}

func (m remoteResponseMarshal) MarshalJSON() ([]byte, error) {
	(*m.calls)++
	m.cancel()
	return []byte(`{}`), nil
}

func TestPublisherRemoteResponseCancelledBeforeExchange(t *testing.T) {
	for _, preCancelled := range []bool{false, true} {
		t.Run(fmt.Sprint(preCancelled), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if preCancelled {
				cancel()
			}
			calls, marshals := 0, 0
			body := &responseProbeBody{reader: strings.NewReader(`{}`)}
			client := remoteResponseProbe(body, http.StatusOK, remoteResponseHeader(), &calls)
			var out remoteResponseRecord
			err := client.call(ctx, "/v1/publish", remoteResponseMarshal{cancel: cancel, calls: &marshals}, &out)
			if !errors.Is(err, context.Canceled) || calls != 0 || body.closed != 0 || (preCancelled && marshals != 0) {
				t.Fatalf("cancelled Publisher request initiated work: err=%v calls=%d marshals=%d closed=%d", err, calls, marshals, body.closed)
			}
		})
	}
}

func TestPublisherRemoteHealthCompleteObservation(t *testing.T) {
	for _, kind := range []string{"valid", "read", "close", "cancel-read", "cancel-close", "oversize", "wrong-status", "wrong-media"} {
		t.Run(kind, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			cause := errors.New("private-health-diagnostic")
			body := &responseProbeBody{reader: strings.NewReader(`{"status":"ok","service":"engineering-github-publisher"}`)}
			status, header := http.StatusOK, remoteResponseHeader()
			switch kind {
			case "read":
				body.readErr = cause
			case "close":
				body.closeErr = cause
			case "cancel-read":
				body.onRead, cause = cancel, context.Canceled
			case "cancel-close":
				body.onClose, cause = cancel, context.Canceled
			case "oversize":
				body.reader = strings.NewReader(strings.Repeat("x", (4<<10)+1))
			case "wrong-status":
				status = http.StatusCreated
			case "wrong-media":
				header.Set("Content-Type", "text/plain")
			}
			calls := 0
			client := remoteResponseProbe(body, status, header, &calls)
			out, err := client.health(ctx, "sha256:"+strings.Repeat("a", 64))
			if kind == "valid" {
				if err != nil || !out.EndpointObserved || out.ProductionQualified || out.PublicationAuthorized || out.UpstreamObserved || out.CapacityObserved || out.ExecutionAuthorized {
					t.Fatalf("health observation expanded authority: %+v %v", out, err)
				}
			} else if err == nil || out != (PublisherHealthObservation{}) {
				t.Fatalf("incomplete health response became observation: %+v %v", out, err)
			}
			if kind == "read" || kind == "close" || strings.HasPrefix(kind, "cancel-") {
				if !errors.Is(err, cause) || strings.Contains(err.Error(), "private-") {
					t.Fatalf("health failure identity lost or leaked: %v", err)
				}
			}
			if calls != 1 || body.closed != 1 {
				t.Fatal("health exchange repeated or body not closed")
			}
		})
	}
}

func TestPublisherRemoteResponseTransportFailure(t *testing.T) {
	for _, health := range []bool{false, true} {
		t.Run(fmt.Sprint(health), func(t *testing.T) {
			cause := errors.New("private-transport-diagnostic")
			calls := 0
			client := &remoteClient{endpoint: "https://publisher.invalid", client: &http.Client{
				Transport: responseProbeTransport(func(*http.Request) (*http.Response, error) {
					calls++
					return nil, cause
				}),
			}}
			var err error
			if health {
				_, err = client.health(context.Background(), "sha256:"+strings.Repeat("a", 64))
			} else {
				err = client.call(context.Background(), "/v1/publish", struct{}{}, &remoteResponseRecord{})
			}
			if !errors.Is(err, cause) || strings.Contains(err.Error(), "private-") || calls != 1 {
				t.Fatalf("transport error lost, disclosed or retried: %v calls=%d", err, calls)
			}
		})
	}
}

func TestPublisherRemoteResponsePublicErrorsHaveNoReceipt(t *testing.T) {
	plan, _ := historicalPullProbePlan(t)
	for _, observe := range []bool{false, true} {
		t.Run(fmt.Sprint(observe), func(t *testing.T) {
			var input any = boundPublicationReceipt(plan, "CREATED")
			if observe {
				input = ObserveResult{Outcome: ObservedConfirmed, Receipt: boundPublicationReceipt(plan, "OBSERVED")}
			}
			raw, err := json.Marshal(input)
			if err != nil {
				t.Fatal(err)
			}
			bad := strings.Replace(string(raw), `"version":1`, `"version":"wrong"`, 1)
			if bad == string(raw) {
				t.Fatal("negative fixture did not alter receipt")
			}
			calls := 0
			body := &responseProbeBody{reader: strings.NewReader(bad)}
			client := remoteResponseProbe(body, http.StatusOK, remoteResponseHeader(), &calls)
			if observe {
				got, err := client.Observe(context.Background(), plan)
				if err == nil || !reflect.DeepEqual(got, ObserveResult{}) {
					t.Fatalf("failed public observation returned partial receipt: %+v %v", got, err)
				}
			} else {
				got, err := client.Publish(context.Background(), plan, "unused-local-bundle", testDispatchIdentity(plan))
				if err == nil || got != (PublicationReceipt{}) {
					t.Fatalf("failed public publication returned partial receipt: %+v %v", got, err)
				}
			}
			if calls != 1 || body.closed != 1 {
				t.Fatal("public method retried or failed to close")
			}
		})
	}
}

func TestPublisherRemoteResponseRealMTLS(t *testing.T) {
	plan, _ := historicalPullProbePlan(t)
	for _, truncated := range []bool{false, true} {
		t.Run(fmt.Sprint(truncated), func(t *testing.T) {
			pki := testsupport.NewPKI(t)
			var calls atomic.Int32
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				var payload any
				switch r.URL.Path {
				case "/healthz":
					payload = publisherHealthPayload{Status: "ok", Service: "engineering-github-publisher"}
				case "/v1/publish":
					payload = boundPublicationReceipt(plan, "CREATED")
				case "/v1/observe":
					payload = ObserveResult{Outcome: ObservedConfirmed, Receipt: boundPublicationReceipt(plan, "OBSERVED")}
				default:
					t.Errorf("unexpected mTLS fixture path %s", r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
					return
				}
				raw, err := json.Marshal(payload)
				if err != nil {
					t.Error(err)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				if truncated {
					// The complete-looking JSON is still an incomplete HTTP body.
					w.Header().Set("Content-Length", fmt.Sprint(len(raw)+64))
				}
				_, _ = w.Write(raw)
			}))
			server.TLS = pki.ServerTLS()
			server.StartTLS()
			defer server.Close()
			config := healthRemoteConfig(t, pki, server.URL, "urn:engineering-platform:control:publisher-client")
			client, err := newRemoteClient(config)
			if err != nil {
				t.Fatal(err)
			}
			defer client.client.CloseIdleConnections()
			// These are local protocol fixtures, not live GitHub publication,
			// selected-host credentials or original dispatcher-quiescence proof.
			pub, pubErr := client.Publish(context.Background(), plan, "unused-local-bundle", testDispatchIdentity(plan))
			obs, obsErr := client.Observe(context.Background(), plan)
			health, healthErr := client.health(context.Background(), "sha256:"+strings.Repeat("a", 64))
			if truncated {
				if !errors.Is(pubErr, io.ErrUnexpectedEOF) || !errors.Is(obsErr, io.ErrUnexpectedEOF) || !errors.Is(healthErr, io.ErrUnexpectedEOF) || pub != (PublicationReceipt{}) || !reflect.DeepEqual(obs, ObserveResult{}) || health != (PublisherHealthObservation{}) {
					t.Fatalf("mTLS truncation lost exact failure or retained partial authority: %v / %v / %v", pubErr, obsErr, healthErr)
				}
			} else if pubErr != nil || obsErr != nil || healthErr != nil || pub.Validate(plan) != nil || obs.Outcome != ObservedConfirmed || !health.EndpointObserved {
				t.Fatalf("complete mTLS response rejected: %v / %v / %v", pubErr, obsErr, healthErr)
			}
			if calls.Load() != 3 {
				t.Fatalf("mTLS client retried: %d requests", calls.Load())
			}
		})
	}
}
