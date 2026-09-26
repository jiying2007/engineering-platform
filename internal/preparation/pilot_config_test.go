package preparation

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/core"
	"github.com/jiying2007/engineering-platform/internal/strictjson"
)

func TestRetainedPilotPreparationTemplateLoads(t *testing.T) {
	templatePath := filepath.Join("..", "..", "examples", "pilots", "worker-preparation.json.tmpl")
	data, err := os.ReadFile(templatePath)
	if err != nil {
		t.Fatal(err)
	}
	git, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	git, err = filepath.EvalSymlinks(git)
	if err != nil {
		t.Fatal(err)
	}
	contextSource := t.TempDir()
	contextBytes := []byte("frozen pilot requirement\n")
	contextDigest := canonical.BytesDigest(contextBytes)
	contextName := strings.TrimPrefix(contextDigest, "sha256:") + ".bin"
	if err := os.WriteFile(filepath.Join(contextSource, contextName), contextBytes, 0o400); err != nil {
		t.Fatal(err)
	}

	replacements := map[string]string{
		"__PREPARATION_ROOT__": t.TempDir(),
		"__GIT_EXECUTABLE__":   git,
		"__CONTEXT_SOURCE__":   contextSource,
		"__RUN_ID__":           "m1-feature-routing-run",
		"__TASK_DIGEST__":      "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"__RUN_INPUT_DIGEST__": "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		"__REPOSITORY_PATH__":  t.TempDir(),
		"__CONTEXT_DIGEST__":   contextDigest,
	}
	rendered := string(data)
	for key, value := range replacements {
		rendered = strings.ReplaceAll(rendered, key, value)
	}
	if strings.Contains(rendered, "__") {
		t.Fatal("unrendered preparation template placeholder")
	}
	var config Configuration
	if err := strictjson.Decode([]byte(rendered), &config); err != nil {
		t.Fatal(err)
	}
	preparer, err := New(config)
	if err != nil {
		t.Fatal(err)
	}
	if preparer.Subject() != "urn:engineering-platform:worker:codex-pilot" {
		t.Fatalf("unexpected Worker subject %q", preparer.Subject())
	}
	ref := core.ContextRef{
		Source: "m1-pilot-requirement", Type: "DOCUMENT", Version: "v1",
		Digest: contextDigest, Trust: core.ContextApproved,
	}
	reader, err := preparer.source.Open(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(reader)
	closeErr := reader.Close()
	if err != nil || closeErr != nil || string(got) != string(contextBytes) {
		t.Fatalf("frozen context materialization failed: data=%q err=%v close=%v", got, err, closeErr)
	}
	if err := preparer.Close(); err != nil {
		t.Fatal(err)
	}
}
