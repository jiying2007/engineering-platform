package production

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/strictjson"
)

const (
	OperationalSLOVersion          = 3
	operationalSLOMaxRecords       = 4096
	operationalSLOMaxWindow        = 31 * 24 * time.Hour
	operationalSLIMaxValue   int64 = 1 << 60
)

// OperationalSLOInput is an explicitly selected v3 readback manifest. It does
// not change the historical latency-only v2 input or report semantics.
type OperationalSLOInput struct {
	Version              int               `json:"version"`
	CriteriaDigest       string            `json:"criteria_digest"`
	SourceDigests        map[string]string `json:"source_digests"`
	ResourceSourceDigest string            `json:"resource_source_digest"`
}

// OperationalSLOCriteria supplies all comparison values. Its declared time
// is checked for consistency only; offline bytes cannot prove pre-registration.
type OperationalSLOCriteria struct {
	Version              int                                       `json:"version"`
	RunID                string                                    `json:"run_id"`
	SubjectDigest        string                                    `json:"subject_digest"`
	DeploymentDigest     string                                    `json:"deployment_digest"`
	WorkloadDigest       string                                    `json:"workload_digest"`
	CollectorDigest      string                                    `json:"collector_digest"`
	DeclaredAt           time.Time                                 `json:"declared_at"`
	StartedAt            time.Time                                 `json:"started_at"`
	CompletedAt          time.Time                                 `json:"completed_at"`
	MinimumSoakMS        *int64                                    `json:"minimum_soak_ms"`
	MaximumSourceAgeMS   *int64                                    `json:"maximum_source_age_ms"`
	MaximumResourceGapMS *int64                                    `json:"maximum_resource_gap_ms"`
	Metrics              map[string]OperationalSLIMetricCriteria   `json:"metrics"`
	Resources            map[string]OperationalSLIResourceCriteria `json:"resources"`
}

// Pointers distinguish omitted/null limits from deliberately configured zero.
type OperationalSLIMetricCriteria struct {
	MinimumAttempts              *int64 `json:"minimum_attempts"`
	MaximumNonSuccessPPM         *int64 `json:"maximum_non_success_ppm"`
	MaximumP99MS                 *int64 `json:"maximum_p99_ms"`
	MaximumLatencyMS             *int64 `json:"maximum_latency_ms"`
	MinimumSuccessMilliPerSecond *int64 `json:"minimum_success_milli_per_second"`
}

type OperationalSLIResourceCriteria struct {
	MaximumPeak              *int64 `json:"maximum_peak"`
	MaximumPositiveNetGrowth *int64 `json:"maximum_positive_net_growth"`
}

// Source bindings are repeated in every raw source and checked against one
// exact criterion document and the independently supplied expected identities.
type OperationalSLISourceBinding struct {
	Version          int       `json:"version"`
	RunID            string    `json:"run_id"`
	SubjectDigest    string    `json:"subject_digest"`
	DeploymentDigest string    `json:"deployment_digest"`
	WorkloadDigest   string    `json:"workload_digest"`
	CollectorDigest  string    `json:"collector_digest"`
	CriteriaDigest   string    `json:"criteria_digest"`
	StartedAt        time.Time `json:"started_at"`
	CompletedAt      time.Time `json:"completed_at"`
}

type OperationalSLIEvent struct {
	Sequence   int64     `json:"sequence"`
	ID         string    `json:"id"`
	ObservedAt time.Time `json:"observed_at"`
	Outcome    string    `json:"outcome"`
	LatencyMS  *int64    `json:"latency_ms"`
}

type OperationalSLIEventSource struct {
	OperationalSLISourceBinding
	Kind   string                `json:"kind"`
	Name   string                `json:"name"`
	Events []OperationalSLIEvent `json:"events"`
}

type OperationalSLIResourceSample struct {
	Sequence   int64             `json:"sequence"`
	ObservedAt time.Time         `json:"observed_at"`
	Values     map[string]*int64 `json:"values"`
}

type OperationalSLIResourceSource struct {
	OperationalSLISourceBinding
	Kind    string                         `json:"kind"`
	Samples []OperationalSLIResourceSample `json:"samples"`
}

type OperationalSLIMetric struct {
	Name                       string `json:"name"`
	Total                      int64  `json:"total"`
	Success                    int64  `json:"success"`
	Failure                    int64  `json:"failure"`
	Cancelled                  int64  `json:"cancelled"`
	Aborted                    int64  `json:"aborted"`
	Timeout                    int64  `json:"timeout"`
	Dropped                    int64  `json:"dropped"`
	NonSuccessPPMUpper         int64  `json:"non_success_ppm_upper"`
	SuccessMilliPerSecondLower int64  `json:"success_milli_per_second_lower"`
	LatencyCount               int    `json:"latency_count"`
	MinMS                      int64  `json:"min_ms"`
	P50MS                      int64  `json:"p50_ms"`
	P95MS                      int64  `json:"p95_ms"`
	P99MS                      int64  `json:"p99_ms"`
	MaxMS                      int64  `json:"max_ms"`
	SourceDigest               string `json:"source_digest"`
}

type OperationalSLIResource struct {
	Name              string `json:"name"`
	Samples           int    `json:"samples"`
	First             int64  `json:"first"`
	Last              int64  `json:"last"`
	Peak              int64  `json:"peak"`
	NetChange         int64  `json:"net_change"`
	PositiveNetGrowth int64  `json:"positive_net_growth"`
}

type OperationalSLIFailure struct {
	Name     string `json:"name"`
	Code     string `json:"code"`
	Observed int64  `json:"observed"`
	Limit    int64  `json:"limit"`
}

type OperationalSLOReport struct {
	Version                      int                      `json:"version"`
	Status                       string                   `json:"status"`
	SourceBytesVerified          bool                     `json:"source_bytes_verified"`
	CriteriaMet                  bool                     `json:"criteria_met"`
	CollectorAuthenticated       bool                     `json:"collector_authenticated"`
	PreRegistrationAuthenticated bool                     `json:"pre_registration_authenticated"`
	QualificationGranted         bool                     `json:"qualification_granted"`
	ProductionQualified          bool                     `json:"production_qualified"`
	ProviderMeasurementPresent   bool                     `json:"provider_measurement_present"`
	EvaluatedAt                  time.Time                `json:"evaluated_at"`
	CriteriaDigest               string                   `json:"criteria_digest"`
	ResourceSourceDigest         string                   `json:"resource_source_digest"`
	Criteria                     OperationalSLOCriteria   `json:"criteria"`
	Metrics                      []OperationalSLIMetric   `json:"metrics"`
	Resources                    []OperationalSLIResource `json:"resources"`
	Failures                     []OperationalSLIFailure  `json:"failures"`
}

type OperationalSLOEnvelope struct {
	Report       OperationalSLOReport `json:"report"`
	ReportDigest string               `json:"report_digest"`
}

var operationalSLIResources = []string{
	"cpu_millicores", "rss_bytes", "disk_bytes", "fd_count", "goroutine_count", "thread_count", "connection_count",
}

func operationalSLIMetricName(name string) bool {
	if validSLOName(name) {
		return true
	}
	switch name {
	case "postgres_queue", "postgres_connection_acquisition", "postgres_lock", "postgres_statement", "postgres_commit", "postgres_complete_request":
		return true
	}
	return false
}

func boundedSLIIdentity(value string) bool {
	if value == "" || len(value) > 256 || strings.TrimSpace(value) != value {
		return false
	}
	for _, ch := range value {
		if ch < 0x20 || ch == 0x7f {
			return false
		}
	}
	return true
}

func requiredSLILimit(value *int64, minimum, maximum int64) bool {
	return value != nil && *value >= minimum && *value <= maximum
}

func (c OperationalSLOCriteria) validate(runID, subject, deployment string, now time.Time) error {
	if c.Version != 1 || !boundedSLIIdentity(runID) || c.RunID != runID || !canonical.ValidDigest(subject) || c.SubjectDigest != subject || !canonical.ValidDigest(deployment) || c.DeploymentDigest != deployment || !canonical.ValidDigest(c.WorkloadDigest) || !canonical.ValidDigest(c.CollectorDigest) {
		return fmt.Errorf("operational SLI criteria identity mismatch")
	}
	window := c.CompletedAt.Sub(c.StartedAt)
	if c.DeclaredAt.IsZero() || c.StartedAt.IsZero() || c.CompletedAt.IsZero() || now.IsZero() || !c.DeclaredAt.Before(c.StartedAt) || window < time.Millisecond || window > operationalSLOMaxWindow || window%time.Millisecond != 0 || c.CompletedAt.After(now) {
		return fmt.Errorf("invalid operational SLI declared measurement window")
	}
	windowMS := window.Milliseconds()
	if !requiredSLILimit(c.MinimumSoakMS, 1, windowMS) || !requiredSLILimit(c.MaximumResourceGapMS, 1, windowMS) || !requiredSLILimit(c.MaximumSourceAgeMS, 1, operationalSLOMaxWindow.Milliseconds()) || now.Sub(c.CompletedAt) > time.Duration(*c.MaximumSourceAgeMS)*time.Millisecond {
		return fmt.Errorf("missing, contradictory or stale operational SLI window limits")
	}
	if len(c.Metrics) < len(nonProviderSLOs) || len(c.Metrics) > 13 || len(c.Resources) != len(operationalSLIResources) {
		return fmt.Errorf("complete bounded operational SLI criteria required")
	}
	for _, name := range nonProviderSLOs {
		if _, ok := c.Metrics[name]; !ok {
			return fmt.Errorf("missing required operational SLI metric %s", name)
		}
	}
	for name, metric := range c.Metrics {
		if !operationalSLIMetricName(name) || !requiredSLILimit(metric.MinimumAttempts, 1, operationalSLOMaxRecords) || !requiredSLILimit(metric.MaximumNonSuccessPPM, 0, 1_000_000) || !requiredSLILimit(metric.MaximumP99MS, 0, windowMS) || !requiredSLILimit(metric.MaximumLatencyMS, 0, windowMS) || *metric.MaximumP99MS > *metric.MaximumLatencyMS || !requiredSLILimit(metric.MinimumSuccessMilliPerSecond, 0, operationalSLIMaxValue) {
			return fmt.Errorf("invalid or missing operational SLI metric limits for %s", name)
		}
	}
	for _, name := range operationalSLIResources {
		limit, ok := c.Resources[name]
		if !ok || !requiredSLILimit(limit.MaximumPeak, 0, operationalSLIMaxValue) || !requiredSLILimit(limit.MaximumPositiveNetGrowth, 0, operationalSLIMaxValue) {
			return fmt.Errorf("invalid or missing operational SLI resource limits for %s", name)
		}
	}
	return nil
}

func (s OperationalSLISourceBinding) matches(c OperationalSLOCriteria, digest string) bool {
	return s.Version == 1 && s.RunID == c.RunID && s.SubjectDigest == c.SubjectDigest && s.DeploymentDigest == c.DeploymentDigest && s.WorkloadDigest == c.WorkloadDigest && s.CollectorDigest == c.CollectorDigest && s.CriteriaDigest == digest && s.StartedAt.Equal(c.StartedAt) && s.CompletedAt.Equal(c.CompletedAt)
}

// ReadOperationalSLOManifest applies the same bounded, regular-file read as
// retained objects. The manifest itself has no externally supplied digest.
func ReadOperationalSLOManifest(path string) ([]byte, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	parent := filepath.Dir(abs)
	if err := safeDirectory(parent); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(parent)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	return readOperationalSLIBytes(root, filepath.Base(abs))
}

func sameOperationalSLIFile(before, after os.FileInfo) bool {
	return os.SameFile(before, after) && after.Mode() == before.Mode() && after.Size() == before.Size() && after.ModTime().Equal(before.ModTime())
}

// The platform opener cannot block on a substituted FIFO or follow a symlink.
// LimitReader also bounds reads if a regular source changes after its stat.
func readOperationalSLIBytes(root *os.Root, name string) ([]byte, error) {
	before, err := root.Lstat(name)
	if err != nil || !before.Mode().IsRegular() || before.Mode().Perm()&0o022 != 0 || before.Size() <= 0 || before.Size() > strictjson.MaxBytes {
		return nil, fmt.Errorf("operational SLI source unavailable or unsafe")
	}
	f, err := openOperationalSLIFile(root, name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !sameOperationalSLIFile(before, opened) {
		return nil, fmt.Errorf("operational SLI source changed before read")
	}
	data, err := io.ReadAll(io.LimitReader(f, strictjson.MaxBytes+1))
	if err != nil || int64(len(data)) != before.Size() {
		return nil, fmt.Errorf("operational SLI source bytes changed during read")
	}
	readInfo, err := f.Stat()
	if err != nil || !sameOperationalSLIFile(before, readInfo) {
		return nil, fmt.Errorf("operational SLI opened descriptor changed during read")
	}
	after, err := root.Lstat(name)
	if err != nil || !sameOperationalSLIFile(before, after) {
		return nil, fmt.Errorf("operational SLI source path changed during read")
	}
	return data, nil
}

func readSLIObject(root *os.Root, digest string, destination any) error {
	if !canonical.ValidDigest(digest) {
		return fmt.Errorf("valid operational SLI source digest required")
	}
	name := strings.TrimPrefix(digest, "sha256:") + ".json"
	data, err := readOperationalSLIBytes(root, name)
	if err != nil {
		return err
	}
	if canonical.BytesDigest(data) != digest {
		return fmt.Errorf("operational SLI source digest mismatch")
	}
	if err := strictjson.Decode(data, destination); err != nil {
		return fmt.Errorf("strict operational SLI source: %w", err)
	}
	return nil
}

// EvaluateOperationalSLO verifies bounded retained bytes and compares them to
// explicit criteria. Neither source authentication nor production acceptance
// is inferred from a digest, declared timestamp or locally successful criteria.
func EvaluateOperationalSLO(input OperationalSLOInput, sourceRoot, runID, subject, deployment string, now time.Time) (OperationalSLOEnvelope, error) {
	if input.Version != OperationalSLOVersion || len(input.SourceDigests) < len(nonProviderSLOs) || len(input.SourceDigests) > 13 {
		return OperationalSLOEnvelope{}, fmt.Errorf("bounded operational SLI version 3 manifest required")
	}
	if err := safeDirectory(sourceRoot); err != nil {
		return OperationalSLOEnvelope{}, err
	}
	root, err := os.OpenRoot(sourceRoot)
	if err != nil {
		return OperationalSLOEnvelope{}, err
	}
	defer root.Close()
	var criteria OperationalSLOCriteria
	if err := readSLIObject(root, input.CriteriaDigest, &criteria); err != nil {
		return OperationalSLOEnvelope{}, err
	}
	if err := criteria.validate(runID, subject, deployment, now); err != nil {
		return OperationalSLOEnvelope{}, err
	}
	if len(input.SourceDigests) != len(criteria.Metrics) {
		return OperationalSLOEnvelope{}, fmt.Errorf("operational SLI metric coverage differs from criteria")
	}
	seenDigests := map[string]bool{input.CriteriaDigest: true}
	names := make([]string, 0, len(input.SourceDigests))
	for name, digest := range input.SourceDigests {
		if _, ok := criteria.Metrics[name]; !ok || !canonical.ValidDigest(digest) || seenDigests[digest] {
			return OperationalSLOEnvelope{}, fmt.Errorf("unknown or duplicated operational SLI source")
		}
		seenDigests[digest] = true
		names = append(names, name)
	}
	if !canonical.ValidDigest(input.ResourceSourceDigest) || seenDigests[input.ResourceSourceDigest] {
		return OperationalSLOEnvelope{}, fmt.Errorf("distinct operational SLI resource source required")
	}
	sort.Strings(names)
	report := OperationalSLOReport{Version: OperationalSLOVersion, Status: "SOURCE_BYTES_EVALUATED", SourceBytesVerified: true, EvaluatedAt: now.UTC(), CriteriaDigest: input.CriteriaDigest, ResourceSourceDigest: input.ResourceSourceDigest, Criteria: criteria, Failures: []OperationalSLIFailure{}}
	_, report.ProviderMeasurementPresent = criteria.Metrics[providerSLO]
	for _, name := range names {
		var source OperationalSLIEventSource
		if err := readSLIObject(root, input.SourceDigests[name], &source); err != nil {
			return OperationalSLOEnvelope{}, fmt.Errorf("%s: %w", name, err)
		}
		if !source.OperationalSLISourceBinding.matches(criteria, input.CriteriaDigest) || source.Kind != "request_events" || source.Name != name {
			return OperationalSLOEnvelope{}, fmt.Errorf("operational SLI event source identity mismatch for %s", name)
		}
		metric, err := summarizeOperationalEvents(source, input.SourceDigests[name])
		if err != nil {
			return OperationalSLOEnvelope{}, err
		}
		report.Metrics = append(report.Metrics, metric)
		limits := criteria.Metrics[name]
		if metric.Dropped != 0 {
			report.fail(name, "DROPPED_EVENTS", metric.Dropped, 0)
		}
		if metric.LatencyCount == 0 {
			report.fail(name, "NO_OBSERVED_LATENCY", 0, 1)
		}
		if metric.Total < *limits.MinimumAttempts {
			report.fail(name, "ATTEMPTS_BELOW_MINIMUM", metric.Total, *limits.MinimumAttempts)
		}
		if metric.NonSuccessPPMUpper > *limits.MaximumNonSuccessPPM {
			report.fail(name, "NON_SUCCESS_ABOVE_MAXIMUM", metric.NonSuccessPPMUpper, *limits.MaximumNonSuccessPPM)
		}
		if metric.P99MS > *limits.MaximumP99MS {
			report.fail(name, "P99_ABOVE_MAXIMUM", metric.P99MS, *limits.MaximumP99MS)
		}
		if metric.MaxMS > *limits.MaximumLatencyMS {
			report.fail(name, "LATENCY_ABOVE_MAXIMUM", metric.MaxMS, *limits.MaximumLatencyMS)
		}
		if metric.SuccessMilliPerSecondLower < *limits.MinimumSuccessMilliPerSecond {
			report.fail(name, "SUCCESS_THROUGHPUT_BELOW_MINIMUM", metric.SuccessMilliPerSecondLower, *limits.MinimumSuccessMilliPerSecond)
		}
	}
	var resources OperationalSLIResourceSource
	if err := readSLIObject(root, input.ResourceSourceDigest, &resources); err != nil {
		return OperationalSLOEnvelope{}, err
	}
	if !resources.OperationalSLISourceBinding.matches(criteria, input.CriteriaDigest) || resources.Kind != "resource_samples" {
		return OperationalSLOEnvelope{}, fmt.Errorf("operational SLI resource source identity mismatch")
	}
	report.Resources, err = summarizeOperationalResources(resources, *criteria.MaximumResourceGapMS)
	if err != nil {
		return OperationalSLOEnvelope{}, err
	}
	for _, resource := range report.Resources {
		limit := criteria.Resources[resource.Name]
		if resource.Peak > *limit.MaximumPeak {
			report.fail(resource.Name, "RESOURCE_PEAK_ABOVE_MAXIMUM", resource.Peak, *limit.MaximumPeak)
		}
		if resource.PositiveNetGrowth > *limit.MaximumPositiveNetGrowth {
			report.fail(resource.Name, "RESOURCE_NET_GROWTH_ABOVE_MAXIMUM", resource.PositiveNetGrowth, *limit.MaximumPositiveNetGrowth)
		}
	}
	report.CriteriaMet = len(report.Failures) == 0
	digest, err := canonical.Digest(report)
	if err != nil {
		return OperationalSLOEnvelope{}, err
	}
	return OperationalSLOEnvelope{Report: report, ReportDigest: digest}, nil
}

func (r *OperationalSLOReport) fail(name, code string, observed, limit int64) {
	r.Failures = append(r.Failures, OperationalSLIFailure{Name: name, Code: code, Observed: observed, Limit: limit})
}

func summarizeOperationalEvents(source OperationalSLIEventSource, digest string) (OperationalSLIMetric, error) {
	if len(source.Events) == 0 || len(source.Events) > operationalSLOMaxRecords {
		return OperationalSLIMetric{}, fmt.Errorf("bounded nonempty operational SLI event set required")
	}
	metric := OperationalSLIMetric{Name: source.Name, Total: int64(len(source.Events)), SourceDigest: digest}
	ids := map[string]bool{}
	previous := source.StartedAt
	latencies := make([]int64, 0, len(source.Events))
	for i, event := range source.Events {
		if event.Sequence != int64(i+1) || !boundedSLIIdentity(event.ID) || ids[event.ID] || event.ObservedAt.Before(previous) || event.ObservedAt.After(source.CompletedAt) {
			return OperationalSLIMetric{}, fmt.Errorf("invalid operational SLI event identity, sequence or time")
		}
		ids[event.ID] = true
		previous = event.ObservedAt
		switch event.Outcome {
		case "success":
			metric.Success++
		case "failure":
			metric.Failure++
		case "cancelled":
			metric.Cancelled++
		case "aborted":
			metric.Aborted++
		case "timeout":
			metric.Timeout++
		case "dropped":
			metric.Dropped++
		default:
			return OperationalSLIMetric{}, fmt.Errorf("unknown operational SLI event outcome")
		}
		if event.Outcome == "dropped" {
			if event.LatencyMS != nil {
				return OperationalSLIMetric{}, fmt.Errorf("dropped event cannot fabricate a latency")
			}
			continue
		}
		if event.LatencyMS == nil || *event.LatencyMS < 0 || *event.LatencyMS > event.ObservedAt.Sub(source.StartedAt).Milliseconds() {
			return OperationalSLIMetric{}, fmt.Errorf("operational SLI event latency missing or exceeds captured window")
		}
		latencies = append(latencies, *event.LatencyMS)
	}
	// Counts are bounded at 4096 and windows at 31 days: these integer products
	// cannot overflow int64. Rounding is conservative for acceptance comparisons.
	nonSuccess := metric.Total - metric.Success
	metric.NonSuccessPPMUpper = (nonSuccess*1_000_000 + metric.Total - 1) / metric.Total
	metric.SuccessMilliPerSecondLower = metric.Success * 1_000_000 / source.CompletedAt.Sub(source.StartedAt).Milliseconds()
	metric.LatencyCount = len(latencies)
	if len(latencies) != 0 {
		sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
		metric.MinMS, metric.P50MS, metric.P95MS, metric.P99MS, metric.MaxMS = latencies[0], percentile(latencies, 50), percentile(latencies, 95), percentile(latencies, 99), latencies[len(latencies)-1]
	}
	return metric, nil
}

func summarizeOperationalResources(source OperationalSLIResourceSource, maximumGapMS int64) ([]OperationalSLIResource, error) {
	if len(source.Samples) < 2 || len(source.Samples) > operationalSLOMaxRecords {
		return nil, fmt.Errorf("bounded operational SLI resource sequence required")
	}
	previous := source.StartedAt
	maximumGap := time.Duration(maximumGapMS) * time.Millisecond
	for i, sample := range source.Samples {
		if sample.Sequence != int64(i+1) || sample.ObservedAt.Before(source.StartedAt) || sample.ObservedAt.After(source.CompletedAt) || (i > 0 && !sample.ObservedAt.After(previous)) || sample.ObservedAt.Sub(previous) > maximumGap || len(sample.Values) != len(operationalSLIResources) {
			return nil, fmt.Errorf("operational SLI resource samples missing, unordered or outside maximum gap")
		}
		for _, name := range operationalSLIResources {
			value, ok := sample.Values[name]
			if !ok || value == nil || *value < 0 || *value > operationalSLIMaxValue {
				return nil, fmt.Errorf("invalid or missing operational SLI resource value")
			}
		}
		previous = sample.ObservedAt
	}
	if source.CompletedAt.Sub(previous) > maximumGap {
		return nil, fmt.Errorf("operational SLI resource tail missing")
	}
	result := make([]OperationalSLIResource, 0, len(operationalSLIResources))
	for _, name := range operationalSLIResources {
		r := OperationalSLIResource{Name: name, Samples: len(source.Samples), First: *source.Samples[0].Values[name], Last: *source.Samples[len(source.Samples)-1].Values[name]}
		for _, sample := range source.Samples {
			if value := *sample.Values[name]; value > r.Peak {
				r.Peak = value
			}
		}
		r.NetChange = r.Last - r.First
		if r.NetChange > 0 {
			r.PositiveNetGrowth = r.NetChange
		}
		result = append(result, r)
	}
	return result, nil
}
