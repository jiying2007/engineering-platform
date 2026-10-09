package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/production"
)

func TestProductionLiveObserveCLIRequiresExplicitSingleOwnerConfig(t *testing.T) {
	good := []string{"--config", "/etc/engineering-platform/production-preflight.json"}
	got, err := parseProductionLiveObserve(good)
	if err != nil || got != good[1] {
		t.Fatal(got, err)
	}
	for _, args := range [][]string{
		nil, {"--config"}, {"--unknown", "x"}, {"--config", "file", "extra"},
		{"--config", "file", "--config", "other"},
		{"--config=file", "--config=other"},
		{"--config", "file", "--provider-qualified"},
		{"--config", "file", "--ready"},
		{"--config", "file", "--execute"},
	} {
		if got, err := parseProductionLiveObserve(args); err == nil {
			t.Fatalf("implicit authority or ambiguous configuration accepted: args=%v got=%q", args, got)
		}
	}
}

func TestLiveProcessDoubleReadRejectsReplacementOrMissingService(t *testing.T) {
	now := time.Unix(1700000000, 0).UTC()
	first := []production.LiveServiceObservation{
		{Unit: "engineering-control-plane.service", ServiceUser: "control", PID: 1001, StartTicks: 501, Binary: "control-plane", ObservedAt: now},
		{Unit: "engineering-worker-admission.service", ServiceUser: "admission", PID: 1002, StartTicks: 502, Binary: "worker", ObservedAt: now},
		{Unit: "engineering-worker-preparation.service", ServiceUser: "preparation", PID: 1003, StartTicks: 503, Binary: "worker", ObservedAt: now},
		{Unit: "engineering-publisher.service", ServiceUser: "publisher", PID: 1004, StartTicks: 504, Binary: "publisher-service", ObservedAt: now},
	}
	second := append([]production.LiveServiceObservation(nil), first...)
	for i := range second {
		second[i].ObservedAt = now.Add(time.Second)
	}
	if !sameLiveServiceIdentities(first, second) {
		t.Fatal("stable double-read process identities rejected")
	}
	for _, tc := range []struct {
		name   string
		change func([]production.LiveServiceObservation) []production.LiveServiceObservation
	}{
		{"lost-service", func(x []production.LiveServiceObservation) []production.LiveServiceObservation { return x[:3] }},
		{"changed-pid", func(x []production.LiveServiceObservation) []production.LiveServiceObservation { x[1].PID++; return x }},
		{"reused-pid-new-generation", func(x []production.LiveServiceObservation) []production.LiveServiceObservation {
			x[1].StartTicks++
			return x
		}},
		{"missing-generation", func(x []production.LiveServiceObservation) []production.LiveServiceObservation {
			x[2].StartTicks = 0
			return x
		}},
		{"wrong-uid", func(x []production.LiveServiceObservation) []production.LiveServiceObservation {
			x[2].ServiceUser = "root"
			return x
		}},
		{"wrong-binary", func(x []production.LiveServiceObservation) []production.LiveServiceObservation {
			x[0].Binary = "other"
			return x
		}},
		{"wrong-unit", func(x []production.LiveServiceObservation) []production.LiveServiceObservation {
			x[0].Unit = "alternate.service"
			return x
		}},
		{"old-fact", func(x []production.LiveServiceObservation) []production.LiveServiceObservation {
			x[0].ObservedAt = now.Add(-time.Second)
			return x
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := append([]production.LiveServiceObservation(nil), second...)
			if sameLiveServiceIdentities(first, tc.change(input)) {
				t.Fatal("ambiguous or restarted service was admitted")
			}
		})
	}
}

func TestProductionLiveObservationNeverClaimsCapacityOrProduction(t *testing.T) {
	report := productionLiveReport{
		Version: 1, Scope: "bounded-host-live-service-observations",
		SourceCommit:                  strings.Repeat("a", 40),
		ObservedAt:                    time.Unix(1700000000, 0).UTC(),
		DatabaseAuthorityClear:        true,
		PublisherEndpointObserved:     true,
		LocalServiceProcessesObserved: true,
		Services:                      []production.LiveServiceObservation{},
		State:                         "LOCAL_COMPONENTS_OBSERVED_PROVIDER_CAPACITY_PENDING",
	}
	raw, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	var readback map[string]any
	if err := json.Unmarshal(raw, &readback); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"provider_live_qualified", "capacity_observed", "ready", "production_qualified"} {
		if readback[key] != false {
			t.Fatalf("local service observation promoted forbidden production authority: key=%s value=%v", key, readback[key])
		}
	}
}
