package production

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/strictjson"
)

// SLOSource is a retained collector record, not a provider credential or a
// production decision. Offline readback proves bytes and subject binding only;
// authenticity remains with the existing Evidence/Verification/Review chain.
type SLOSource struct {
	Version         int       `json:"version"`
	Name            string    `json:"name"`
	RunID           string    `json:"run_id"`
	SubjectDigest   string    `json:"subject_digest"`
	Procedure       string    `json:"procedure"`
	CollectorDigest string    `json:"collector_digest"`
	StartedAt       time.Time `json:"started_at"`
	CompletedAt     time.Time `json:"completed_at"`
	SamplesMS       []int64   `json:"samples_ms"`
}

// VerifySLOReport consumes already retained content-addressed source bytes.
// It cannot qualify a provider, set latency targets, or grant production use.
func VerifySLOReport(observations []SLOObservation, sourceRoot, runID, subject string) (SLOReportEnvelope, error) {
	envelope, err := BuildSLOReport(observations)
	if err != nil {
		return envelope, err
	}
	if runID == "" || len(runID) > 256 || strings.TrimSpace(runID) != runID || strings.ContainsAny(runID, "\r\n\x00") || !canonical.ValidDigest(subject) {
		return SLOReportEnvelope{}, fmt.Errorf("exact run and subject identity required")
	}
	if err := safeDirectory(sourceRoot); err != nil {
		return SLOReportEnvelope{}, err
	}
	root, err := os.OpenRoot(sourceRoot)
	if err != nil {
		return SLOReportEnvelope{}, err
	}
	defer root.Close()
	for _, observation := range observations {
		name := strings.TrimPrefix(observation.SourceDigest, "sha256:") + ".json"
		if filepath.Base(name) != name {
			return SLOReportEnvelope{}, fmt.Errorf("invalid source locator")
		}
		before, e := root.Lstat(name)
		if e != nil || !before.Mode().IsRegular() || before.Mode().Perm()&0o022 != 0 || before.Size() <= 0 || before.Size() > 1<<20 {
			return SLOReportEnvelope{}, fmt.Errorf("required SLO source unavailable or unsafe")
		}
		data, e := root.ReadFile(name)
		if e != nil || int64(len(data)) != before.Size() || canonical.BytesDigest(data) != observation.SourceDigest {
			return SLOReportEnvelope{}, fmt.Errorf("SLO source digest mismatch")
		}
		after, e := root.Lstat(name)
		if e != nil || !os.SameFile(before, after) || after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) {
			return SLOReportEnvelope{}, fmt.Errorf("SLO source changed during read")
		}
		var source SLOSource
		if strictjson.Decode(data, &source) != nil || source.Version != 1 || source.Name != observation.Name || source.RunID != runID || source.SubjectDigest != subject || source.Procedure != "engineering-platform.slo."+observation.Name+".v1" || !canonical.ValidDigest(source.CollectorDigest) || source.StartedAt.IsZero() || !source.CompletedAt.After(source.StartedAt) || !slices.Equal(source.SamplesMS, observation.SamplesMS) {
			return SLOReportEnvelope{}, fmt.Errorf("SLO source does not bind exact run, subject, procedure, window and samples")
		}
		window := source.CompletedAt.Sub(source.StartedAt)
		for _, sample := range source.SamplesMS {
			if time.Duration(sample)*time.Millisecond > window {
				return SLOReportEnvelope{}, fmt.Errorf("SLO sample exceeds captured source window")
			}
		}
	}
	envelope.Report.Status = SLOStatusSourceVerified
	envelope.Report.SourceBytesVerified = true
	envelope.Report.RunID = runID
	envelope.Report.SubjectDigest = subject
	envelope.ReportDigest, err = canonical.Digest(envelope.Report)
	return envelope, err
}
