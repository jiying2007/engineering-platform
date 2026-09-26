package cievidence

import (
	"archive/zip"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/core"
	"github.com/jiying2007/engineering-platform/internal/runtime/codexapp"
)

func fixtureImport(t *testing.T) ImportRequest {
	t.Helper()
	root := t.TempDir()
	commit := strings.Repeat("a", 40)
	files := []File{}
	binaryMembers := map[string][]byte{}
	for i, name := range []string{"codex-qualifier", "control-plane", "eng", "sandbox-guard", "worker"} {
		data := []byte(strings.Repeat(string(rune('A'+i)), i+3))
		binaryMembers[name] = data
		files = append(files, File{Path: name, Digest: canonical.BytesDigest(data), Size: int64(len(data))})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	manifest, err := json.Marshal(files)
	if err != nil {
		t.Fatal(err)
	}
	binaryMembers["file-manifest.json"] = manifest
	binaryMembers["SHA256SUMS"] = []byte("fixture\n")
	binaryZip := writeFixtureZip(t, root, "binaries.zip", binaryMembers)
	binaryInfo, _ := os.Stat(binaryZip)
	binaryDigest := digestFixtureFile(t, binaryZip)

	qualification := codexapp.QualificationReceipt{
		SchemaVersion:                1,
		CLI:                          "codex-cli",
		Version:                      codexapp.QualifiedCodexVersion,
		ReleaseTag:                   codexapp.QualifiedCodexReleaseTag,
		ReleaseCommit:                codexapp.QualifiedCodexReleaseCommit,
		BinaryDigest:                 "sha256:" + strings.Repeat("1", 64),
		StableSchemaDigest:           "sha256:" + strings.Repeat("2", 64),
		ExperimentalSchemaDigest:     "sha256:" + strings.Repeat("3", 64),
		Transport:                    "stdio",
		FreshProcess:                 true,
		ManagedDaemon:                false,
		PerThreadConfigOverride:      false,
		InitializePassed:             true,
		ThreadStartPassed:            true,
		ThreadStartModel:             "gpt-5.6-sol",
		StableSchemaContractChecked:  true,
		ExperimentalSurfaceChecked:   true,
		CredentialSafeConfigDigest:   "sha256:" + strings.Repeat("4", 64),
		CredentialSafeProfileChecked: true,
		EngineeringConfigDigest:      codexapp.EngineeringConfigDigest(),
		EngineeringProfileChecked:    true,
	}
	qdata, err := json.Marshal(qualification)
	if err != nil {
		t.Fatal(err)
	}
	codexZip := writeFixtureZip(t, root, "codex.zip", map[string][]byte{
		"codex-qualification.json": qdata,
		"codex-npm-integrity.txt":  []byte("sha512-fixture\n"),
	})
	codexInfo, _ := os.Stat(codexZip)
	codexDigest := digestFixtureFile(t, codexZip)

	receipt := Receipt{
		SchemaVersion: SchemaVersion,
		Repository:    TrustedRepository,
		Workflow:      TrustedWorkflow,
		Event:         "push",
		SourceSHA:     commit,
		TestedSHA:     commit,
		RunID:         42,
		RunAttempt:    1,
		Jobs: []Job{
			{Name: "codex-app-server-0.155.0-qualification", ID: 101, Conclusion: "success"},
			{Name: "go", ID: 102, Conclusion: "success"},
			{Name: "offline-container-integration", ID: 103, Conclusion: "success"},
			{Name: "postgres-authority-restore-drill", ID: 104, Conclusion: "success"},
		},
		Artifacts: []Artifact{
			{Name: "codex-0.155.0-qualification-" + commit, ID: 201, Digest: codexDigest, Size: codexInfo.Size()},
			{Name: "engineering-binaries-" + commit, ID: 202, Digest: binaryDigest, Size: binaryInfo.Size()},
		},
		Files: files,
	}
	envelope, err := NewEnvelope(receipt)
	if err != nil {
		t.Fatal(err)
	}
	edata, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	envelopeZip := writeFixtureZip(t, root, "evidence.zip", map[string][]byte{"ci-evidence-envelope.json": edata})
	envelopeInfo, _ := os.Stat(envelopeZip)
	envelopeDigest := digestFixtureFile(t, envelopeZip)

	delivery := core.DeliveryReceipt{
		ID:                 "delivery-1",
		TaskContractDigest: "sha256:" + strings.Repeat("5", 64),
		RunID:              "run-1",
		BaseCommit:         strings.Repeat("b", 40),
		ResultCommit:       commit,
		Artifacts: []core.ArtifactRef{{
			ID: "ci-provenance", Digest: envelopeDigest, MediaType: "application/zip",
		}},
	}
	delivery.SubjectDigest, err = delivery.CalculateSubjectDigest()
	if err != nil {
		t.Fatal(err)
	}
	live := LiveFacts{
		Run: RunFact{
			ID: 42, Attempt: 1, Repository: TrustedRepository, Workflow: TrustedWorkflow,
			Event: "push", HeadBranch: "main", HeadSHA: commit, WorkflowPath: ".github/workflows/ci.yml", Status: "completed", Conclusion: "success",
		},
		Jobs: append([]Job(nil), receipt.Jobs...),
		Artifacts: []ArtifactFact{
			{ID: 201, Name: receipt.Artifacts[0].Name, Digest: codexDigest, Size: codexInfo.Size(), RunID: 42, HeadSHA: commit},
			{ID: 202, Name: receipt.Artifacts[1].Name, Digest: binaryDigest, Size: binaryInfo.Size(), RunID: 42, HeadSHA: commit},
			{ID: 203, Name: "trusted-ci-evidence-" + commit, Digest: envelopeDigest, Size: envelopeInfo.Size(), RunID: 42, HeadSHA: commit},
		},
	}
	return ImportRequest{
		EvidenceID:         "evidence-ci-1",
		RequirementID:      "req-ci",
		EvidenceArtifactID: "ci-provenance",
		Delivery:           delivery,
		Files:              ImportFiles{EnvelopeZip: envelopeZip, BinariesZip: binaryZip, CodexZip: codexZip},
		Live:               live,
	}
}

func TestVerifyTrustedImportProducesRequirementBoundEvidence(t *testing.T) {
	req := fixtureImport(t)
	got, err := VerifyTrustedImport(req)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != req.EvidenceID || got.RequirementID != req.RequirementID || got.DeliveryReceiptID != req.Delivery.ID ||
		got.SubjectDigest != req.Delivery.SubjectDigest || got.Issuer != TrustedIssuer || got.Procedure != TrustedProcedure ||
		got.Result != "PASS" || !got.Applicable || len(got.ArtifactRefs) != 1 || got.ArtifactRefs[0] != req.EvidenceArtifactID {
		t.Fatalf("unexpected imported evidence: %#v", got)
	}
}

func TestVerifyTrustedImportFailsClosedOnAuthorityDrift(t *testing.T) {
	for _, kind := range []string{"commit", "job", "duplicate-job", "artifact", "delivery-artifact", "subject", "expired"} {
		t.Run(kind, func(t *testing.T) {
			req := fixtureImport(t)
			switch kind {
			case "commit":
				req.Live.Run.HeadSHA = strings.Repeat("c", 40)
			case "job":
				req.Live.Jobs[1].Conclusion = "failure"
			case "duplicate-job":
				req.Live.Jobs = append(req.Live.Jobs, req.Live.Jobs[1])
			case "artifact":
				req.Live.Artifacts[0].Digest = "sha256:" + strings.Repeat("f", 64)
			case "delivery-artifact":
				req.Delivery.Artifacts[0].Digest = "sha256:" + strings.Repeat("e", 64)
				req.Delivery.SubjectDigest, _ = req.Delivery.CalculateSubjectDigest()
			case "subject":
				req.Delivery.SubjectDigest = "sha256:" + strings.Repeat("d", 64)
			case "expired":
				req.Live.Artifacts[1].Expired = true
			}
			if _, err := VerifyTrustedImport(req); err == nil {
				t.Fatal("authority drift accepted")
			}
		})
	}
}

func TestVerifyTrustedImportRejectsTamperedBinaryBytes(t *testing.T) {
	req := fixtureImport(t)
	f, err := os.OpenFile(req.Files.BinariesZip, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.Write([]byte("tamper"))
	_ = f.Close()
	if _, err := VerifyTrustedImport(req); err == nil {
		t.Fatal("tampered archive accepted")
	}
}

func writeFixtureZip(t *testing.T, root, name string, members map[string][]byte) string {
	t.Helper()
	path := filepath.Join(root, name)
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(f)
	names := make([]string, 0, len(members))
	for name := range members {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		header := &zip.FileHeader{Name: name, Method: zip.Deflate}
		header.SetMode(0o600)
		entry, err := w.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write(members[name]); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func digestFixtureFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return canonical.BytesDigest(data)
}

func TestReadEnvelopeZipRejectsExtraMember(t *testing.T) {
	req := fixtureImport(t)
	envelope, err := ReadEnvelopeZip(req.Files.EnvelopeZip)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(envelope)
	bad := writeFixtureZip(t, filepath.Dir(req.Files.EnvelopeZip), "bad-envelope.zip", map[string][]byte{
		"ci-evidence-envelope.json": data,
		"extra.txt":                 []byte("not allowed"),
	})
	if _, err := ReadEnvelopeZip(bad); err == nil {
		t.Fatal("extra envelope member accepted")
	}
}

func TestBinaryManifestMustMatchEnvelopeFacts(t *testing.T) {
	req := fixtureImport(t)
	envelope, err := ReadEnvelopeZip(req.Files.EnvelopeZip)
	if err != nil {
		t.Fatal(err)
	}
	changed := append([]File(nil), envelope.Receipt.Files...)
	changed[0].Size++
	if err := verifyBinaryArchive(req.Files.BinariesZip, changed); err == nil {
		t.Fatal("mismatched binary manifest accepted")
	}
}
