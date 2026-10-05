package workeragent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jiying2007/engineering-platform/internal/artifactset"
	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/offline"
	"github.com/jiying2007/engineering-platform/internal/sandbox"
	"github.com/jiying2007/engineering-platform/internal/strictjson"
)

// Both digests must be pinned independently of this command. Local producer
// consistency is not authenticated Core acceptance or fresh execution authority.
// Only deterministic producer filenames are read; no payload path is followed.
type OfflineCaptureRequest struct {
	Records, RunID, ExecutionID, PermitDigest, ReportDigest, Destination string
}

func (q OfflineCaptureRequest) Validate() error {
	if !readbackRunID.MatchString(q.RunID) || !readbackExecutionID.MatchString(q.ExecutionID) || !canonical.ValidDigest(q.PermitDigest) || !canonical.ValidDigest(q.ReportDigest) {
		return fmt.Errorf("exact Run, execution and external permit/report digests required")
	}
	for _, p := range []string{q.Records, q.Destination} {
		if !filepath.IsAbs(p) || filepath.Clean(p) != p || strings.ContainsAny(p, "\x00\r\n") {
			return fmt.Errorf("explicit canonical private paths required")
		}
	}
	rel, err := filepath.Rel(q.Records, q.Destination)
	if err != nil || rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("destination overlaps producer record directory")
	}
	return nil
}

type OfflineCaptureReport struct {
	Archive              artifactset.Report `json:"archive"`
	Selection            string             `json:"selection"`
	PermitDigest         string             `json:"permit_digest"`
	ReportDigest         string             `json:"report_digest"`
	ResultDigest         string             `json:"result_digest"`
	OutputContractDigest string             `json:"output_contract_digest"`
	OutputState          string             `json:"output_state"`
	OutputCount          int                `json:"output_count"`
	CommandExitCode      int                `json:"command_exit_code"`
	CoreObservation      string             `json:"core_observation"`
	FullRunBackup        bool               `json:"full_run_backup"`
	ExecutionAuthorized  bool               `json:"execution_authorized"`
	ProductionQualified  bool               `json:"production_qualified"`
}

func offlineReportID(id string) string { return sandbox.Hash([]byte(id + ":report"))[7:] }

func readOfflineCapture(ctx context.Context, q OfflineCaptureRequest) (offline.Permit, offline.Report, map[string][]byte, error) {
	var p offline.Permit
	var r offline.Report
	if err := ctx.Err(); err != nil {
		return p, r, nil, err
	}
	root, before, err := readbackDirectory(q.Records)
	if err != nil {
		return p, r, nil, err
	}
	defer root.Close()
	permitName := "offline-" + q.ExecutionID + ".json"
	reportName := "offline-" + offlineReportID(q.ExecutionID) + ".json"
	permitRaw, err := readbackFile(root, permitName)
	if err != nil || canonical.BytesDigest(permitRaw) != q.PermitDigest || strictjson.Decode(permitRaw, &p) != nil {
		return p, r, nil, fmt.Errorf("offline permit bytes or identity rejected")
	}
	if p.Token.ID != q.ExecutionID || p.Token.RunID != q.RunID || p.Check(p.Preparation.Admission.Worker, offline.Start{RunID: q.RunID, WorkerProfile: p.Token.WorkerProfile, Profile: p.Profile}) != nil || len(p.Profile.Outputs) == 0 {
		return p, r, nil, fmt.Errorf("frozen offline output permit required")
	}
	reportRaw, err := readbackFile(root, reportName)
	if err != nil || canonical.BytesDigest(reportRaw) != q.ReportDigest || strictjson.Decode(reportRaw, &r) != nil || r.Token != p.Token || r.Result.Validate(p.Profile) != nil || r.Result.BuildOutputs == nil {
		return p, r, nil, fmt.Errorf("offline report bytes or frozen outputs rejected")
	}
	now, err := os.Lstat(q.Records)
	if err != nil || !os.SameFile(before, now) {
		return p, r, nil, fmt.Errorf("offline record directory changed")
	}
	return p, r, map[string][]byte{permitName: permitRaw, reportName: reportRaw}, ctx.Err()
}

// CaptureOfflineBuildArtifacts exports all outputs already embedded in the
// original, fsynced local report. It never runs a producer or fetches a receipt.
// Failed-command reports retain their failure facts and ZERO output members.
// It reuses raw-set format and durability; nothing here grants SQL/Git/model or
// execution authority, and no Core confirmation is inferred from local bytes.
func CaptureOfflineBuildArtifacts(ctx context.Context, q OfflineCaptureRequest) (report OfflineCaptureReport, err error) {
	var zero OfflineCaptureReport
	if err = q.Validate(); err != nil {
		return zero, err
	}
	p, r, original, err := readOfflineCapture(ctx, q)
	if err != nil {
		return zero, err
	}
	plan := artifactset.Plan{Version: 1, Subject: artifactset.Subject{RunID: q.RunID, ExecutionID: q.ExecutionID, TaskDigest: p.Assignment.Intent.TaskDigest, InputDigest: p.Assignment.Intent.InputDigest, BaseCommit: p.Preparation.Facts.BaseCommit}}
	add := func(id, kind, path string, raw []byte) {
		plan.Members = append(plan.Members, artifactset.Input{Entry: artifactset.Entry{ID: id, Kind: kind, Size: int64(len(raw)), Digest: canonical.BytesDigest(raw)}, Path: path})
	}
	for name, raw := range original {
		add(name, "runtime-record", filepath.Join(q.Records, name), raw)
	}
	sort.Slice(plan.Members, func(i, j int) bool { return plan.Members[i].ID < plan.Members[j].ID })
	// Strong owned/private/single-link checks occur before any staging writes.
	if err = artifactset.VerifyInputs(ctx, plan); err != nil {
		return zero, err
	}
	parent, _, err := readbackDirectory(filepath.Dir(q.Destination))
	if err != nil {
		return zero, err
	}
	parent.Close()
	if _, err := os.Lstat(q.Destination); !errors.Is(err, os.ErrNotExist) {
		return zero, fmt.Errorf("new destination required")
	}
	stage, err := os.MkdirTemp(filepath.Dir(q.Destination), ".offline-capture-")
	if err != nil {
		return zero, err
	}
	defer func() {
		if cleanup := os.RemoveAll(stage); cleanup != nil {
			report = zero
			err = errors.Join(err, cleanup)
		}
	}()
	write := func(name string, raw []byte) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		f, err := os.OpenFile(filepath.Join(stage, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return err
		}
		_, e := f.Write(raw)
		return errors.Join(e, f.Sync(), f.Close())
	}
	for i, output := range r.Result.BuildOutputs.Files {
		// Archive leaf IDs are not output paths. Original case-sensitive names,
		// order, budgets and byte identities remain in the unmodified report.
		name := fmt.Sprintf("output-%03d.bin", i)
		if err = write(name, output.Bytes); err != nil {
			return zero, err
		}
		add(name, "build-output", filepath.Join(stage, name), output.Bytes)
	}
	sort.Slice(plan.Members, func(i, j int) bool { return plan.Members[i].ID < plan.Members[j].ID })
	planRaw, err := json.Marshal(plan)
	if err != nil {
		return zero, err
	}
	if err = write("plan.json", planRaw); err != nil {
		return zero, err
	}
	packed, err := artifactset.Pack(ctx, filepath.Join(stage, "plan.json"), canonical.BytesDigest(planRaw), q.Destination)
	if err != nil {
		return zero, err
	}
	_, _, after, err := readOfflineCapture(ctx, q)
	if err != nil {
		return zero, err
	}
	for name, raw := range original {
		if !bytes.Equal(raw, after[name]) {
			return zero, fmt.Errorf("offline producer record changed during capture")
		}
	}
	if err = artifactset.VerifyInputs(ctx, plan); err != nil {
		return zero, err
	}
	resultDigest, err := canonical.Digest(r.Result)
	if err != nil {
		return zero, err
	}
	b := r.Result.BuildOutputs
	return OfflineCaptureReport{Archive: packed, Selection: "FROZEN_OFFLINE_RECORDS_AND_COLLECTED_OUTPUTS", PermitDigest: q.PermitDigest, ReportDigest: q.ReportDigest, ResultDigest: resultDigest, OutputContractDigest: b.ContractDigest, OutputState: b.State, OutputCount: len(b.Files), CommandExitCode: r.Result.ExitCode, CoreObservation: "NOT_OBSERVED"}, nil
}
