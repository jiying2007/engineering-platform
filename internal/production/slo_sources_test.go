package production

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/canonical"
)

func TestAuditSyntheticSamplesNeverGrantQualification(t *testing.T) {
	var observations []SLOObservation
	for _, name := range append(append([]string{}, nonProviderSLOs...), providerSLO) {
		observations = append(observations, SLOObservation{Name: name, SamplesMS: []int64{1}, SourceDigest: "sha256:" + strings.Repeat("0", 64)})
	}
	result, err := BuildSLOReport(observations)
	if err != nil {
		t.Fatal(err)
	}
	if result.Report.Status != SLOStatusUnverified || result.Report.SourceBytesVerified || result.Report.QualificationGranted {
		t.Fatal("fabricated samples became qualification")
	}
	if _, err := VerifySLOReport(observations, t.TempDir(), "test-run", "sha256:"+strings.Repeat("a", 64)); err == nil {
		t.Fatal("missing source with fabricated digest accepted")
	}
}

func sourceFixture(t *testing.T) ([]SLOObservation, string, string) {
	t.Helper()
	dir := t.TempDir()
	subject := canonical.BytesDigest([]byte("test-subject"))
	var observations []SLOObservation
	for _, name := range append(append([]string{}, nonProviderSLOs...), providerSLO) {
		source := SLOSource{Version: 1, Name: name, RunID: "test-run", SubjectDigest: subject, Procedure: "engineering-platform.slo." + name + ".v1", CollectorDigest: canonical.BytesDigest([]byte("test-only-collector")), StartedAt: time.Unix(1, 0).UTC(), CompletedAt: time.Unix(2, 0).UTC(), SamplesMS: []int64{1, 5, 2}}
		data, err := json.Marshal(source)
		if err != nil {
			t.Fatal(err)
		}
		digest := canonical.BytesDigest(data)
		if err := os.WriteFile(filepath.Join(dir, digest[7:]+".json"), data, 0o600); err != nil {
			t.Fatal(err)
		}
		observations = append(observations, SLOObservation{Name: name, SamplesMS: source.SamplesMS, SourceDigest: digest})
	}
	return observations, dir, subject
}
func TestSLOSourceReadbackIsNotOperationalQualification(t *testing.T) {
	observations, dir, subject := sourceFixture(t)
	result, err := VerifySLOReport(observations, dir, "test-run", subject)
	if err != nil {
		t.Fatal(err)
	}
	if result.Report.Status != SLOStatusSourceVerified || !result.Report.SourceBytesVerified || result.Report.QualificationGranted {
		t.Fatal("incorrect source/qualification separation")
	}
	for _, kind := range []string{"run", "subject", "samples", "tamper", "missing", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			obs, dir, subject := sourceFixture(t)
			run := "test-run"
			path := filepath.Join(dir, obs[0].SourceDigest[7:]+".json")
			switch kind {
			case "run":
				run = "other"
			case "subject":
				subject = canonical.BytesDigest([]byte("other"))
			case "samples":
				obs[0].SamplesMS = []int64{999}
			case "tamper":
				os.WriteFile(path, []byte("{}"), 0o600)
			case "missing":
				os.Remove(path)
			case "symlink":
				data, _ := os.ReadFile(path)
				other := filepath.Join(t.TempDir(), "source.json")
				os.WriteFile(other, data, 0o600)
				os.Remove(path)
				os.Symlink(other, path)
			}
			if _, err := VerifySLOReport(obs, dir, run, subject); err == nil {
				t.Fatalf("%s accepted", kind)
			}
		})
	}
}
