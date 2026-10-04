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
	s, err := production.EvaluateSnapshot(production.Snapshot{Version: production.OperationalStatusVersion, CapturedAt: now, RecoveryMode: "NORMAL"})
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
