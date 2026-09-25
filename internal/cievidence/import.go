package cievidence

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jiying2007/engineering-platform/internal/access"
	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/core"
	"github.com/jiying2007/engineering-platform/internal/runtime/codexapp"
)

const (
	TrustedRepository = "jiying2007/engineering-platform"
	TrustedWorkflow   = "CI"
	TrustedIssuer     = access.TrustedCIIssuer
	TrustedProcedure  = access.TrustedCIProcedure
	ImporterSubject   = access.TrustedCIImporterSubject
)

type RunFact struct {
	ID         int64
	Attempt    int64
	Repository string
	Workflow   string
	WorkflowPath string
	Event      string
	HeadBranch string
	HeadSHA    string
	Status     string
	Conclusion string
}

type ArtifactFact struct {
	ID      int64
	Name    string
	Digest  string
	Size    int64
	Expired bool
	RunID   int64
	HeadSHA string
}

type LiveFacts struct {
	Run       RunFact
	Jobs      []Job
	Artifacts []ArtifactFact
}

type ImportFiles struct {
	EnvelopeZip string
	BinariesZip string
	CodexZip    string
}

type ImportRequest struct {
	EvidenceID         string
	RequirementID      string
	EvidenceArtifactID string
	Delivery           core.DeliveryReceipt
	Files              ImportFiles
	Live               LiveFacts
}

func ReadEnvelopeZip(path string) (Envelope, error) {
	var envelope Envelope
	entry, err := readExactZipFile(path, "ci-evidence-envelope.json", 1<<20, map[string]bool{"ci-evidence-envelope.json": true})
	if err != nil {
		return envelope, err
	}
	decoder := json.NewDecoder(bytes.NewReader(entry))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil {
		return envelope, fmt.Errorf("decode CI evidence envelope: %w", err)
	}
	if decoder.Decode(new(any)) != io.EOF {
		return envelope, fmt.Errorf("trailing CI evidence JSON")
	}
	if err := envelope.Verify(); err != nil {
		return envelope, err
	}
	return envelope, nil
}

func VerifyTrustedImport(req ImportRequest) (core.EvidenceRef, error) {
	var empty core.EvidenceRef
	if !validLogicalID(req.EvidenceID) || !validLogicalID(req.RequirementID) || !validLogicalID(req.EvidenceArtifactID) {
		return empty, fmt.Errorf("bounded evidence, requirement and artifact IDs required")
	}
	if req.Delivery.ID == "" || req.Delivery.ResultCommit == "" || req.Delivery.SubjectDigest == "" {
		return empty, fmt.Errorf("completed delivery identity and result commit required")
	}
	calculated, err := req.Delivery.CalculateSubjectDigest()
	if err != nil || calculated != req.Delivery.SubjectDigest {
		return empty, fmt.Errorf("delivery subject digest mismatch")
	}
	trustedArtifact, ok := findArtifact(req.Live.Artifacts, "trusted-ci-evidence-"+req.Delivery.ResultCommit)
	if !ok {
		return empty, fmt.Errorf("trusted CI evidence artifact missing")
	}
	if err := verifyArchiveFile(req.Files.EnvelopeZip, trustedArtifact); err != nil {
		return empty, fmt.Errorf("trusted envelope archive: %w", err)
	}
	envelope, err := ReadEnvelopeZip(req.Files.EnvelopeZip)
	if err != nil {
		return empty, err
	}
	if err := verifyLiveFacts(envelope, req.Live, req.Delivery.ResultCommit); err != nil {
		return empty, err
	}
	binaryArtifact, ok := findArtifact(req.Live.Artifacts, "engineering-binaries-"+req.Delivery.ResultCommit)
	if !ok {
		return empty, fmt.Errorf("engineering binary artifact missing")
	}
	if err := verifyArchiveFile(req.Files.BinariesZip, binaryArtifact); err != nil {
		return empty, fmt.Errorf("binary archive: %w", err)
	}
	if err := verifyBinaryArchive(req.Files.BinariesZip, envelope.Receipt.Files); err != nil {
		return empty, err
	}
	if err := verifyArchiveFile(req.Files.BinariesZip, binaryArtifact); err != nil {
		return empty, fmt.Errorf("binary archive changed during verification: %w", err)
	}
	codexArtifact, ok := findArtifact(req.Live.Artifacts, "codex-0.155.0-qualification-"+req.Delivery.ResultCommit)
	if !ok {
		return empty, fmt.Errorf("Codex qualification artifact missing")
	}
	if err := verifyArchiveFile(req.Files.CodexZip, codexArtifact); err != nil {
		return empty, fmt.Errorf("Codex archive: %w", err)
	}
	if err := verifyCodexArchive(req.Files.CodexZip); err != nil {
		return empty, err
	}
	if err := verifyArchiveFile(req.Files.CodexZip, codexArtifact); err != nil {
		return empty, fmt.Errorf("Codex archive changed during verification: %w", err)
	}
	deliveryArtifact, ok := exactDeliveryArtifact(req.Delivery, req.EvidenceArtifactID)
	if !ok || deliveryArtifact.Digest != trustedArtifact.Digest {
		return empty, fmt.Errorf("delivery must bind the exact trusted CI envelope artifact digest")
	}
	if deliveryArtifact.MediaType != "" && deliveryArtifact.MediaType != "application/zip" {
		return empty, fmt.Errorf("trusted CI delivery artifact must be application/zip")
	}
	return core.EvidenceRef{
		ID:                req.EvidenceID,
		DeliveryReceiptID: req.Delivery.ID,
		RequirementID:     req.RequirementID,
		SubjectDigest:     req.Delivery.SubjectDigest,
		Issuer:            TrustedIssuer,
		Procedure:         TrustedProcedure,
		Result:            "PASS",
		ArtifactRefs:      []string{deliveryArtifact.ID},
		Applicable:        true,
	}, nil
}

func verifyLiveFacts(envelope Envelope, live LiveFacts, resultCommit string) error {
	if err := envelope.Verify(); err != nil {
		return err
	}
	r := envelope.Receipt
	if r.Repository != TrustedRepository || r.Workflow != TrustedWorkflow || r.Event != "push" ||
		r.SourceSHA != resultCommit || r.TestedSHA != resultCommit || r.BaseSHA != "" {
		return fmt.Errorf("CI receipt is not exact trusted-main evidence for the delivery commit")
	}
	if live.Run.ID != r.RunID || live.Run.Attempt != r.RunAttempt || live.Run.Repository != TrustedRepository ||
		live.Run.Workflow != TrustedWorkflow || live.Run.WorkflowPath != ".github/workflows/ci.yml" ||
		live.Run.Event != "push" || live.Run.HeadBranch != "main" || live.Run.HeadSHA != resultCommit ||
		live.Run.Status != "completed" || live.Run.Conclusion != "success" {
		return fmt.Errorf("live GitHub run does not match retained receipt")
	}
	jobs := append([]Job(nil), live.Jobs...)
	sort.Slice(jobs, func(i, j int) bool { return jobs[i].Name < jobs[j].Name })
	if len(jobs) < len(r.Jobs) {
		return fmt.Errorf("live GitHub jobs missing")
	}
	for _, expected := range r.Jobs {
		matches := 0
		for _, actual := range jobs {
			if actual.Name != expected.Name {
				continue
			}
			matches++
			if actual.ID != expected.ID || actual.Conclusion != "success" {
				return fmt.Errorf("required live GitHub job %q mismatch", expected.Name)
			}
		}
		if matches != 1 {
			return fmt.Errorf("required live GitHub job %q is missing or ambiguous", expected.Name)
		}
	}
	for _, expected := range r.Artifacts {
		actual, ok := findArtifact(live.Artifacts, expected.Name)
		if !ok || actual.ID != expected.ID || actual.Digest != expected.Digest || actual.Size != expected.Size ||
			actual.Expired || actual.RunID != r.RunID || actual.HeadSHA != resultCommit {
			return fmt.Errorf("required live GitHub artifact %q mismatch", expected.Name)
		}
	}
	return nil
}

func verifyArchiveFile(path string, fact ArtifactFact) error {
	before, err := os.Lstat(path)
	if err != nil || !before.Mode().IsRegular() || before.Mode()&os.ModeSymlink != 0 || before.Size() != fact.Size {
		return fmt.Errorf("artifact file size/type mismatch")
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !os.SameFile(before, opened) {
		return fmt.Errorf("artifact file changed before verification")
	}
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	digest := "sha256:" + hex.EncodeToString(h.Sum(nil))
	after, err := os.Lstat(path)
	if err != nil || !os.SameFile(before, after) || after.Size() != before.Size() {
		return fmt.Errorf("artifact file changed during verification")
	}
	if digest != fact.Digest {
		return fmt.Errorf("artifact archive digest mismatch")
	}
	return nil
}

func verifyBinaryArchive(path string, expected []File) error {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return fmt.Errorf("open binary artifact: %w", err)
	}
	defer reader.Close()
	want := map[string]File{}
	for _, file := range expected {
		want[file.Path] = file
	}
	allowed := map[string]bool{"SHA256SUMS": true, "file-manifest.json": true}
	for name := range want {
		allowed[name] = true
	}
	if len(want) != 5 {
		return fmt.Errorf("expected five retained executable facts")
	}
	seen := map[string]bool{}
	var manifest []byte
	for _, entry := range reader.File {
		if entry.FileInfo().IsDir() {
			continue
		}
		if !safeZipName(entry.Name) || !allowed[entry.Name] || seen[entry.Name] || !entry.FileInfo().Mode().IsRegular() {
			return fmt.Errorf("unexpected or unsafe binary artifact member %q", entry.Name)
		}
		seen[entry.Name] = true
		if entry.Name == "file-manifest.json" {
			manifest, err = readZipEntry(entry, 1<<20)
			if err != nil {
				return err
			}
			continue
		}
		if expectedFile, ok := want[entry.Name]; ok {
			if int64(entry.UncompressedSize64) != expectedFile.Size {
				return fmt.Errorf("binary %s size mismatch", entry.Name)
			}
			digest, size, err := zipEntryDigest(entry, 64<<20)
			if err != nil || size != expectedFile.Size || digest != expectedFile.Digest {
				return fmt.Errorf("binary %s digest mismatch", entry.Name)
			}
		}
	}
	for name := range allowed {
		if !seen[name] {
			return fmt.Errorf("binary artifact member %q missing", name)
		}
	}
	var got []File
	decoder := json.NewDecoder(bytes.NewReader(manifest))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&got); err != nil || decoder.Decode(new(any)) != io.EOF {
		return fmt.Errorf("invalid binary file manifest")
	}
	sort.Slice(got, func(i, j int) bool { return got[i].Path < got[j].Path })
	expectedCopy := append([]File(nil), expected...)
	sort.Slice(expectedCopy, func(i, j int) bool { return expectedCopy[i].Path < expectedCopy[j].Path })
	if !equalFiles(got, expectedCopy) {
		return fmt.Errorf("binary file manifest does not match CI envelope")
	}
	return nil
}

func verifyCodexArchive(path string) error {
	data, err := readExactZipFile(path, "codex-qualification.json", 1<<20, map[string]bool{
		"codex-qualification.json": true,
		"codex-npm-integrity.txt":  true,
	})
	if err != nil {
		return fmt.Errorf("Codex qualification artifact: %w", err)
	}
	var q codexapp.QualificationReceipt
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&q); err != nil || decoder.Decode(new(any)) != io.EOF {
		return fmt.Errorf("invalid Codex qualification receipt")
	}
	if q.SchemaVersion != 1 || q.CLI != "codex-cli" || q.Version != codexapp.QualifiedCodexVersion ||
		q.ReleaseTag != codexapp.QualifiedCodexReleaseTag || q.ReleaseCommit != codexapp.QualifiedCodexReleaseCommit ||
		!canonical.ValidDigest(q.BinaryDigest) || !canonical.ValidDigest(q.StableSchemaDigest) ||
		!canonical.ValidDigest(q.ExperimentalSchemaDigest) || q.Transport != "stdio" || !q.FreshProcess ||
		q.ManagedDaemon || q.PerThreadConfigOverride || !q.InitializePassed || !q.ThreadStartPassed ||
		q.ThreadStartModel != "gpt-5.6-sol" || !q.StableSchemaContractChecked || !q.ExperimentalSurfaceChecked ||
		!canonical.ValidDigest(q.CredentialSafeConfigDigest) || !q.CredentialSafeProfileChecked {
		return fmt.Errorf("Codex qualification receipt does not satisfy pinned contract")
	}
	return nil
}

func readExactZipFile(path, target string, limit int64, allowed map[string]bool) ([]byte, error) {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	seen := map[string]bool{}
	var result []byte
	for _, entry := range reader.File {
		if entry.FileInfo().IsDir() {
			continue
		}
		if !safeZipName(entry.Name) || !allowed[entry.Name] || seen[entry.Name] || !entry.FileInfo().Mode().IsRegular() {
			return nil, fmt.Errorf("unexpected or unsafe zip member %q", entry.Name)
		}
		seen[entry.Name] = true
		if entry.Name == target {
			result, err = readZipEntry(entry, limit)
			if err != nil {
				return nil, err
			}
		}
	}
	for name := range allowed {
		if !seen[name] {
			return nil, fmt.Errorf("required zip member %q missing", name)
		}
	}
	if result == nil {
		return nil, fmt.Errorf("zip target missing")
	}
	return result, nil
}

func readZipEntry(entry *zip.File, limit int64) ([]byte, error) {
	if int64(entry.UncompressedSize64) > limit {
		return nil, fmt.Errorf("zip member exceeds limit")
	}
	rc, err := entry.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	data, err := io.ReadAll(io.LimitReader(rc, limit+1))
	if err != nil || int64(len(data)) > limit {
		return nil, fmt.Errorf("zip member read failed or exceeds limit")
	}
	return data, nil
}

func zipEntryDigest(entry *zip.File, limit int64) (string, int64, error) {
	if int64(entry.UncompressedSize64) > limit {
		return "", 0, fmt.Errorf("zip member exceeds limit")
	}
	rc, err := entry.Open()
	if err != nil {
		return "", 0, err
	}
	defer rc.Close()
	h := sha256.New()
	n, err := io.Copy(h, io.LimitReader(rc, limit+1))
	if err != nil || n > limit {
		return "", 0, fmt.Errorf("zip member read failed or exceeds limit")
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), n, nil
}

func fileDigest(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

func safeZipName(name string) bool {
	return name != "" && filepath.Base(name) == name && name != "." && name != ".." && !strings.ContainsAny(name, "\\/")
}

func findArtifact(items []ArtifactFact, name string) (ArtifactFact, bool) {
	var found ArtifactFact
	ok := false
	for _, item := range items {
		if item.Name != name {
			continue
		}
		if ok {
			return ArtifactFact{}, false
		}
		found, ok = item, true
	}
	return found, ok
}

func exactDeliveryArtifact(delivery core.DeliveryReceipt, id string) (core.ArtifactRef, bool) {
	var found core.ArtifactRef
	ok := false
	for _, artifact := range delivery.Artifacts {
		if artifact.ID != id {
			continue
		}
		if ok {
			return core.ArtifactRef{}, false
		}
		found, ok = artifact, true
	}
	return found, ok
}

func equalFiles(a, b []File) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func validLogicalID(value string) bool {
	if value == "" || len(value) > 192 || strings.TrimSpace(value) != value {
		return false
	}
	for _, ch := range value {
		if !(ch >= 'A' && ch <= 'Z' || ch >= 'a' && ch <= 'z' || ch >= '0' && ch <= '9' || ch == '.' || ch == '_' || ch == ':' || ch == '-') {
			return false
		}
	}
	return true
}
