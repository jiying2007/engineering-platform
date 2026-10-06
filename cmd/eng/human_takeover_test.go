package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jiying2007/engineering-platform/internal/controlclient"
	"github.com/jiying2007/engineering-platform/internal/run"
	"github.com/jiying2007/engineering-platform/internal/session"
	"github.com/jiying2007/engineering-platform/internal/testsupport"
)

func TestHumanTakeoverUsesCertificateActorAndEpochFence(t *testing.T) {
	actor := "urn:engineering-platform:operator:takeover"
	pki := testsupport.NewPKI(t)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/runs/run-1/takeover" {
			t.Errorf("unexpected takeover request %s %s", r.Method, r.URL.Path)
		}
		var body struct {
			ExecutionEpoch uint64 `json:"execution_epoch"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil || body.ExecutionEpoch != 7 {
			t.Error("takeover epoch drift", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"run": run.Run{
				ID: "run-1", State: run.HumanControlled,
				CurrentEpoch: 8, ControlOwner: "HUMAN",
			},
			"session": session.Session{
				RunID: "run-1", ExecutionEpoch: 8, Owner: session.Human,
			},
		})
	}))
	server.TLS = pki.ServerTLS()
	server.StartTLS()
	defer server.Close()
	client, err := controlclient.New(server.URL, &tls.Config{
		MinVersion: tls.VersionTLS13, RootCAs: pki.Roots,
		Certificates: []tls.Certificate{pki.ClientCertificate(t, actor)},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	receipt, err := executeHumanTakeover(context.Background(), client, humanTakeoverOptions{runID: "run-1", epoch: 7})
	if err != nil ||
		receipt.Actor != actor || receipt.PreviousEpoch != 7 || receipt.ExecutionEpoch != 8 ||
		receipt.ControlOwner != "HUMAN" || receipt.RuntimeReplay ||
		receipt.ModelTurnExecuted || receipt.ProductionQualified {
		t.Fatal(receipt, err)
	}
}

func TestHumanTakeoverCLIRequiresExactRunAndEpoch(t *testing.T) {
	if _, err := parseHumanTakeover([]string{"--run", "run-1", "--epoch", "7"}); err != nil {
		t.Fatal(err)
	}
	for _, bad := range [][]string{
		{},
		{"--run", "run-1"},
		{"--epoch", "7"},
		{"--run", "../x", "--epoch", "7"},
		{"--run", "run-1", "--epoch", "0"},
		{"--run", "run-1", "--epoch", "7", "--execute"},
		{"--run", "run-1", "--run", "run-2", "--epoch", "7"},
	} {
		if _, err := parseHumanTakeover(bad); err == nil {
			t.Fatal("unsafe takeover accepted", bad)
		}
	}
}
