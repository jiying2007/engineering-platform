package production

import (
	"bytes"
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/strictjson"
	"github.com/jiying2007/engineering-platform/internal/workerqueue"
)

func capacityFixture() (CapacityResponse, time.Time) {
	now := time.Now().UTC()
	poll := now.Add(-24 * time.Hour) // A historical poll is never a heartbeat.
	grant := WorkerCapacityGrant{Subject: "urn:engineering-platform:worker:capacity", Profiles: []string{"worker/capacity"}, Lane: "ADMISSION"}
	authority, _ := EvaluateSnapshot(Snapshot{Version: OperationalStatusVersion, CapturedAt: now, RecoveryMode: "NORMAL", WorkerPolls: []WorkerPollObservation{{WorkerProfile: grant.Profiles[0], KnownIdentities: 1, LatestPollAt: poll}}})
	response := CapacityResponse{
		Version: CapacityVersion, Scope: CapacityScope, GeneratedAt: now,
		Source: CapacitySource{ServerCertificateDigest: canonical.BytesDigest([]byte("certificate")), AccessPolicyDigest: canonical.BytesDigest([]byte("policy")), SourceCommit: strings.Repeat("a", 40), InstanceID: strings.Repeat("b", 32)},
		Facts: CapacityFacts{Authority: authority, CapturedAt: now, RecoveryMode: "NORMAL",
			Workers: []WorkerCapacityFact{{Grant: grant, Recorded: true, Protocol: workerqueue.Protocol, Status: "ONLINE", LastPollProfile: grant.Profiles[0], LastPollAt: &poll}},
			Queues:  []QueueCapacityFact{{Profile: grant.Profiles[0]}},
			Pool:    PoolCapacityFact{CapturedAt: now, Maximum: 8, Total: 5, Acquired: 2, Idle: 2, Constructing: 1},
		},
	}
	return response, now
}

func capacityBytes(t *testing.T, response CapacityResponse) []byte {
	t.Helper()
	raw, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestCapacityRetainsOriginalFactsWithoutPromotingHistoricalRegistration(t *testing.T) {
	response, now := capacityFixture()
	raw := append(capacityBytes(t, response), '\n')
	got, err := ObserveCapacity(raw, response.Source.ServerCertificateDigest, now.Add(-time.Second), now, response.Source)
	if err != nil {
		t.Fatal(err)
	}
	if got.PoolReservationHeadroom != 5 || got.WorkerReservationHeadroom != 1 || len(got.Workers) != 1 || got.Workers[0].LiveLeaseLimit != 1 {
		t.Fatalf("incorrect reservation facts: %#v", got)
	}
	if !bytes.Equal(got.ResponseBytes, raw) || got.ResponseDigest != canonical.BytesDigest(raw) || !reflect.DeepEqual(got.Response, response) {
		t.Fatal("original response bytes or decoded facts changed")
	}
	if got.Ready || got.ProductionQualified || got.ExecutionAuthorized || got.WorkerProcessCapacityObserved || got.PublisherCapacityObserved || got.ServiceReadiness != ServiceReadinessUnobserved {
		t.Fatal("old poll or unoccupied reservation became runtime capacity/readiness")
	}
	raw[0] = 'x'
	if got.ResponseBytes[0] != '{' {
		t.Fatal("retained bytes alias caller buffer")
	}
	for _, change := range []func(*WorkerCapacityFact){
		func(f *WorkerCapacityFact) { f.Status = "OFFLINE" },
		func(f *WorkerCapacityFact) { f.Protocol = "historical-protocol" },
		func(f *WorkerCapacityFact) { f.LastPollProfile = "worker/no-longer-authorized" },
		func(f *WorkerCapacityFact) {
			f.Recorded = false
			f.Protocol = ""
			f.Status = ""
			f.LastPollProfile = ""
			f.LastPollAt = nil
		},
	} {
		changed, at := capacityFixture()
		change(&changed.Facts.Workers[0])
		fact := changed.Facts.Workers[0]
		snapshot := changed.Facts.Authority.Snapshot
		snapshot.WorkerPolls = []WorkerPollObservation{}
		if fact.Recorded && fact.Status == "ONLINE" {
			snapshot.WorkerPolls = []WorkerPollObservation{{WorkerProfile: fact.LastPollProfile, KnownIdentities: 1, LatestPollAt: *fact.LastPollAt}}
		}
		changed.Facts.Authority, _ = EvaluateSnapshot(snapshot)
		r, err := ObserveCapacity(capacityBytes(t, changed), changed.Source.ServerCertificateDigest, at.Add(-time.Second), at, changed.Source)
		if err != nil || r.WorkerReservationHeadroom != 0 || r.Workers[0].RecordedRegistrationUsable {
			t.Fatalf("inactive/foreign/missing registration counted as headroom: %#v %v", r, err)
		}
	}
}

func TestCapacitySourcePinsRejectCopiedConfigurationAndRestart(t *testing.T) {
	for name, change := range map[string]func(*CapacitySource){
		"certificate":                            func(s *CapacitySource) { s.ServerCertificateDigest = canonical.BytesDigest([]byte("other")) },
		"policy":                                 func(s *CapacitySource) { s.AccessPolicyDigest = canonical.BytesDigest([]byte("other")) },
		"source":                                 func(s *CapacitySource) { s.SourceCommit = strings.Repeat("c", 40) },
		"same-certificate-policy-other-instance": func(s *CapacitySource) { s.InstanceID = strings.Repeat("c", 32) },
	} {
		t.Run(name, func(t *testing.T) {
			response, now := capacityFixture()
			expected := response.Source
			change(&response.Source)
			got, err := ObserveCapacity(capacityBytes(t, response), expected.ServerCertificateDigest, now.Add(-time.Second), now, expected)
			if err == nil || !reflect.DeepEqual(got, CapacityObservation{}) {
				t.Fatal("source mismatch retained an observation", err)
			}
		})
	}
}

func TestCapacityContradictionAndFreshnessFailClosed(t *testing.T) {
	for name, mutate := range map[string]func(*CapacityResponse){
		"worker-expiry-exceeds-authority": func(r *CapacityResponse) { r.Facts.Workers[0].ExpiredLeases = 1 },
		"queue-expiry-exceeds-authority":  func(r *CapacityResponse) { r.Facts.Queues[0].ExpiredLeases = 1 },
		"queue-active-exceeds-authority":  func(r *CapacityResponse) { r.Facts.Queues[0].ActiveLeases = 1 },
		"recorded-online-absent-from-history": func(r *CapacityResponse) {
			snapshot := r.Facts.Authority.Snapshot
			snapshot.WorkerPolls = []WorkerPollObservation{}
			r.Facts.Authority, _ = EvaluateSnapshot(snapshot)
		},
		"recorded-poll-newer-than-history": func(r *CapacityResponse) {
			at := r.Facts.CapturedAt.Add(-time.Hour)
			r.Facts.Workers[0].LastPollAt = &at
		},
		"negative-active":                     func(r *CapacityResponse) { r.Facts.Workers[0].ActiveLeases = -1 },
		"two-live-leases-one-identity":        func(r *CapacityResponse) { r.Facts.Workers[0].ActiveLeases = 2 },
		"missing-worker-row-with-reservation": func(r *CapacityResponse) { r.Facts.Workers[0].Recorded = false; r.Facts.Workers[0].ActiveLeases = 1 },
		"pool-sum":                            func(r *CapacityResponse) { r.Facts.Pool.Idle++ },
		"pool-over-limit":                     func(r *CapacityResponse) { r.Facts.Pool.Maximum = 1 },
		"queue-count-time":                    func(r *CapacityResponse) { r.Facts.Queues[0].Pending = 1 },
		"missing-authorized-queue":            func(r *CapacityResponse) { r.Facts.Queues = []QueueCapacityFact{} },
		"duplicate-grant-profile": func(r *CapacityResponse) {
			r.Facts.Workers[0].Grant.Profiles = []string{"worker/capacity", "worker/capacity"}
		},
		"unknown-recovery":     func(r *CapacityResponse) { r.Facts.RecoveryMode = "READY" },
		"stale-database":       func(r *CapacityResponse) { r.Facts.CapturedAt = r.GeneratedAt.Add(-31 * time.Second) },
		"future-database":      func(r *CapacityResponse) { r.Facts.CapturedAt = r.GeneratedAt.Add(6 * time.Second) },
		"pool-after-generated": func(r *CapacityResponse) { r.Facts.Pool.CapturedAt = r.GeneratedAt.Add(time.Millisecond) },
		"future-poll": func(r *CapacityResponse) {
			at := r.Facts.CapturedAt.Add(time.Second)
			r.Facts.Workers[0].LastPollAt = &at
		},
	} {
		t.Run(name, func(t *testing.T) {
			response, now := capacityFixture()
			mutate(&response)
			if got, err := ObserveCapacity(capacityBytes(t, response), response.Source.ServerCertificateDigest, now.Add(-time.Second), now, response.Source); err == nil || !reflect.DeepEqual(got, CapacityObservation{}) {
				t.Fatal("inconsistent capacity response accepted", err)
			}
		})
	}
	response, now := capacityFixture()
	// A report can be less than the general age limit and still predate the GET.
	if _, err := ObserveCapacity(capacityBytes(t, response), response.Source.ServerCertificateDigest, now.Add(6*time.Second), now.Add(7*time.Second), response.Source); err == nil {
		t.Fatal("cached pre-request facts accepted")
	}
	if _, err := ObserveCapacity(capacityBytes(t, response), response.Source.ServerCertificateDigest, now.Add(-31*time.Second), now, response.Source); err == nil {
		t.Fatal("unbounded request window accepted")
	}
}

func TestCapacityWireRequiresEveryFactAndExplicitNullableTimestamp(t *testing.T) {
	response, _ := capacityFixture()
	raw := capacityBytes(t, response)
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	type objectPath struct {
		path   []string
		object map[string]any
	}
	var paths []objectPath
	var walk func(map[string]any, []string)
	walk = func(object map[string]any, path []string) {
		paths = append(paths, objectPath{path, object})
		for key, child := range object {
			switch child := child.(type) {
			case map[string]any:
				walk(child, append(append([]string{}, path...), key))
			case []any:
				if len(child) > 0 {
					if nested, ok := child[0].(map[string]any); ok {
						walk(nested, append(append([]string{}, path...), key, "0"))
					}
				}
			}
		}
	}
	walk(value, nil)
	for _, location := range paths {
		keys := make([]string, 0, len(location.object))
		for key := range location.object {
			keys = append(keys, key)
		}
		slices.Sort(keys)
		for _, key := range keys {
			original := location.object[key]
			name := strings.Join(append(append([]string{}, location.path...), key), "/")
			for _, mode := range []string{"missing", "null"} {
				if mode == "null" && slices.Contains([]string{"last_poll_at", "oldest_pending_at", "oldest_pending_worker_at", "oldest_pending_outbox_at", "last_worker_admission_at", "last_outbox_dispatch_at"}, key) {
					continue
				}
				t.Run(name+"/"+mode, func(t *testing.T) {
					if mode == "missing" {
						delete(location.object, key)
					} else {
						location.object[key] = nil
					}
					bad, err := json.Marshal(value)
					if err != nil {
						t.Fatal(err)
					}
					location.object[key] = original
					var decoded CapacityResponse
					if strictjson.Decode(bad, &decoded) == nil {
						t.Fatalf("%s fact silently defaulted", name)
					}
				})
			}
		}
	}
	for _, bad := range [][]byte{
		bytes.Replace(raw, []byte(`"active_leases":0`), []byte(`"active_leases":0,"active_leases":1`), 1),
		bytes.Replace(raw, []byte(`"active_leases":0`), []byte(`"Active_Leases":0`), 1),
		bytes.Replace(raw, []byte(`"scope":`), []byte(`"ready":true,"scope":`), 1),
		append(append([]byte{}, raw...), []byte(`{}`)...),
	} {
		var decoded CapacityResponse
		if strictjson.Decode(bad, &decoded) == nil {
			t.Fatal("ambiguous/overclaimed capacity bytes accepted")
		}
	}
}

func TestCapacityPreservesOriginalAuthorityBlockersBesideUnoccupiedBudget(t *testing.T) {
	response, now := capacityFixture()
	snapshot := response.Facts.Authority.Snapshot
	snapshot.UnknownCodexExecutions = 1
	snapshot.UnknownOfflineExecutions = 1
	snapshot.ExpiredWorkerLeases = 1
	snapshot.ExpiredOutboxLeases = 1
	snapshot.LeasedOutbox = 1
	response.Facts.Authority, _ = EvaluateSnapshot(snapshot)
	got, err := ObserveCapacity(capacityBytes(t, response), response.Source.ServerCertificateDigest, now.Add(-time.Second), now, response.Source)
	if err != nil || got.Response.Facts.Authority.AuthorityClear || got.Response.Facts.Authority.State != OperationalDegraded || got.WorkerReservationHeadroom != 1 || got.Ready {
		t.Fatal("capacity hid global unresolved authority facts", got, err)
	}
	response.Facts.Authority.AuthorityClear = true
	if _, err := ObserveCapacity(capacityBytes(t, response), response.Source.ServerCertificateDigest, now.Add(-time.Second), now, response.Source); err == nil {
		t.Fatal("forged clear authority accepted alongside unknown executions")
	}
}
