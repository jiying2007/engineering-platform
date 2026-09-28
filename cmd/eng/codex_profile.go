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
	"github.com/jiying2007/engineering-platform/internal/strictjson"
)

type codexProfileOutput struct {
	CodexExecutable     string                        `json:"codex_executable"`
	Qualification       codexapp.QualificationReceipt `json:"qualification"`
	QualificationDigest string                        `json:"qualification_digest"`
	Profile             codexexec.Profile             `json:"profile"`
	ProfileDigest       string                        `json:"profile_digest"`
	ToolProfile         string                        `json:"tool_profile"`
	ActionGrant         struct {
		Action     string `json:"action"`
		RiskClass  string `json:"risk_class"`
		Capability string `json:"capability"`
	} `json:"action_grant"`
}

func codexProfile(args []string) error {
	fs := flag.NewFlagSet("codex-profile", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	executable := fs.String("codex", "", "absolute native Codex executable")
	qualificationFile := fs.String("qualification", "", "compatibility qualification receipt JSON for this exact binary")
	model := fs.String("model", "", "exact model")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 ||
		strings.TrimSpace(*executable) == "" || strings.TrimSpace(*qualificationFile) == "" ||
		strings.TrimSpace(*model) == "" {
		return fmt.Errorf("usage: eng codex-profile --codex ABSOLUTE_NATIVE_CODEX --qualification RECEIPT.json --model MODEL")
	}
	data, err := os.ReadFile(*qualificationFile)
	if err != nil {
		return err
	}
	var qualification codexapp.QualificationReceipt
	if err := strictjson.Decode(data, &qualification); err != nil {
		return err
	}
	output, err := buildCodexProfile(*executable, *model, qualification)
	if err != nil {
		return err
	}
	printJSON(output)
	return nil
}

func buildCodexProfile(executable, model string, qualification codexapp.QualificationReceipt) (codexProfileOutput, error) {
	var output codexProfileOutput
	if !filepath.IsAbs(executable) || strings.TrimSpace(model) != model || model == "" || len(model) > 128 {
		return output, fmt.Errorf("absolute Codex executable and bounded exact model required")
	}
	if err := qualification.Validate(); err != nil || qualification.ThreadStartModel != model {
		return output, fmt.Errorf("valid compatibility qualification for exact model required")
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
	opened, statErr := file.Stat()
	if statErr != nil || !os.SameFile(info, opened) {
		_ = file.Close()
		return output, fmt.Errorf("Codex binary changed before hash read")
	}
	hash := sha256.New()
	n, copyErr := io.Copy(hash, io.LimitReader(file, (1<<30)+1))
	closeErr := file.Close()
	after, afterErr := os.Lstat(resolved)
	if copyErr != nil || closeErr != nil || afterErr != nil || !os.SameFile(info, after) ||
		n <= 0 || n > 1<<30 || n != info.Size() || after.Size() != info.Size() {
		return output, fmt.Errorf("Codex binary changed during hash read or exceeds limit")
	}
	binaryDigest := "sha256:" + hex.EncodeToString(hash.Sum(nil))
	if binaryDigest != qualification.BinaryDigest ||
		qualification.EngineeringConfigDigest != codexapp.EngineeringConfigDigest() {
		return output, fmt.Errorf("Codex binary/engineering contract does not match compatibility qualification")
	}
	qualificationDigest, err := qualification.Digest()
	if err != nil {
		return output, err
	}

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
	actualVersion, err := codexapp.ParseCodexVersionOutput(string(stdout.data))
	if err != nil || actualVersion != qualification.Version {
		return output, fmt.Errorf(
			"Codex version drift after qualification: got %q want %q",
			strings.TrimSpace(string(stdout.data)),
			"codex-cli "+qualification.Version,
		)
	}

	profile := codexexec.Profile{
		Version:                 2,
		CodexVersion:            actualVersion,
		BinaryDigest:            binaryDigest,
		QualificationDigest:     qualificationDigest,
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
		CodexExecutable:     resolved,
		Qualification:       qualification,
		QualificationDigest: qualificationDigest,
		Profile:             profile,
		ProfileDigest:       digest,
		ToolProfile:         "codex/" + digest,
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
