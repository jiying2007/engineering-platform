package workeragent

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"syscall"

	"github.com/jiying2007/engineering-platform/internal/artifactset"
	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/runtime/codexapp"
	"github.com/jiying2007/engineering-platform/internal/strictjson"
)

// ExecutionCaptureRequest explicitly selects private inputs. ContextDirectory is
// the original frozen bundle, not a resolver or a path read from a local record.
// There is no Core/network access, model execution, directory discovery or repair.
type ExecutionCaptureRequest struct {
	Readback             PostTurnReadbackRequest
	ContextDirectory     string
	RuntimeBinary        string
	QualificationReceipt string
	ContinuationArchive  string
	Destination          string
}

type ExecutionCaptureReport struct {
	Archive                   artifactset.Report `json:"archive"`
	Selection                 string             `json:"selection"`
	PermitDigest              string             `json:"permit_digest"`
	ContextManifestDigest     string             `json:"context_manifest_digest"`
	RecordCount               int                `json:"record_count"`
	ContextCount              int                `json:"context_count"`
	SourceCheckpointRetained  bool               `json:"source_checkpoint_retained"`
	ResultBundleRetained      bool               `json:"result_bundle_retained"`
	RuntimeBinaryRetained     bool               `json:"runtime_binary_retained"`
	QualificationRetained     bool               `json:"qualification_retained"`
	ControlTranscriptRetained bool               `json:"control_transcript_retained"`
	ItemHistoryRetained       bool               `json:"item_history_retained"`
	ContinuationRetained      bool               `json:"continuation_retained"`
	RuntimeBinaryDigest       string             `json:"runtime_binary_digest"`
	QualificationDigest       string             `json:"qualification_digest"`
	FullRunBackup             bool               `json:"full_run_backup"`
	ExecutionAuthorized       bool               `json:"execution_authorized"`
	ProductionQualified       bool               `json:"production_qualified"`
}

// CaptureExecutionArtifacts derives the existing raw-set plan from verified
// producer records and the frozen Run input, so callers cannot omit a referenced
// checkpoint, result bundle or context member while claiming this selection.
// It retains current record formats unchanged and additionally requires private
// copies of the exact Codex binary and qualification receipt already frozen by
// the Permit. If this Run is an explicit source continuation, the exact upstream
// source-checkpoint archive is also mandatory and is validated with the SAME
// ref/Task/base/profile checks used by restoration before it enters the plan.
// Current result/checkpoint producers retain their own base Git graph. The exact
// sealed private ControlTranscript and exact bounded completed-item history are
// mandatory for current capture. This is still NOT all Run dependencies:
 // credential/session material and provider-internal streaming state remain outside
// this narrowly declared selection.
func CaptureExecutionArtifacts(ctx context.Context, q ExecutionCaptureRequest) (report ExecutionCaptureReport, err error) {
	var zero ExecutionCaptureReport
	for _, p := range []string{q.ContextDirectory, q.RuntimeBinary, q.QualificationReceipt, q.Destination} {
		if !filepath.IsAbs(p) || filepath.Clean(p) != p {
			return zero, fmt.Errorf("canonical explicit context/runtime/qualification/destination required")
		}
	}
	if q.ContinuationArchive != "" && (!filepath.IsAbs(q.ContinuationArchive) || filepath.Clean(q.ContinuationArchive) != q.ContinuationArchive) {
		return zero, fmt.Errorf("canonical explicit continuation archive required")
	}
	// Do not add staging/output into producer records or the frozen context.
	for _, root := range []string{q.Readback.Records, q.ContextDirectory} {
		rel, e := filepath.Rel(root, q.Destination)
		if e == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return zero, fmt.Errorf("destination overlaps protected input directory")
		}
	}
	before, err := InspectPostTurn(ctx, q.Readback)
	if err != nil {
		return zero, err
	}
	if !before.ControlTranscriptRetained {
		return zero, fmt.Errorf("sealed private control transcript required for complete execution capture")
	}
	if !before.ItemHistoryRetained || !canonical.ValidDigest(before.ItemHistoryDigest) {
		return zero, fmt.Errorf("exact private model/tool item history required for complete execution capture")
	}
	if before.result == nil && before.checkpoint == nil {
		return zero, fmt.Errorf("no bound stopped source or result to retain")
	}
	if (before.checkpoint != nil) != (q.Readback.Archive != "") || (before.result != nil) != (q.Readback.Bundle != "") {
		return zero, fmt.Errorf("every referenced source archive and result bundle must be supplied explicitly")
	}
	p := before.permit
	continuationRetained := false
	if p.Assignment.Input.Continuation != nil {
		if q.ContinuationArchive == "" {
			return zero, fmt.Errorf("successor Run requires its exact upstream continuation archive")
		}
		if _, err := VerifyContinuationArchive(ctx, p.Assignment, q.ContinuationArchive); err != nil {
			return zero, fmt.Errorf("upstream continuation archive: %w", err)
		}
		continuationRetained = true
	} else if q.ContinuationArchive != "" {
		return zero, fmt.Errorf("ordinary Run cannot claim an upstream continuation archive")
	}
	runtimeDigest, runtimeSize, _, err := capturePrivateDependency(ctx, q.RuntimeBinary, artifactset.MaxFile, false)
	if err != nil || runtimeDigest != p.Profile.BinaryDigest {
		return zero, fmt.Errorf("runtime binary does not match frozen profile")
	}
	qualificationRawDigest, qualificationSize, qualificationRaw, err := capturePrivateDependency(ctx, q.QualificationReceipt, artifactset.MaxMetadata, true)
	if err != nil {
		return zero, fmt.Errorf("qualification receipt unavailable: %w", err)
	}
	var qualification codexapp.QualificationReceipt
	if strictjson.Decode(qualificationRaw, &qualification) != nil {
		return zero, fmt.Errorf("invalid qualification receipt")
	}
	qualificationDigest, err := qualification.Digest()
	if err != nil || qualificationDigest != p.Profile.QualificationDigest ||
		qualification.BinaryDigest != p.Profile.BinaryDigest ||
		qualification.Version != p.Profile.CodexVersion ||
		qualification.ThreadStartModel != p.Profile.Model ||
		qualification.EngineeringConfigDigest != p.Profile.EngineeringConfigDigest {
		return zero, fmt.Errorf("qualification receipt does not match frozen profile")
	}
	plan := artifactset.Plan{Version: 1, Subject: artifactset.Subject{RunID: q.Readback.RunID, ExecutionID: q.Readback.ExecutionID, TaskDigest: p.Assignment.Intent.TaskDigest, InputDigest: p.Assignment.Intent.InputDigest, BaseCommit: p.Preparation.Facts.BaseCommit}}
	add := func(id, kind, path, digest string, size int64) {
		plan.Members = append(plan.Members, artifactset.Input{Entry: artifactset.Entry{ID: id, Kind: kind, Size: size, Digest: digest}, Path: path})
	}
	for _, f := range before.Files {
		add(f.Name, "runtime-record", filepath.Join(q.Readback.Records, f.Name), f.Digest, int64(f.Size))
	}
	add("runtime-codex.bin", "runtime-binary", q.RuntimeBinary, runtimeDigest, runtimeSize)
	add("runtime-qualification.json", "qualification", q.QualificationReceipt, qualificationRawDigest, qualificationSize)
	contextRaw, err := json.Marshal(p.Preparation.Facts.Context)
	if err != nil {
		return zero, err
	}
	add("context-manifest.json", "context", filepath.Join(q.ContextDirectory, "manifest.json"), p.Preparation.Facts.BundleDigest, int64(len(contextRaw)))
	names := []string{"manifest.json"}
	for _, entry := range p.Preparation.Facts.Context.Entries {
		// Permit.Check has already verified each exact derived leaf name and digest.
		names = append(names, entry.File)
		add(entry.File, "context", filepath.Join(q.ContextDirectory, entry.File), entry.Ref.Digest, entry.Size)
	}
	if p.Assignment.Input.Continuation != nil {
		ref := p.Assignment.Input.Continuation
		add("continuation-source-checkpoint.tar", "source", q.ContinuationArchive, ref.ArchiveDigest, ref.ArchiveSize)
	}
	if before.checkpoint != nil {
		add("source-checkpoint.tar", "source", q.Readback.Archive, before.checkpoint.ArchiveDigest, before.checkpoint.ArchiveSize)
	}
	if before.result != nil {
		add("result.bundle", "git-bundle", q.Readback.Bundle, before.result.Change.BundleDigest, before.result.Change.BundleSize)
	}
	sort.Slice(plan.Members, func(i, j int) bool { return plan.Members[i].ID < plan.Members[j].ID })
	if err := artifactset.VerifyInputs(ctx, plan); err != nil {
		return zero, err
	}
	// Reject extra context files instead of copying arbitrary unreferenced bytes.
	if err := contextInventory(q.ContextDirectory, names); err != nil {
		return zero, err
	}
	for _, in := range plan.Members {
		if q.Destination == in.Path {
			return zero, fmt.Errorf("destination overlaps input")
		}
	}
	parent, _, err := readbackDirectory(filepath.Dir(q.Destination))
	if err != nil {
		return zero, err
	}
	parent.Close()
	stage, err := os.MkdirTemp(filepath.Dir(q.Destination), ".execution-capture-")
	if err != nil {
		return zero, err
	}
	// Only our new private staging directory is cleaned. Never remove a published
	// archive on failure: an error after publication requires exact observation.
	defer func() {
		if cleanup := os.RemoveAll(stage); cleanup != nil {
			report = zero
			err = errors.Join(err, cleanup)
		}
	}()
	planRaw, err := json.Marshal(plan)
	if err != nil {
		return zero, err
	}
	planPath := filepath.Join(stage, "plan.json")
	f, err := os.OpenFile(planPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return zero, err
	}
	_, writeErr := f.Write(planRaw)
	if err = errors.Join(writeErr, f.Sync(), f.Close()); err != nil {
		return zero, err
	}
	packed, err := artifactset.Pack(ctx, planPath, canonical.BytesDigest(planRaw), q.Destination)
	if err != nil {
		return zero, err
	}
	// Re-evaluate selected records including absences, not just files already in
	// the archive. Pack itself rehashes every selected byte and private identity.
	after, err := InspectPostTurn(ctx, q.Readback)
	if err != nil || !reflect.DeepEqual(before.Files, after.Files) {
		return zero, fmt.Errorf("execution records changed during capture")
	}
	if err := contextInventory(q.ContextDirectory, names); err != nil {
		return zero, err
	}
	selection := "EXECUTION_RECORDS_CONTROL_HISTORY_CONTEXT_AND_FROZEN_RUNTIME"
	if continuationRetained {
		selection = "EXECUTION_RECORDS_CONTROL_HISTORY_CONTEXT_RUNTIME_AND_UPSTREAM_CONTINUATION"
	}
	return ExecutionCaptureReport{Archive: packed, Selection: selection, PermitDigest: q.Readback.PermitDigest, ContextManifestDigest: p.Preparation.Facts.BundleDigest, RecordCount: len(before.Files), ContextCount: len(p.Assignment.Input.ContextRefs), SourceCheckpointRetained: before.checkpoint != nil, ResultBundleRetained: before.result != nil, RuntimeBinaryRetained: true, QualificationRetained: true, ControlTranscriptRetained: true, ItemHistoryRetained: true, ContinuationRetained: continuationRetained, RuntimeBinaryDigest: p.Profile.BinaryDigest, QualificationDigest: p.Profile.QualificationDigest}, nil
}

func contextInventory(path string, expected []string) error {
	root, _, err := readbackDirectory(path)
	if err != nil {
		return err
	}
	defer root.Close()
	f, err := root.Open(".")
	if err != nil {
		return err
	}
	defer f.Close()
	entries, err := f.ReadDir(len(expected) + 1)
	if err != nil && err != io.EOF {
		return err
	}
	if len(entries) != len(expected) {
		return fmt.Errorf("unexpected context members")
	}
	want := map[string]bool{}
	for _, n := range expected {
		want[n] = true
	}
	for _, e := range entries {
		if !want[e.Name()] || !e.Type().IsRegular() {
			return fmt.Errorf("unexpected context member")
		}
	}
	return nil
}

func capturePrivateDependency(ctx context.Context, path string, limit int64, retain bool) (string, int64, []byte, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || limit <= 0 {
		return "", 0, nil, fmt.Errorf("canonical bounded dependency path required")
	}
	root, _, err := readbackDirectory(filepath.Dir(path))
	if err != nil {
		return "", 0, nil, err
	}
	defer root.Close()
	name := filepath.Base(path)
	before, err := root.Lstat(name)
	if err != nil || !before.Mode().IsRegular() || before.Mode().Perm()&0022 != 0 ||
		before.Size() <= 0 || before.Size() > limit {
		return "", 0, nil, fmt.Errorf("private bounded dependency required")
	}
	stat, ok := before.Sys().(*syscall.Stat_t)
	if !ok || stat.Nlink != 1 || stat.Uid != uint32(os.Geteuid()) {
		return "", 0, nil, fmt.Errorf("dependency ownership/link identity invalid")
	}
	file, err := openReadbackFile(root, name)
	if err != nil {
		return "", 0, nil, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(before, opened) {
		return "", 0, nil, fmt.Errorf("dependency changed before read")
	}
	hash := sha256.New()
	var raw bytes.Buffer
	var writer io.Writer = hash
	if retain {
		writer = io.MultiWriter(hash, &raw)
	}
	n, err := io.Copy(writer, &readbackContextReader{ctx: ctx, r: io.LimitReader(file, limit+1)})
	after, statErr := root.Lstat(name)
	var afterStat *syscall.Stat_t
	afterOK := false
	if after != nil {
		afterStat, afterOK = after.Sys().(*syscall.Stat_t)
	}
	if err != nil || statErr != nil || !afterOK || n != before.Size() || !os.SameFile(before, after) ||
		before.Size() != after.Size() || before.Mode() != after.Mode() ||
		!before.ModTime().Equal(after.ModTime()) || stat.Nlink != afterStat.Nlink ||
		stat.Uid != afterStat.Uid || stat.Ctim != afterStat.Ctim {
		return "", 0, nil, fmt.Errorf("dependency changed during read")
	}
	digest := "sha256:" + hex.EncodeToString(hash.Sum(nil))
	if retain {
		return digest, n, raw.Bytes(), nil
	}
	return digest, n, nil, nil
}
