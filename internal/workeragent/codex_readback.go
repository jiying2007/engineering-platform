package workeragent

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"time"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/codexexec"
	"github.com/jiying2007/engineering-platform/internal/controlclient"
	"github.com/jiying2007/engineering-platform/internal/runtime/codexapp"
	"github.com/jiying2007/engineering-platform/internal/sandbox"
	"github.com/jiying2007/engineering-platform/internal/sourcecheckpoint"
	"github.com/jiying2007/engineering-platform/internal/strictjson"
)

var readbackRunID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,255}$`)
var readbackExecutionID = regexp.MustCompile(`^[0-9a-f]{64}$`)
var postTurnPhases = [...]string{"TURN_RECEIPT", "FINALIZE", "RESULT_PERSIST", "REPORT_RENEW", "RESULT_REPORT", "RECEIPT_VERIFY"}

// PostTurnReadbackRequest names private records, not a workspace to execute or
// repair. Artifact paths are supplied independently; metadata paths are ignored.
// PermitDigest is the externally pinned raw digest, not proof of authority alone.
type PostTurnReadbackRequest struct {
	Records, RunID, ExecutionID, PermitDigest string
	Archive, Bundle                           string
}

func (q PostTurnReadbackRequest) Validate() error {
	if !readbackRunID.MatchString(q.RunID) || !readbackExecutionID.MatchString(q.ExecutionID) || !canonical.ValidDigest(q.PermitDigest) || !filepath.IsAbs(q.Records) || filepath.Clean(q.Records) != q.Records {
		return fmt.Errorf("exact Run, execution, permit digest and absolute private records directory required")
	}
	for _, path := range []string{q.Archive, q.Bundle} {
		if path != "" && (!filepath.IsAbs(path) || filepath.Clean(path) != path) {
			return fmt.Errorf("explicit absolute artifact path required")
		}
	}
	return nil
}

type ReadbackFile struct {
	Name   string `json:"name"`
	Digest string `json:"digest"`
	Size   int    `json:"size"`
}

// PostTurnReadback separates local observations, optional byte checks and an
// optional authenticated Core observation. No field grants an effect.
type PostTurnReadback struct {
	Version                   int             `json:"version"`
	Token                     codexexec.Token `json:"token"`
	PermitDigest              string          `json:"permit_digest"`
	LocalObservation          string          `json:"local_observation"`
	EnteredPhases             []string        `json:"entered_phases"`
	FailedPhase               string          `json:"failed_phase,omitempty"`
	TranscriptDigest          string          `json:"control_transcript_digest,omitempty"`
	ControlTranscriptRetained bool            `json:"control_transcript_retained"`
	ExpectedResultDigest      string          `json:"expected_result_digest,omitempty"`
	CheckpointDigest          string          `json:"checkpoint_digest,omitempty"`
	LocalRegistrationClaim    bool            `json:"local_registration_claim"`
	SourceBytes               string          `json:"source_bytes"`
	BundleBytes               string          `json:"bundle_bytes"`
	CoreObservation           string          `json:"core_observation"`
	CoreCheckpoint            string          `json:"core_checkpoint"`
	ObservedAt                time.Time       `json:"observed_at"`
	CoreObservedAt            *time.Time      `json:"core_observed_at,omitempty"`
	Files                     []ReadbackFile  `json:"files"`
	ExecutionAuthorized       bool            `json:"execution_authorized"`
	ReplayAuthorized          bool            `json:"replay_authorized"`
	ProductionQualified       bool            `json:"production_qualified"`
	permit                    codexexec.Permit
	checkpoint                *codexexec.SourceCheckpoint
	result                    *codexexec.Result
	controlTranscript         *codexexec.ControlTranscript
}

func recordName(id, suffix string) string {
	return "codex-" + sandbox.Hash([]byte(id + suffix))[7:] + ".json"
}

// The directory must remain host-owned and quiescent while observed. This is
// consistency readback, not protection from a malicious same-UID writer/root.
func readbackDirectory(path string) (*os.Root, os.FileInfo, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil || path != resolved {
		return nil, nil, fmt.Errorf("record directory alias or missing directory")
	}
	before, err := os.Lstat(path)
	if err != nil || !before.IsDir() || before.Mode().Perm()&0077 != 0 {
		return nil, nil, fmt.Errorf("private record directory required")
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, nil, fmt.Errorf("record directory unavailable")
	}
	f, err := root.Open(".")
	if err != nil {
		root.Close()
		return nil, nil, fmt.Errorf("record directory unavailable")
	}
	info, err := f.Stat()
	f.Close()
	if err != nil || !os.SameFile(before, info) {
		root.Close()
		return nil, nil, fmt.Errorf("record directory changed")
	}
	return root, before, nil
}
func readbackFile(root *os.Root, name string) ([]byte, error) {
	before, err := root.Lstat(name)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil || !before.Mode().IsRegular() || before.Mode().Perm()&0077 != 0 || before.Size() <= 0 || before.Size() > strictjson.MaxBytes {
		return nil, fmt.Errorf("invalid private record: %s", name)
	}
	f, err := openReadbackFile(root, name)
	if err != nil {
		return nil, fmt.Errorf("record unavailable: %s", name)
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(before, opened) || opened.Mode().Perm()&0077 != 0 {
		return nil, fmt.Errorf("record changed: %s", name)
	}
	raw, err := io.ReadAll(io.LimitReader(f, strictjson.MaxBytes+1))
	after, statErr := root.Lstat(name)
	if err != nil || statErr != nil || !os.SameFile(before, after) || before.Size() != int64(len(raw)) || !before.ModTime().Equal(after.ModTime()) || before.Mode() != after.Mode() {
		return nil, fmt.Errorf("record changed: %s", name)
	}
	return raw, nil
}
func checkReadbackCheckpoint(c codexexec.SourceCheckpoint, p codexexec.Permit, transcript string) error {
	if c.Validate() != nil || c.Binding.Token != p.Token || c.Binding.ExecutionEpoch != p.Assignment.Intent.ExecutionEpoch || c.TaskDigest != p.Assignment.Intent.TaskDigest || c.InputDigest != p.Assignment.Intent.InputDigest || c.BaseCommit != p.Preparation.Facts.BaseCommit || c.BaselineDigest != p.Preparation.Facts.SourceDigest || c.TranscriptDigest != transcript {
		return fmt.Errorf("checkpoint identity mismatch")
	}
	return nil
}
func validatePhase(r postTurnRecord, p codexexec.Permit, phase string, terminal bool) error {
	want := "ENTERED"
	if terminal {
		want = "FAILED_UNCONFIRMED"
	}
	if r.Version != 1 || r.Token != p.Token || r.Phase != phase || r.Observation != want || r.TaskDigest != p.Assignment.Intent.TaskDigest || r.InputDigest != p.Assignment.Intent.InputDigest || !canonical.ValidDigest(r.TranscriptDigest) || r.ExecutionAuthorized || r.DeliveryConfirmedByWorker {
		return fmt.Errorf("invalid phase identity or authority claim")
	}
	early := phase == postTurnPhases[0] || phase == postTurnPhases[1]
	if (early && r.ResultDigest != "") || (!early && !canonical.ValidDigest(r.ResultDigest)) {
		return fmt.Errorf("phase/result identity mismatch")
	}
	if !terminal && (r.Checkpoint != nil || r.RegistrationConfirmed) {
		return fmt.Errorf("phase entry claims completion")
	}
	if r.RegistrationConfirmed && r.Checkpoint == nil {
		return fmt.Errorf("registration without checkpoint")
	}
	return nil
}

// InspectPostTurn never writes, executes Git/model tools, scans source/HOME,
// or follows metadata paths. Absence/ENTERED/FAILED never become success.
func InspectPostTurn(ctx context.Context, q PostTurnReadbackRequest) (PostTurnReadback, error) {
	var empty PostTurnReadback
	if err := q.Validate(); err != nil {
		return empty, err
	}
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	root, dirInfo, err := readbackDirectory(q.Records)
	if err != nil {
		return empty, err
	}
	defer root.Close()
	observed := map[string][]byte{}
	get := func(name string, out any) (bool, error) {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		raw, err := readbackFile(root, name)
		if err != nil {
			return false, err
		}
		observed[name] = raw
		if raw == nil {
			return false, nil
		}
		if out != nil && strictjson.Decode(raw, out) != nil {
			return false, fmt.Errorf("invalid record JSON: %s", name)
		}
		return true, nil
	}
	permitName := "codex-" + q.ExecutionID + ".json"
	var p codexexec.Permit
	ok, err := get(permitName, &p)
	if err != nil {
		return empty, err
	}
	if !ok || canonical.BytesDigest(observed[permitName]) != q.PermitDigest || p.Token.ID != q.ExecutionID || p.Token.RunID != q.RunID || p.Check(p.Preparation.Admission.Worker, codexexec.Start{RunID: q.RunID, WorkerProfile: p.Token.WorkerProfile, Profile: p.Profile}) != nil {
		return empty, fmt.Errorf("permit anchor or identity mismatch")
	}
	r := PostTurnReadback{Version: 1, Token: p.Token, PermitDigest: q.PermitDigest, LocalObservation: "NO_POST_TURN_RECORDS", EnteredPhases: []string{}, SourceBytes: "NOT_READ", BundleBytes: "NOT_READ", CoreObservation: "NOT_OBSERVED", CoreCheckpoint: "NOT_OBSERVED", Files: []ReadbackFile{}, permit: p}
	controlName := recordName(q.ExecutionID, ":control-transcript")
	var localControl codexexec.ControlTranscript
	controlSaved, err := get(controlName, &localControl)
	if err != nil {
		return empty, err
	}
	if controlSaved {
		digest, digestErr := localControl.Digest()
		if digestErr != nil || localControl.Close.Binding.Token != p.Token ||
			localControl.Close.Binding.ExecutionEpoch != p.Assignment.Intent.ExecutionEpoch {
			return empty, fmt.Errorf("retained control transcript identity mismatch")
		}
		r.TranscriptDigest = digest
		r.ControlTranscriptRetained = true
		r.controlTranscript = &localControl
	}
	bind := func(entry postTurnRecord) error {
		if r.TranscriptDigest != "" && r.TranscriptDigest != entry.TranscriptDigest {
			return fmt.Errorf("mixed transcript records")
		}
		r.TranscriptDigest = entry.TranscriptDigest
		if entry.ResultDigest != "" {
			if r.ExpectedResultDigest != "" && r.ExpectedResultDigest != entry.ResultDigest {
				return fmt.Errorf("mixed result records")
			}
			r.ExpectedResultDigest = entry.ResultDigest
		}
		return nil
	}
	missing := false
	for _, phase := range postTurnPhases {
		var entry postTurnRecord
		ok, err := get(recordName(q.ExecutionID, ":post-turn:"+phase), &entry)
		if err != nil {
			return empty, err
		}
		if !ok {
			missing = true
			continue
		}
		if missing {
			return empty, fmt.Errorf("noncontiguous phase records")
		}
		if err := validatePhase(entry, p, phase, false); err != nil {
			return empty, err
		}
		if err := bind(entry); err != nil {
			return empty, err
		}
		r.EnteredPhases = append(r.EnteredPhases, phase)
		r.LocalObservation = "PHASE_ENTRIES_ONLY"
	}
	var failed postTurnRecord
	ok, err = get(recordName(q.ExecutionID, ":post-turn-failure"), &failed)
	if err != nil {
		return empty, err
	}
	if ok {
		index := -1
		for i, phase := range postTurnPhases {
			if phase == failed.Phase {
				index = i
			}
		}
		// A failed phase write may leave only its predecessor entry.
		if index < 0 || (index != len(r.EnteredPhases)-1 && index != len(r.EnteredPhases)) {
			return empty, fmt.Errorf("failure phase contradicts progress")
		}
		if err := validatePhase(failed, p, failed.Phase, true); err != nil {
			return empty, err
		}
		if err := bind(failed); err != nil {
			return empty, err
		}
		r.LocalObservation = "FAILED_UNCONFIRMED"
		r.FailedPhase = failed.Phase
		r.LocalRegistrationClaim = failed.RegistrationConfirmed
		r.checkpoint = failed.Checkpoint
	}
	var artifact sourcecheckpoint.Artifact
	ok, err = get(recordName(q.ExecutionID, ":source-checkpoint"), &artifact)
	if err != nil {
		return empty, err
	}
	if ok {
		if r.TranscriptDigest == "" {
			r.TranscriptDigest = artifact.Facts.TranscriptDigest
		}
		if err := checkReadbackCheckpoint(artifact.Facts, p, r.TranscriptDigest); err != nil {
			return empty, err
		}
		if r.checkpoint != nil && *r.checkpoint != artifact.Facts {
			return empty, fmt.Errorf("conflicting checkpoint records")
		}
		r.checkpoint = &artifact.Facts // archive_path is intentionally NEVER followed.
	}
	if r.checkpoint != nil {
		if err := checkReadbackCheckpoint(*r.checkpoint, p, r.TranscriptDigest); err != nil {
			return empty, err
		}
		r.CheckpointDigest, _ = r.checkpoint.Digest()
	}
	var turn codexapp.EngineeringReceipt
	turnName := recordName(q.ExecutionID, ":turn")
	turnSaved, err := get(turnName, &turn)
	if err != nil {
		return empty, err
	}
	if turnSaved {
		profile := p.Profile
		if len(r.EnteredPhases) == 0 || turn.Validate() != nil || turn.Provider != profile.Provider || turn.Version != profile.CodexVersion || turn.BinaryDigest != profile.BinaryDigest || turn.QualificationDigest != profile.QualificationDigest || turn.EngineeringConfigDigest != profile.EngineeringConfigDigest || turn.Model != profile.Model {
			return empty, fmt.Errorf("turn/permit identity mismatch")
		}
		if r.checkpoint != nil && (r.checkpoint.Binding.ThreadID != turn.ThreadID || r.checkpoint.Binding.TurnID != turn.TurnID) {
			return empty, fmt.Errorf("checkpoint/turn identity mismatch")
		}
		if r.controlTranscript != nil && (r.controlTranscript.Close.Binding.ThreadID != turn.ThreadID ||
			r.controlTranscript.Close.Binding.TurnID != turn.TurnID) {
			return empty, fmt.Errorf("retained control transcript/turn identity mismatch")
		}
	} else if len(r.EnteredPhases) >= 2 {
		return empty, fmt.Errorf("Finalize phase without saved turn")
	}
	// Decode the existing producer's exact lower-case wrapper. Its stored path
	// is never used to open a bundle; the operator must supply --bundle separately.
	resultName := recordName(q.ExecutionID, ":result")
	var saved struct {
		Result     codexexec.Result `json:"result"`
		BundlePath string           `json:"bundle_path"`
	}
	ok, err = get(resultName, &saved)
	if err != nil {
		return empty, err
	}
	if ok {
		result := saved.Result
		if !turnSaved || result.Codex != turn || result.Validate(p.Profile, p) != nil {
			return empty, fmt.Errorf("invalid local result")
		}
		digest, _ := canonical.Digest(result)
		if digest != r.ExpectedResultDigest || result.ControlTranscriptDigest != r.TranscriptDigest || len(r.EnteredPhases) < 3 {
			return empty, fmt.Errorf("result/phase identity mismatch")
		}
		r.result = &result
		if r.checkpoint != nil && (r.checkpoint.Binding.ThreadID != result.Codex.ThreadID || r.checkpoint.Binding.TurnID != result.Codex.TurnID) {
			return empty, fmt.Errorf("checkpoint/result turn mismatch")
		}
	}
	if len(r.EnteredPhases) >= 4 && r.result == nil {
		return empty, fmt.Errorf("reported phase without saved result")
	}
	if q.Archive != "" {
		if r.checkpoint == nil {
			return empty, fmt.Errorf("archive lacks bound checkpoint")
		}
		facts, err := sourcecheckpoint.Verify(ctx, q.Archive, r.checkpoint.ArchiveDigest, q.RunID)
		if err != nil || facts != *r.checkpoint {
			return empty, fmt.Errorf("source archive readback failed")
		}
		r.SourceBytes = "BYTES_VERIFIED"
	}
	if q.Bundle != "" {
		if r.result == nil {
			return empty, fmt.Errorf("bundle lacks bound result")
		}
		if err := verifyReadbackBundle(ctx, q.Bundle, r.result.Change.BundleDigest, r.result.Change.BundleSize); err != nil {
			return empty, err
		}
		r.BundleBytes = "BYTES_VERIFIED_NOT_GIT_REPLAYED"
	}
	for name, raw := range observed {
		current, err := readbackFile(root, name)
		if err != nil || !bytes.Equal(raw, current) {
			return empty, fmt.Errorf("record set changed during readback")
		}
	}
	current, err := os.Lstat(q.Records)
	resolved, resolveErr := filepath.EvalSymlinks(q.Records)
	if err != nil || resolveErr != nil || resolved != q.Records || !os.SameFile(dirInfo, current) || current.Mode().Perm()&0077 != 0 {
		return empty, fmt.Errorf("record directory changed during readback")
	}
	// Deterministic output; retain only names/hashes, never source/model/steer text.
	names := []string{permitName, controlName}
	for _, phase := range postTurnPhases {
		names = append(names, recordName(q.ExecutionID, ":post-turn:"+phase))
	}
	names = append(names, recordName(q.ExecutionID, ":post-turn-failure"), recordName(q.ExecutionID, ":source-checkpoint"), turnName, resultName)
	for _, name := range names {
		if raw := observed[name]; raw != nil {
			r.Files = append(r.Files, ReadbackFile{Name: name, Digest: canonical.BytesDigest(raw), Size: len(raw)})
		}
	}
	r.ObservedAt = time.Now().UTC()
	return r, nil
}
func verifyReadbackBundle(ctx context.Context, path, digest string, size int64) error {
	if !canonical.ValidDigest(digest) || size <= 0 || size > 1<<30 {
		return fmt.Errorf("invalid expected bundle identity")
	}
	root, _, err := readbackDirectory(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer root.Close()
	name := filepath.Base(path)
	before, err := root.Lstat(name)
	if err != nil || !before.Mode().IsRegular() || before.Mode().Perm()&0022 != 0 || before.Size() != size {
		return fmt.Errorf("private exact bundle required")
	}
	f, err := openReadbackFile(root, name)
	if err != nil {
		return fmt.Errorf("bundle unavailable")
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !os.SameFile(before, opened) {
		return fmt.Errorf("bundle changed")
	}
	h := sha256.New()
	n, err := io.Copy(h, &readbackContextReader{ctx: ctx, r: io.LimitReader(f, size+1)})
	after, statErr := root.Lstat(name)
	if err != nil || statErr != nil || n != size || !os.SameFile(before, after) || !before.ModTime().Equal(after.ModTime()) || before.Size() != after.Size() || before.Mode() != after.Mode() || "sha256:"+hex.EncodeToString(h.Sum(nil)) != digest {
		return fmt.Errorf("bundle readback failed")
	}
	return nil
}

type readbackContextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r *readbackContextReader) Read(b []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(b)
}

// ObservePostTurnCore performs exactly one authenticated GET. It does not POST,
// reinterpret a local failure as Core state, or print the provider output.
func ObservePostTurnCore(ctx context.Context, c *controlclient.Client, r PostTurnReadback) (PostTurnReadback, error) {
	if c == nil || r.Version != 1 || !readbackRunID.MatchString(r.Token.RunID) || r.permit.Token != r.Token || r.ExecutionAuthorized || r.ReplayAuthorized || r.ProductionQualified {
		return PostTurnReadback{}, fmt.Errorf("verified local observation required")
	}
	raw, err := c.Raw(ctx, http.MethodGet, "/api/v1/runs/"+r.Token.RunID+"/codex", nil)
	if err != nil {
		return PostTurnReadback{}, err
	}
	var status codexexec.Status
	if strictjson.Decode(raw, &status) != nil || status.Token != r.Token {
		return PostTurnReadback{}, fmt.Errorf("Core execution identity mismatch")
	}
	switch status.State {
	case codexexec.Authorized, codexexec.Unknown, codexexec.Stopped:
		if status.Receipt != nil {
			return PostTurnReadback{}, fmt.Errorf("non-finished Core state carries receipt")
		}
	case codexexec.Finished:
		if status.Receipt == nil || status.VerifyFinishedControls() != nil || status.Receipt.Verify(r.permit.Preparation.Admission.Worker, r.permit, status.Receipt.Result) != nil || r.ExpectedResultDigest == "" || status.Receipt.ResultDigest != r.ExpectedResultDigest {
			return PostTurnReadback{}, fmt.Errorf("Core result readback mismatch")
		}
	default:
		return PostTurnReadback{}, fmt.Errorf("unknown Core execution state")
	}
	if r.TranscriptDigest != "" {
		t := status.Runtime
		if t == nil || t.State != "SEALED" || t.Transcript == nil {
			return PostTurnReadback{}, fmt.Errorf("Core sealed transcript missing")
		}
		digest, err := t.Transcript.Digest()
		if err != nil || digest != r.TranscriptDigest || t.Digest != digest || t.Binding != t.Transcript.Close.Binding || t.Binding.Token != r.Token || t.Binding.ExecutionEpoch != r.permit.Assignment.Intent.ExecutionEpoch || !t.Transcript.Close.ProcessScope.Quiescent() {
			return PostTurnReadback{}, fmt.Errorf("Core transcript readback mismatch")
		}
		if r.ControlTranscriptRetained && (r.controlTranscript == nil || !reflect.DeepEqual(*r.controlTranscript, *t.Transcript)) {
			return PostTurnReadback{}, fmt.Errorf("Core transcript differs from retained private transcript")
		}
	}
	r.CoreCheckpoint = "NOT_PRESENT"
	if status.SourceCheckpoint != nil {
		if err := checkReadbackCheckpoint(*status.SourceCheckpoint, r.permit, r.TranscriptDigest); err != nil {
			return PostTurnReadback{}, err
		}
		if status.Runtime == nil || status.SourceCheckpoint.Binding != status.Runtime.Binding {
			return PostTurnReadback{}, fmt.Errorf("Core checkpoint runtime mismatch")
		}
		if r.checkpoint != nil && *status.SourceCheckpoint != *r.checkpoint {
			return PostTurnReadback{}, fmt.Errorf("Core checkpoint readback mismatch")
		}
		r.CoreCheckpoint = "DESCRIPTOR_OBSERVED_NOT_BYTES"
		if r.checkpoint != nil {
			r.CoreCheckpoint = "MATCHES_LOCAL_DESCRIPTOR"
		}
	}
	r.CoreObservation = status.State
	now := time.Now().UTC()
	r.CoreObservedAt = &now
	return r, nil
}
