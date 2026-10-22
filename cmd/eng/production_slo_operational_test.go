package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/production"
)

func operationalCLILimit(n int64) *int64 { return &n }

// All records below are synthetic. No collector, deployment or credential is
// contacted or authenticated. The actual built CLI must preserve that boundary.
func operationalCLIFixture(t *testing.T, fail bool) ([]string, string, string) {
	t.Helper()
	root := t.TempDir()
	start := time.Now().UTC().Truncate(time.Millisecond).Add(-20 * time.Second)
	c := production.OperationalSLOCriteria{Version: 1, RunID: "synthetic-cli-run", SubjectDigest: canonical.BytesDigest([]byte("synthetic-cli-subject")), DeploymentDigest: canonical.BytesDigest([]byte("synthetic-cli-deployment")), WorkloadDigest: canonical.BytesDigest([]byte("synthetic-cli-workload")), CollectorDigest: canonical.BytesDigest([]byte("synthetic-cli-collector")), DeclaredAt: start.Add(-time.Hour), StartedAt: start, CompletedAt: start.Add(10 * time.Second), MinimumSoakMS: operationalCLILimit(10000), MaximumSourceAgeMS: operationalCLILimit(600000), MaximumResourceGapMS: operationalCLILimit(10000), Metrics: map[string]production.OperationalSLIMetricCriteria{}, Resources: map[string]production.OperationalSLIResourceCriteria{}}
	names := []string{"control_api_roundtrip", "worker_admission", "worker_preparation", "engineering_run", "publication_ci_verification", "postgres_authority_restore"}
	resources := []string{"cpu_millicores", "rss_bytes", "disk_bytes", "fd_count", "goroutine_count", "thread_count", "connection_count"}
	for _, name := range names {
		c.Metrics[name] = production.OperationalSLIMetricCriteria{MinimumAttempts: operationalCLILimit(1), MaximumNonSuccessPPM: operationalCLILimit(0), MaximumP99MS: operationalCLILimit(10), MaximumLatencyMS: operationalCLILimit(10), MinimumSuccessMilliPerSecond: operationalCLILimit(100)}
	}
	for _, name := range resources {
		c.Resources[name] = production.OperationalSLIResourceCriteria{MaximumPeak: operationalCLILimit(10), MaximumPositiveNetGrowth: operationalCLILimit(0)}
	}
	write := func(value any) string {
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		digest := canonical.BytesDigest(raw)
		if err := os.WriteFile(filepath.Join(root, digest[7:]+".json"), raw, 0o600); err != nil {
			t.Fatal(err)
		}
		return digest
	}
	input := production.OperationalSLOInput{Version: 3, CriteriaDigest: write(c), SourceDigests: map[string]string{}}
	b := production.OperationalSLISourceBinding{Version: 1, RunID: c.RunID, SubjectDigest: c.SubjectDigest, DeploymentDigest: c.DeploymentDigest, WorkloadDigest: c.WorkloadDigest, CollectorDigest: c.CollectorDigest, CriteriaDigest: input.CriteriaDigest, StartedAt: c.StartedAt, CompletedAt: c.CompletedAt}
	var legacy []production.SLOObservation
	for i, name := range names {
		outcome := "success"
		if fail && i == 0 {
			outcome = "failure"
		}
		s := production.OperationalSLIEventSource{OperationalSLISourceBinding: b, Kind: "request_events", Name: name, Events: []production.OperationalSLIEvent{{Sequence: 1, ID: "one", ObservedAt: start.Add(time.Second), Outcome: outcome, LatencyMS: operationalCLILimit(1)}}}
		input.SourceDigests[name] = write(s)
		legacy = append(legacy, production.SLOObservation{Name: name, SamplesMS: []int64{1, 2, 3}, SourceDigest: canonical.BytesDigest([]byte("legacy-test-only"))})
	}
	r := production.OperationalSLIResourceSource{OperationalSLISourceBinding: b, Kind: "resource_samples"}
	for i := 0; i < 2; i++ {
		values := map[string]*int64{}
		for _, name := range resources {
			values[name] = operationalCLILimit(1)
		}
		r.Samples = append(r.Samples, production.OperationalSLIResourceSample{Sequence: int64(i + 1), ObservedAt: start.Add(time.Duration(i) * 10 * time.Second), Values: values})
	}
	input.ResourceSourceDigest = write(r)
	manifest := filepath.Join(root, "manifest.json")
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifest, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	legacyPath := filepath.Join(root, "legacy.json")
	raw, err = json.Marshal(productionSLOInput{Version: 2, Observations: legacy})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacyPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return []string{"production-slo-report", "--operational", manifest, "--source-root", root, "--run", c.RunID, "--subject", c.SubjectDigest, "--deployment", c.DeploymentDigest}, legacyPath, manifest
}

type operationalSLIFailingWriter struct{ short bool }

func (w operationalSLIFailingWriter) Write(data []byte) (int, error) {
	if w.short {
		return len(data) - 1, nil
	}
	return 0, errors.New("sensitive writer details must not reach stderr")
}

func TestProductionOperationalSLOOutputFailureIsRejected(t *testing.T) {
	for _, short := range []bool{true, false} {
		err := writeOperationalSLOReport(operationalSLIFailingWriter{short: short}, production.OperationalSLOEnvelope{})
		if err == nil || err.Error() != "operational SLI report output failed" {
			t.Fatalf("short=%v: write failure was lost or disclosed: %v", short, err)
		}
	}
}

func TestProductionOperationalSLOThroughActualCLI(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("v3 source readback requires the Linux no-follow, nonblocking opener")
	}
	goBinary, err := exec.LookPath("go")
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(t.TempDir(), "eng")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if out, err := exec.CommandContext(ctx, goBinary, "build", "-trimpath", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build actual CLI: %v %s", err, out)
	}
	cases := []struct {
		name        string
		fail        bool
		mode        string
		wantCode    int
		wantVersion int
		criteria    bool
	}{
		{name: "report", wantVersion: 3, criteria: true},
		{name: "criteria-pass", mode: "gate", wantVersion: 3, criteria: true},
		{name: "criteria-failure-report", fail: true, wantVersion: 3},
		{name: "criteria-failure-exit", fail: true, mode: "gate", wantCode: 1, wantVersion: 3},
		{name: "identity-drift", mode: "identity", wantCode: 1},
		{name: "both-inputs", mode: "both", wantCode: 1},
		{name: "missing-source-root", mode: "missing", wantCode: 1},
		{name: "duplicate-json", mode: "duplicate", wantCode: 1},
		{name: "output-full", mode: "full", wantCode: 1},
		{name: "legacy-v2", mode: "legacy", wantVersion: 2},
		{name: "legacy-unverified-gate", mode: "legacy-gate", wantCode: 1, wantVersion: 2},
		{name: "legacy-rejects-operational-flag", mode: "legacy-mixed", wantCode: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			args, legacy, manifest := operationalCLIFixture(t, tc.fail)
			switch tc.mode {
			case "gate":
				args = append(args, "--require-criteria-met")
			case "identity":
				args[len(args)-1] = canonical.BytesDigest([]byte("other-deployment"))
			case "both":
				args = append(args, "--observations", legacy)
			case "missing":
				args = []string{"production-slo-report", "--operational", manifest}
			case "duplicate":
				raw, err := os.ReadFile(manifest)
				if err != nil {
					t.Fatal(err)
				}
				raw = append([]byte(`{"version":3,`), raw[1:]...)
				if err := os.WriteFile(manifest, raw, 0o600); err != nil {
					t.Fatal(err)
				}
			case "legacy":
				args = []string{"production-slo-report", "--observations", legacy}
			case "legacy-gate":
				args = []string{"production-slo-report", "--observations", legacy, "--require-verified"}
			case "legacy-mixed":
				args = []string{"production-slo-report", "--observations", legacy, "--require-criteria-met"}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, bin, args...)
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			if tc.mode == "full" {
				full, err := os.OpenFile("/dev/full", os.O_WRONLY, 0)
				if err != nil {
					t.Fatal(err)
				}
				defer full.Close()
				cmd.Stdout = full
			}
			err := cmd.Run()
			code := 0
			if err != nil {
				if exit, ok := err.(*exec.ExitError); ok {
					code = exit.ExitCode()
				} else {
					t.Fatal(err)
				}
			}
			if code != tc.wantCode {
				t.Fatalf("exit %d want %d: %s", code, tc.wantCode, stderr.String())
			}
			if tc.mode == "full" && stderr.String() != "operational SLI report output failed\n" {
				t.Fatal("output failure did not return the fixed diagnostic")
			}
			if tc.wantVersion == 0 {
				if stdout.Len() != 0 {
					t.Fatal("invalid input emitted a report")
				}
				return
			}
			if tc.wantVersion == 3 {
				var envelope production.OperationalSLOEnvelope
				if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
					t.Fatal(err)
				}
				r := envelope.Report
				if r.Version != 3 || r.CriteriaMet != tc.criteria || !r.SourceBytesVerified || r.QualificationGranted || r.ProductionQualified || r.CollectorAuthenticated || r.PreRegistrationAuthenticated {
					t.Fatalf("unexpected operational boundary: %+v", r)
				}
			} else {
				var envelope production.SLOReportEnvelope
				if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
					t.Fatal(err)
				}
				if envelope.Report.Version != 2 || envelope.Report.Status != production.SLOStatusUnverified || envelope.Report.SourceBytesVerified || envelope.Report.QualificationGranted {
					t.Fatal("legacy v2 semantics changed")
				}
			}
		})
	}
}
