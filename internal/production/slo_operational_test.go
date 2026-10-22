package production

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/canonical"
)

func sliLimit(n int64) *int64 { return &n }

type operationalSLIFixture struct {
	input     OperationalSLOInput
	criteria  OperationalSLOCriteria
	events    map[string]OperationalSLIEventSource
	resources OperationalSLIResourceSource
	root      string
	now       time.Time
}

func newOperationalSLIFixture(t *testing.T) *operationalSLIFixture {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("v3 source readback requires the Linux no-follow, nonblocking opener")
	}
	start := time.Unix(1_700_000_000, 0).UTC()
	c := OperationalSLOCriteria{Version: 1, RunID: "synthetic-run", SubjectDigest: canonical.BytesDigest([]byte("synthetic-subject")), DeploymentDigest: canonical.BytesDigest([]byte("synthetic-deployment")), WorkloadDigest: canonical.BytesDigest([]byte("synthetic-workload")), CollectorDigest: canonical.BytesDigest([]byte("synthetic-collector")), DeclaredAt: start.Add(-time.Hour), StartedAt: start, CompletedAt: start.Add(10 * time.Second), MinimumSoakMS: sliLimit(10000), MaximumSourceAgeMS: sliLimit(60000), MaximumResourceGapMS: sliLimit(5000), Metrics: map[string]OperationalSLIMetricCriteria{}, Resources: map[string]OperationalSLIResourceCriteria{}}
	b := OperationalSLISourceBinding{Version: 1, RunID: c.RunID, SubjectDigest: c.SubjectDigest, DeploymentDigest: c.DeploymentDigest, WorkloadDigest: c.WorkloadDigest, CollectorDigest: c.CollectorDigest, StartedAt: c.StartedAt, CompletedAt: c.CompletedAt}
	f := &operationalSLIFixture{criteria: c, root: t.TempDir(), now: c.CompletedAt.Add(time.Second), events: map[string]OperationalSLIEventSource{}, resources: OperationalSLIResourceSource{OperationalSLISourceBinding: b, Kind: "resource_samples"}}
	for _, name := range nonProviderSLOs {
		f.criteria.Metrics[name] = OperationalSLIMetricCriteria{MinimumAttempts: sliLimit(3), MaximumNonSuccessPPM: sliLimit(500000), MaximumP99MS: sliLimit(900), MaximumLatencyMS: sliLimit(1000), MinimumSuccessMilliPerSecond: sliLimit(100)}
		f.events[name] = OperationalSLIEventSource{OperationalSLISourceBinding: b, Kind: "request_events", Name: name, Events: []OperationalSLIEvent{
			{Sequence: 1, ID: "one", ObservedAt: start.Add(time.Second), Outcome: "success", LatencyMS: sliLimit(10)},
			{Sequence: 2, ID: "two", ObservedAt: start.Add(2 * time.Second), Outcome: "success", LatencyMS: sliLimit(20)},
			{Sequence: 3, ID: "three", ObservedAt: start.Add(3 * time.Second), Outcome: "failure", LatencyMS: sliLimit(90)},
		}}
	}
	for _, name := range operationalSLIResources {
		f.criteria.Resources[name] = OperationalSLIResourceCriteria{MaximumPeak: sliLimit(10), MaximumPositiveNetGrowth: sliLimit(5)}
	}
	for i := 0; i < 3; i++ {
		values := map[string]*int64{}
		for _, name := range operationalSLIResources {
			values[name] = sliLimit(int64(i + 1))
		}
		f.resources.Samples = append(f.resources.Samples, OperationalSLIResourceSample{Sequence: int64(i + 1), ObservedAt: start.Add(time.Duration(i) * 5 * time.Second), Values: values})
	}
	f.retain(t)
	return f
}

func (f *operationalSLIFixture) write(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	digest := canonical.BytesDigest(raw)
	if err := os.WriteFile(filepath.Join(f.root, digest[7:]+".json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return digest
}

func (f *operationalSLIFixture) retain(t *testing.T) {
	t.Helper()
	f.input = OperationalSLOInput{Version: OperationalSLOVersion, CriteriaDigest: f.write(t, f.criteria), SourceDigests: map[string]string{}}
	for name, source := range f.events {
		source.CriteriaDigest = f.input.CriteriaDigest
		f.events[name] = source
		f.input.SourceDigests[name] = f.write(t, source)
	}
	f.resources.CriteriaDigest = f.input.CriteriaDigest
	f.input.ResourceSourceDigest = f.write(t, f.resources)
}

func (f *operationalSLIFixture) evaluate() (OperationalSLOEnvelope, error) {
	return EvaluateOperationalSLO(f.input, f.root, f.criteria.RunID, f.criteria.SubjectDigest, f.criteria.DeploymentDigest, f.now)
}

func TestOperationalSLOReadbackDeterministicAndNeverQualifies(t *testing.T) {
	f := newOperationalSLIFixture(t)
	first, err := f.evaluate()
	if err != nil {
		t.Fatal(err)
	}
	second, err := f.evaluate()
	if err != nil || !reflect.DeepEqual(first, second) {
		t.Fatalf("nondeterministic report: %v", err)
	}
	r := first.Report
	if r.Version != 3 || r.Status != "SOURCE_BYTES_EVALUATED" || !r.CriteriaMet || !r.SourceBytesVerified || r.CollectorAuthenticated || r.PreRegistrationAuthenticated || r.QualificationGranted || r.ProductionQualified || r.ProviderMeasurementPresent {
		t.Fatal("source comparison became authentication or production acceptance")
	}
	if len(r.Metrics) != 6 || len(r.Resources) != 7 || len(r.Failures) != 0 {
		t.Fatal("coverage lost")
	}
	for _, metric := range r.Metrics {
		if metric.Total != 3 || metric.Success != 2 || metric.Failure != 1 || metric.NonSuccessPPMUpper != 333334 || metric.SuccessMilliPerSecondLower != 200 || metric.LatencyCount != 3 || metric.P99MS != 90 {
			t.Fatalf("bad conservative summary: %+v", metric)
		}
	}
	if r.Resources[0].Peak != 3 || r.Resources[0].PositiveNetGrowth != 2 {
		t.Fatal("resource summary lost raw values")
	}
}

func TestOperationalSLOAllOutcomesEnterDenominatorAndLatency(t *testing.T) {
	f := newOperationalSLIFixture(t)
	name := nonProviderSLOs[0]
	s := f.events[name]
	s.Events = nil
	for i, outcome := range []string{"success", "failure", "cancelled", "aborted", "timeout", "dropped"} {
		e := OperationalSLIEvent{Sequence: int64(i + 1), ID: fmt.Sprint(i), ObservedAt: f.criteria.StartedAt.Add(time.Duration(i+1) * time.Second), Outcome: outcome}
		if outcome != "dropped" {
			e.LatencyMS = sliLimit([]int64{10, 100, 200, 300, 900}[i])
		}
		s.Events = append(s.Events, e)
	}
	f.events[name] = s
	f.retain(t)
	r, err := f.evaluate()
	if err != nil {
		t.Fatal(err)
	}
	m := r.Report.Metrics[0]
	if m.Name != name || m.Total != 6 || m.Success != 1 || m.Failure != 1 || m.Cancelled != 1 || m.Aborted != 1 || m.Timeout != 1 || m.Dropped != 1 || m.LatencyCount != 5 || m.P50MS != 200 || m.P99MS != 900 || m.NonSuccessPPMUpper != 833334 || m.SuccessMilliPerSecondLower != 100 {
		t.Fatalf("outcomes discarded: %+v", m)
	}
	if r.Report.CriteriaMet {
		t.Fatal("dropped samples accepted")
	}
	found := false
	for _, failure := range r.Report.Failures {
		found = found || failure.Code == "DROPPED_EVENTS"
	}
	if !found {
		t.Fatal("dropped evidence missing")
	}
}

func TestOperationalSLOCriteriaFailuresRemainVisible(t *testing.T) {
	f := newOperationalSLIFixture(t)
	name := nonProviderSLOs[0]
	f.criteria.Metrics[name] = OperationalSLIMetricCriteria{MinimumAttempts: sliLimit(4), MaximumNonSuccessPPM: sliLimit(333333), MaximumP99MS: sliLimit(40), MaximumLatencyMS: sliLimit(50), MinimumSuccessMilliPerSecond: sliLimit(201)}
	f.criteria.Resources["rss_bytes"] = OperationalSLIResourceCriteria{MaximumPeak: sliLimit(2), MaximumPositiveNetGrowth: sliLimit(1)}
	f.retain(t)
	r, err := f.evaluate()
	if err != nil {
		t.Fatal(err)
	}
	if r.Report.CriteriaMet || len(r.Report.Failures) != 7 {
		t.Fatalf("expected all seven actual failures: %+v", r.Report.Failures)
	}
	for _, failure := range r.Report.Failures {
		if failure.Code == "" || failure.Name == "" {
			t.Fatal("unlocatable failure")
		}
	}
}

func TestOperationalSLORejectsInvalidCriteria(t *testing.T) {
	cases := map[string]func(*operationalSLIFixture){
		"version":        func(f *operationalSLIFixture) { f.criteria.Version = 2 },
		"missing_metric": func(f *operationalSLIFixture) { delete(f.criteria.Metrics, nonProviderSLOs[0]) },
		"unknown_metric": func(f *operationalSLIFixture) { f.criteria.Metrics["unknown"] = f.criteria.Metrics[nonProviderSLOs[0]] },
		"missing_limit": func(f *operationalSLIFixture) {
			m := f.criteria.Metrics[nonProviderSLOs[0]]
			m.MaximumNonSuccessPPM = nil
			f.criteria.Metrics[nonProviderSLOs[0]] = m
		},
		"huge_limit": func(f *operationalSLIFixture) {
			m := f.criteria.Metrics[nonProviderSLOs[0]]
			m.MaximumNonSuccessPPM = sliLimit(math.MaxInt64)
			f.criteria.Metrics[nonProviderSLOs[0]] = m
		},
		"inverted_latency_limits": func(f *operationalSLIFixture) {
			m := f.criteria.Metrics[nonProviderSLOs[0]]
			m.MaximumLatencyMS = sliLimit(1)
			f.criteria.Metrics[nonProviderSLOs[0]] = m
		},
		"missing_resource": func(f *operationalSLIFixture) { delete(f.criteria.Resources, "rss_bytes") },
		"resource_missing_limit": func(f *operationalSLIFixture) {
			f.criteria.Resources["rss_bytes"] = OperationalSLIResourceCriteria{MaximumPeak: sliLimit(0)}
		},
		"declared_late": func(f *operationalSLIFixture) { f.criteria.DeclaredAt = f.criteria.StartedAt },
		"zero_window":   func(f *operationalSLIFixture) { f.criteria.CompletedAt = f.criteria.StartedAt },
		"over_cap_window": func(f *operationalSLIFixture) {
			f.criteria.StartedAt = f.criteria.CompletedAt.Add(-32 * 24 * time.Hour)
			f.criteria.DeclaredAt = f.criteria.StartedAt.Add(-time.Hour)
		},
		"incomplete_ms_window": func(f *operationalSLIFixture) { f.criteria.CompletedAt = f.criteria.CompletedAt.Add(time.Nanosecond) },
		"future_window":        func(f *operationalSLIFixture) { f.now = f.criteria.StartedAt },
		"stale":                func(f *operationalSLIFixture) { f.now = f.criteria.CompletedAt.Add(61 * time.Second) },
		"shorter_than_soak":    func(f *operationalSLIFixture) { f.criteria.MinimumSoakMS = sliLimit(10001) },
		"missing_soak":         func(f *operationalSLIFixture) { f.criteria.MinimumSoakMS = nil },
		"gap_exceeds_window":   func(f *operationalSLIFixture) { f.criteria.MaximumResourceGapMS = sliLimit(10001) },
		"missing_age":          func(f *operationalSLIFixture) { f.criteria.MaximumSourceAgeMS = nil },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			f := newOperationalSLIFixture(t)
			mutate(f)
			f.retain(t)
			if _, err := f.evaluate(); err == nil {
				t.Fatal("invalid criteria accepted")
			}
		})
	}
}

func TestOperationalSLORejectsSourceIdentityDrift(t *testing.T) {
	cases := map[string]func(*OperationalSLIEventSource){
		"run":        func(s *OperationalSLIEventSource) { s.RunID = "other" },
		"subject":    func(s *OperationalSLIEventSource) { s.SubjectDigest = canonical.BytesDigest([]byte("other")) },
		"deployment": func(s *OperationalSLIEventSource) { s.DeploymentDigest = canonical.BytesDigest([]byte("other")) },
		"workload":   func(s *OperationalSLIEventSource) { s.WorkloadDigest = canonical.BytesDigest([]byte("other")) },
		"collector":  func(s *OperationalSLIEventSource) { s.CollectorDigest = canonical.BytesDigest([]byte("other")) },
		"criteria":   func(s *OperationalSLIEventSource) { s.CriteriaDigest = canonical.BytesDigest([]byte("other")) },
		"window":     func(s *OperationalSLIEventSource) { s.CompletedAt = s.CompletedAt.Add(time.Second) },
		"kind":       func(s *OperationalSLIEventSource) { s.Kind = "resource_samples" },
		"name":       func(s *OperationalSLIEventSource) { s.Name = nonProviderSLOs[1] },
		"version":    func(s *OperationalSLIEventSource) { s.Version = 2 },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			f := newOperationalSLIFixture(t)
			key := nonProviderSLOs[0]
			s := f.events[key]
			mutate(&s)
			f.input.SourceDigests[key] = f.write(t, s)
			if _, err := f.evaluate(); err == nil {
				t.Fatal("source identity drift accepted")
			}
		})
	}
	f := newOperationalSLIFixture(t)
	if _, err := EvaluateOperationalSLO(f.input, f.root, "other", f.criteria.SubjectDigest, f.criteria.DeploymentDigest, f.now); err == nil {
		t.Fatal("expected run ignored")
	}
	if _, err := EvaluateOperationalSLO(f.input, f.root, f.criteria.RunID, canonical.BytesDigest([]byte("other")), f.criteria.DeploymentDigest, f.now); err == nil {
		t.Fatal("expected subject ignored")
	}
	if _, err := EvaluateOperationalSLO(f.input, f.root, f.criteria.RunID, f.criteria.SubjectDigest, canonical.BytesDigest([]byte("other")), f.now); err == nil {
		t.Fatal("expected deployment ignored")
	}
}

func TestOperationalSLORejectsMalformedOrIncompleteEvents(t *testing.T) {
	cases := map[string]func(*OperationalSLIEventSource){
		"duplicate_id":        func(s *OperationalSLIEventSource) { s.Events[1].ID = s.Events[0].ID },
		"missing_sequence":    func(s *OperationalSLIEventSource) { s.Events[1].Sequence = 3 },
		"unknown_outcome":     func(s *OperationalSLIEventSource) { s.Events[0].Outcome = "pending" },
		"missing_latency":     func(s *OperationalSLIEventSource) { s.Events[0].LatencyMS = nil },
		"negative_latency":    func(s *OperationalSLIEventSource) { s.Events[0].LatencyMS = sliLimit(-1) },
		"huge_latency":        func(s *OperationalSLIEventSource) { s.Events[0].LatencyMS = sliLimit(math.MaxInt64) },
		"dropped_has_latency": func(s *OperationalSLIEventSource) { s.Events[0].Outcome = "dropped" },
		"backwards_time":      func(s *OperationalSLIEventSource) { s.Events[1].ObservedAt = s.StartedAt },
		"future_time":         func(s *OperationalSLIEventSource) { s.Events[0].ObservedAt = s.CompletedAt.Add(time.Second) },
		"empty":               func(s *OperationalSLIEventSource) { s.Events = nil },
		"oversized":           func(s *OperationalSLIEventSource) { s.Events = make([]OperationalSLIEvent, operationalSLOMaxRecords+1) },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			f := newOperationalSLIFixture(t)
			key := nonProviderSLOs[0]
			s := f.events[key]
			mutate(&s)
			f.input.SourceDigests[key] = f.write(t, s)
			if _, err := f.evaluate(); err == nil {
				t.Fatal("malformed events accepted")
			}
		})
	}
}

func TestOperationalSLORejectsResourceGapsAndMalformedValues(t *testing.T) {
	cases := map[string]func(*OperationalSLIResourceSource){
		"empty":          func(s *OperationalSLIResourceSource) { s.Samples = nil },
		"duplicate_time": func(s *OperationalSLIResourceSource) { s.Samples[1].ObservedAt = s.Samples[0].ObservedAt },
		"gap": func(s *OperationalSLIResourceSource) {
			s.Samples[1].ObservedAt = s.Samples[1].ObservedAt.Add(time.Millisecond)
		},
		"tail_missing": func(s *OperationalSLIResourceSource) {
			s.Samples = s.Samples[:2]
			s.Samples[1].ObservedAt = s.StartedAt.Add(4 * time.Second)
		},
		"begin_missing": func(s *OperationalSLIResourceSource) {
			s.Samples = s.Samples[1:]
			s.Samples[0].ObservedAt = s.StartedAt.Add(6 * time.Second)
			s.Samples[0].Sequence = 1
			s.Samples[1].Sequence = 2
		},
		"missing_value":   func(s *OperationalSLIResourceSource) { delete(s.Samples[0].Values, "rss_bytes") },
		"unknown_value":   func(s *OperationalSLIResourceSource) { s.Samples[0].Values["unknown"] = sliLimit(1) },
		"negative":        func(s *OperationalSLIResourceSource) { s.Samples[0].Values["rss_bytes"] = sliLimit(-1) },
		"overflow":        func(s *OperationalSLIResourceSource) { s.Samples[0].Values["rss_bytes"] = sliLimit(math.MaxInt64) },
		"sequence":        func(s *OperationalSLIResourceSource) { s.Samples[0].Sequence = 2 },
		"source_identity": func(s *OperationalSLIResourceSource) { s.CollectorDigest = canonical.BytesDigest([]byte("other")) },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			f := newOperationalSLIFixture(t)
			mutate(&f.resources)
			f.input.ResourceSourceDigest = f.write(t, f.resources)
			if _, err := f.evaluate(); err == nil {
				t.Fatal("resource gap or malformed value accepted")
			}
		})
	}
}

func TestOperationalSLOResourceNullRejectedAndZeroAccepted(t *testing.T) {
	for _, value := range []string{"null", "0"} {
		t.Run(value, func(t *testing.T) {
			f := newOperationalSLIFixture(t)
			path := filepath.Join(f.root, f.input.ResourceSourceDigest[7:]+".json")
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			changed := bytes.Replace(raw, []byte(`"rss_bytes":1`), []byte(`"rss_bytes":`+value), 1)
			if bytes.Equal(raw, changed) {
				t.Fatal("resource fixture did not contain the target integer")
			}
			digest := canonical.BytesDigest(changed)
			if err := os.WriteFile(filepath.Join(f.root, digest[7:]+".json"), changed, 0o600); err != nil {
				t.Fatal(err)
			}
			f.input.ResourceSourceDigest = digest
			report, err := f.evaluate()
			if value == "null" {
				if err == nil {
					t.Fatal("JSON null was accepted as a numeric resource value")
				}
			} else if err != nil || !report.Report.CriteriaMet {
				t.Fatalf("explicit measured zero was rejected: %v", err)
			}
		})
	}
}

func TestOperationalSLOSourceReadbackRejectsUnsafeAndAmbiguousBytes(t *testing.T) {
	for _, kind := range []string{"missing", "tamper", "symlink", "writable", "oversize", "duplicate_json", "unknown_json", "null_json", "duplicate_digest"} {
		t.Run(kind, func(t *testing.T) {
			f := newOperationalSLIFixture(t)
			name := nonProviderSLOs[0]
			path := filepath.Join(f.root, f.input.SourceDigests[name][7:]+".json")
			switch kind {
			case "missing":
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			case "tamper":
				if err := os.WriteFile(path, []byte("{}"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				raw, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				other := filepath.Join(t.TempDir(), "raw")
				if err = os.WriteFile(other, raw, 0o600); err != nil {
					t.Fatal(err)
				}
				if err = os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err = os.Symlink(other, path); err != nil {
					t.Fatal(err)
				}
			case "writable":
				if err := os.Chmod(path, 0o666); err != nil {
					t.Fatal(err)
				}
			case "oversize":
				if err := os.WriteFile(path, make([]byte, (1<<20)+1), 0o600); err != nil {
					t.Fatal(err)
				}
			case "duplicate_digest":
				f.input.SourceDigests[name] = f.input.SourceDigests[nonProviderSLOs[1]]
			default:
				raw, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if kind == "duplicate_json" {
					raw = append([]byte(`{"version":1,`), raw[1:]...)
				}
				if kind == "unknown_json" {
					raw = append([]byte(`{"unexpected":1,`), raw[1:]...)
				}
				if kind == "null_json" {
					raw = []byte("null")
				}
				digest := canonical.BytesDigest(raw)
				if err := os.WriteFile(filepath.Join(f.root, digest[7:]+".json"), raw, 0o600); err != nil {
					t.Fatal(err)
				}
				f.input.SourceDigests[name] = digest
			}
			if _, err := f.evaluate(); err == nil {
				t.Fatal("unsafe or ambiguous retained source accepted")
			}
		})
	}
}

func TestOperationalSLOExtremeCountersAndZeroMeasurements(t *testing.T) {
	f := newOperationalSLIFixture(t)
	name := nonProviderSLOs[0]
	s := f.events[name]
	s.Events = nil
	for i := 0; i < operationalSLOMaxRecords; i++ {
		s.Events = append(s.Events, OperationalSLIEvent{Sequence: int64(i + 1), ID: fmt.Sprint(i), ObservedAt: s.CompletedAt, Outcome: "failure", LatencyMS: sliLimit(0)})
	}
	f.input.SourceDigests[name] = f.write(t, s)
	r, err := f.evaluate()
	if err != nil {
		t.Fatal(err)
	}
	m := r.Report.Metrics[0]
	if m.NonSuccessPPMUpper != 1000000 || m.SuccessMilliPerSecondLower != 0 || m.Total != 4096 || m.LatencyCount != 4096 || m.P99MS != 0 {
		t.Fatalf("bounded integer or zero duration semantics failed: %+v", m)
	}
	if r.Report.CriteriaMet {
		t.Fatal("all failures met configured criteria")
	}
}
