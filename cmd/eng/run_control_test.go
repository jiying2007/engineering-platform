package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/codexexec"
	"github.com/jiying2007/engineering-platform/internal/controlclient"
	"github.com/jiying2007/engineering-platform/internal/session"
	"github.com/jiying2007/engineering-platform/internal/testsupport"
)

func TestRunControlInputAndForbiddenFallbacks(t *testing.T) {
	text := filepath.Join(t.TempDir(), "input.txt")
	if err := os.WriteFile(text, []byte("只修复故障。"), 0600); err != nil {
		t.Fatal(err)
	}
	valid := []string{"steer", "--run", "run", "--id", "cmd", "--epoch", "1", "--sequence", "1", "--thread", "thread", "--turn", "turn", "--text-file", text}
	got, err := parseRunControl(valid)
	if err != nil || got.input.Text != "只修复故障。" {
		t.Fatal(got, err)
	}
	for _, args := range [][]string{nil, {"pause"}, {"takeover"}, {"resume"}, {"status", "--id", "../x"}, {"inspect", "--run", "run", "--id", "x"}, {"steer", "--run", "run"}, append(append([]string{}, valid...), "--actor", "spoof"), append([]string{"interrupt"}, valid[1:]...)} {
		if _, err := parseRunControl(args); err == nil {
			t.Fatal("invalid flags accepted", args)
		}
	}
	link := text + "-link"
	if err := os.Symlink(text, link); err != nil {
		t.Fatal(err)
	}
	invalid := append([]string{}, valid...)
	invalid[len(invalid)-1] = link
	if _, err := parseRunControl(invalid); err == nil {
		t.Fatal("symlink text accepted")
	}
	for _, data := range []string{"", strings.Repeat("x", 8193), "\xff", "text\x00"} {
		if err := os.WriteFile(text, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := parseRunControl(valid); err == nil {
			t.Fatal("unbounded or invalid text accepted")
		}
	}
}
func TestRunControlActualMTLSNoPOSTReplay(t *testing.T) {
	pki := testsupport.NewPKI(t)
	actor := "urn:engineering-platform:engineer:cli-test"
	requests := make(chan codexexec.ControlInput, 2)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var i codexexec.ControlInput
		if err := json.NewDecoder(r.Body).Decode(&i); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		requests <- i
		if r.URL.Path != "/api/v1/runs/run/steer" {
			t.Error("bad request path", r.URL.Path)
		}
		if i.Actor != actor || r.TLS.PeerCertificates[0].URIs[0].String() != actor {
			t.Error("actor not bound to TLS")
		}
		p := codexexec.ControlPayload{Kind: codexexec.ControlSteer, Text: i.Text, Binding: codexexec.ControlBinding{Token: codexexec.Token{ID: strings.Repeat("a", 64), RunID: "run", WorkerProfile: "worker/codex", ProfileDigest: canonical.BytesDigest([]byte("profile"))}, ExecutionEpoch: i.ExecutionEpoch, ThreadID: i.ThreadID, TurnID: i.TurnID}}
		digest, _ := canonical.Digest(p)
		d := codexexec.ControlDelivery{Payload: p, State: codexexec.ControlQueued, Command: session.SteeringCommand{ID: i.ID, Actor: i.Actor, RunID: "run", ExecutionEpoch: i.ExecutionEpoch, Sequence: i.Sequence, ContentDigest: digest, CreatedAt: time.Now().UTC()}}
		if i.ID == "lost" {
			w.WriteHeader(503)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(202)
		_ = json.NewEncoder(w).Encode(d)
	})
	s := httptest.NewUnstartedServer(handler)
	s.TLS = pki.ServerTLS()
	s.StartTLS()
	defer s.Close()
	c, err := controlclient.New(s.URL, &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: pki.Roots, Certificates: []tls.Certificate{pki.ClientCertificate(t, actor)}})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	o := runControlOptions{action: "steer", runID: "run", input: codexexec.ControlInput{ID: "first", ExecutionEpoch: 1, Sequence: 1, ThreadID: "thread", TurnID: "turn", Text: "keep scope"}}
	reply, err := executeRunControl(context.Background(), c, o)
	if err != nil {
		t.Fatal(err)
	}
	if reply.(codexexec.ControlDelivery).State != codexexec.ControlQueued {
		t.Fatal("CLI promoted queued receipt")
	}
	<-requests
	o.input.ID = "lost"
	if _, err = executeRunControl(context.Background(), c, o); err == nil {
		t.Fatal("lost reply ignored")
	}
	<-requests
	select {
	case <-requests:
		t.Fatal("POST replayed")
	default:
	}
}
