package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/jiying2007/engineering-platform/internal/codexexec"
	"github.com/jiying2007/engineering-platform/internal/runtime/codexapp"
)

type codexProfileOutput struct {
	CodexExecutable string            `json:"codex_executable"`
	Profile         codexexec.Profile `json:"profile"`
	ProfileDigest   string            `json:"profile_digest"`
	ToolProfile     string            `json:"tool_profile"`
	ActionGrant     struct {
		Action     string `json:"action"`
		RiskClass  string `json:"risk_class"`
		Capability string `json:"capability"`
	} `json:"action_grant"`
}

func codexProfile(args []string) error {
	fs := flag.NewFlagSet("codex-profile", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	executable := fs.String("codex", "", "absolute native Codex executable")
	model := fs.String("model", "", "exact managed-workspace model")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 ||
		strings.TrimSpace(*executable) == "" || strings.TrimSpace(*model) == "" {
		return fmt.Errorf("usage: eng codex-profile --codex ABSOLUTE_NATIVE_CODEX --model MODEL")
	}
	output, err := buildCodexProfile(*executable, *model)
	if err != nil {
		return err
	}
	printJSON(output)
	return nil
}

func buildCodexProfile(executable, model string) (codexProfileOutput, error) {
	var output codexProfileOutput
	if !filepath.IsAbs(executable) || strings.TrimSpace(model) != model || model == "" || len(model) > 128 {
		return output, fmt.Errorf("absolute Codex executable and bounded exact model required")
	}
	clean := filepath.Clean(executable)
	resolved, err := filepath.EvalSymlinks(clean)
	if err != nil {
		return output, fmt.Errorf("resolve Codex executable: %w", err)
	}
	info, err := os.Lstat(resolved)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o022 != 0 || info.Mode().Perm()&0o111 == 0 {
		return output, fmt.Errorf("trusted regular executable Codex binary required")
	}
	file, err := os.Open(resolved)
	if err != nil {
		return output, err
	}
	hash := sha256.New()
	n, copyErr := io.Copy(hash, io.LimitReader(file, (1<<30)+1))
	closeErr := file.Close()
	if copyErr != nil || closeErr != nil || n <= 0 || n > 1<<30 || n != info.Size() {
		return output, fmt.Errorf("Codex binary hash read failed or exceeds limit")
	}
	binaryDigest := "sha256:" + hex.EncodeToString(hash.Sum(nil))

	home, err := os.MkdirTemp("", "engineering-platform-codex-profile-")
	if err != nil {
		return output, err
	}
	defer os.RemoveAll(home)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, resolved, "--version")
	cmd.Env = []string{
		"HOME=" + home,
		"XDG_CONFIG_HOME=" + home,
		"LANG=C",
		"LC_ALL=C",
		"PATH=" + filepath.Dir(resolved) + ":/usr/bin:/bin",
	}
	var stdout, stderr boundedProfileOutput
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	cmd.WaitDelay = time.Second
	if err := cmd.Run(); err != nil {
		return output, fmt.Errorf("Codex version probe failed: %w", err)
	}
	if got := strings.TrimSpace(string(stdout.data)); got != "codex-cli "+codexapp.QualifiedCodexVersion {
		return output, fmt.Errorf(
			"Codex version mismatch: got %q want %q",
			got,
			"codex-cli "+codexapp.QualifiedCodexVersion,
		)
	}

	profile := codexexec.Profile{
		Version:                 1,
		CodexVersion:            codexapp.QualifiedCodexVersion,
		BinaryDigest:            binaryDigest,
		EngineeringConfigDigest: codexapp.EngineeringConfigDigest(),
		Model:                   model,
		Sandbox:                 "workspace-write",
		ApprovalPolicy:          "never",
		ToolNetwork:             false,
	}
	digest, err := profile.Digest()
	if err != nil {
		return output, err
	}
	output = codexProfileOutput{
		CodexExecutable: resolved,
		Profile:         profile,
		ProfileDigest:   digest,
		ToolProfile:     "codex/" + digest,
	}
	output.ActionGrant.Action = codexexec.Action
	output.ActionGrant.RiskClass = "CONTROLLED_MUTATION"
	output.ActionGrant.Capability = digest
	return output, nil
}

type boundedProfileOutput struct {
	data []byte
}

func (b *boundedProfileOutput) Write(p []byte) (int, error) {
	if len(b.data)+len(p) > 8<<10 {
		return 0, fmt.Errorf("Codex version output exceeds limit")
	}
	b.data = append(b.data, p...)
	return len(p), nil
}
