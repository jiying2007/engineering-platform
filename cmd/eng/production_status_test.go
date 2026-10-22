package main

import (
	"encoding/json"
	"github.com/jiying2007/engineering-platform/internal/production"
	"testing"
	"time"
)

func TestProductionStatusRejectsUnexpectedArguments(t *testing.T) {
	if err := productionStatus([]string{"unexpected"}); err == nil {
		t.Fatal("unexpected production-status argument accepted")
	}
}

func TestProductionStatusReadbackRejectsContradictoryEnvelope(t *testing.T) {
	now := time.Now().UTC()
	s, err := production.EvaluateSnapshot(production.Snapshot{Version: production.OperationalStatusVersion, CapturedAt: now, RecoveryMode: "NORMAL", WorkerPolls: []production.WorkerPollObservation{}})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(s)
	if _, err = decodeProductionStatus(raw, now); err != nil {
		t.Fatal(err)
	}
	s.Ready = true
	raw, _ = json.Marshal(s)
	if _, err = decodeProductionStatus(raw, now); err == nil {
		t.Fatal("forged READY was trusted")
	}
	s.Ready = false
	raw, _ = json.Marshal(s)
	raw = append(raw[:len(raw)-1], []byte(`,"ready":true}`)...)
	if _, err = decodeProductionStatus(raw, now); err == nil {
		t.Fatal("duplicate key was trusted")
	}
	if err = productionStatus([]string{"--require-ready", "--require-authority-clear"}); err == nil {
		t.Fatal("ambiguous status scope")
	}
}

func TestProductionStatusReadbackRejectsIncompleteV6Counters(t *testing.T) {
	now := time.Now().UTC()
	status, err := production.EvaluateSnapshot(production.Snapshot{
		Version: production.OperationalStatusVersion, CapturedAt: now,
		RecoveryMode: "NORMAL", WorkerPolls: []production.WorkerPollObservation{},
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"unknown_offline_executions", "expired_offline_executions", "expired_codex_executions", "expired_outbox_leases"} {
		var envelope map[string]json.RawMessage
		if err := json.Unmarshal(raw, &envelope); err != nil {
			t.Fatal(err)
		}
		var snapshot map[string]json.RawMessage
		if err := json.Unmarshal(envelope["snapshot"], &snapshot); err != nil {
			t.Fatal(err)
		}
		delete(snapshot, field)
		envelope["snapshot"], err = json.Marshal(snapshot)
		if err != nil {
			t.Fatal(err)
		}
		bad, err := json.Marshal(envelope)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := decodeProductionStatus(bad, now); err == nil {
			t.Fatalf("CLI accepted incomplete v6 snapshot without %s", field)
		}
	}
}
