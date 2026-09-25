package engineeringevidence

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jiying2007/engineering-platform/internal/core"
)

func gitFixture(t *testing.T) (string, string, string) {
	t.Helper()
	repo := t.TempDir()
	runGit := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	runGit("init", "--object-format=sha1")
	runGit("config", "user.name", "engineering-platform-test")
	runGit("config", "user.email", "test@example.invalid")
	if err := os.WriteFile(filepath.Join(repo, "firmware.c"), []byte("int value = 1;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit("add", "firmware.c")
	runGit("commit", "-m", "base")
	base := runGit("rev-parse", "HEAD")
	if err := os.WriteFile(filepath.Join(repo, "firmware.c"), []byte("int value = 2;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit("add", "firmware.c")
	runGit("commit", "-m", "result")
	result := runGit("rev-parse", "HEAD")
	return repo, base, result
}

func TestCaptureAndVerifyGitChangedTree(t *testing.T) {
	repo, base, result := gitFixture(t)
	ctx := context.Background()
	manifest, err := CaptureGitChange(ctx, repo, base, result)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.BaseCommit != base || manifest.ResultCommit != result ||
		manifest.BaseTree == manifest.ResultTree || manifest.BaseSourceDigest == manifest.ResultSourceDigest {
		t.Fatalf("unexpected changed-tree manifest: %#v", manifest)
	}
	data, err := MarshalGitChangeManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "git-change.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	retained, fileDigest, err := ReadGitChangeManifest(path)
	if err != nil {
		t.Fatal(err)
	}
	observed, err := CaptureGitChange(ctx, repo, base, result)
	if err != nil {
		t.Fatal(err)
	}
	delivery := core.DeliveryReceipt{
		ID: "delivery-git", TaskContractDigest: "sha256:" + strings.Repeat("a", 64),
		RunID: "run-git", BaseCommit: base, ResultCommit: result,
		Artifacts: []core.ArtifactRef{{ID: "git-change", Digest: fileDigest, MediaType: GitArtifactType}},
	}
	delivery.SubjectDigest, err = delivery.CalculateSubjectDigest()
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := VerifyGitImport(GitImportRequest{
		EvidenceID: "evidence-git", RequirementID: "req-git", EvidenceArtifactID: "git-change",
		Delivery: delivery, Manifest: retained, ManifestFileDigest: fileDigest, Observed: observed,
	})
	if err != nil {
		t.Fatal(err)
	}
	if evidence.Result != "PASS" || evidence.Issuer != GitIssuer || evidence.Procedure != GitProcedure ||
		len(evidence.ArtifactRefs) != 1 || evidence.ArtifactRefs[0] != "git-change" {
		t.Fatalf("unexpected Git evidence: %#v", evidence)
	}
}

func TestGitChangedTreeRejectsNoChangeAndDrift(t *testing.T) {
	repo, base, result := gitFixture(t)
	if _, err := CaptureGitChange(context.Background(), repo, base, base); err == nil {
		t.Fatal("unchanged commit accepted as changed-tree evidence")
	}
	manifest, err := CaptureGitChange(context.Background(), repo, base, result)
	if err != nil {
		t.Fatal(err)
	}
	data, err := MarshalGitChangeManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "git-change.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	retained, fileDigest, err := ReadGitChangeManifest(path)
	if err != nil {
		t.Fatal(err)
	}
	delivery := core.DeliveryReceipt{
		ID: "delivery-git", TaskContractDigest: "sha256:" + strings.Repeat("b", 64),
		RunID: "run-git", BaseCommit: base, ResultCommit: result,
		Artifacts: []core.ArtifactRef{{ID: "git-change", Digest: fileDigest, MediaType: GitArtifactType}},
	}
	delivery.SubjectDigest, _ = delivery.CalculateSubjectDigest()
	for _, mode := range []string{"observed", "artifact", "result"} {
		t.Run(mode, func(t *testing.T) {
			req := GitImportRequest{
				EvidenceID: "evidence-git", RequirementID: "req-git", EvidenceArtifactID: "git-change",
				Delivery: delivery, Manifest: retained, ManifestFileDigest: fileDigest, Observed: retained,
			}
			switch mode {
			case "observed":
				req.Observed.ResultSourceDigest = "sha256:" + strings.Repeat("f", 64)
			case "artifact":
				req.Delivery.Artifacts[0].Digest = "sha256:" + strings.Repeat("e", 64)
				req.Delivery.SubjectDigest, _ = req.Delivery.CalculateSubjectDigest()
			case "result":
				req.Delivery.ResultCommit = strings.Repeat("c", 40)
				req.Delivery.SubjectDigest, _ = req.Delivery.CalculateSubjectDigest()
			}
			if _, err := VerifyGitImport(req); err == nil {
				t.Fatal("drifted Git evidence accepted")
			}
		})
	}
}
