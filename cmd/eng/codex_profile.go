package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jiying2007/engineering-platform/internal/codexexec"
	"github.com/jiying2007/engineering-platform/internal/provideridentity"
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
	providerID := fs.String("provider", "", "exact provider ID")
	credentialMode := fs.String("credential", "", "exact credential mode")
	executionMode := fs.String("execution", "", "exact execution mode")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 ||
		strings.TrimSpace(*executable) == "" || strings.TrimSpace(*qualificationFile) == "" ||
		strings.TrimSpace(*model) == "" || strings.TrimSpace(*providerID) == "" ||
		strings.TrimSpace(*credentialMode) == "" || strings.TrimSpace(*executionMode) == "" {
		return fmt.Errorf("usage: eng codex-profile --codex ABSOLUTE_NATIVE_CODEX --qualification RECEIPT.json --model MODEL --provider PROVIDER --credential CREDENTIAL --execution EXECUTION")
	}
	data, err := os.ReadFile(*qualificationFile)
	if err != nil {
		return err
	}
	var qualification codexapp.QualificationReceipt
	if err := strictjson.Decode(data, &qualification); err != nil {
		return err
	}
	provider, err := provideridentity.New(*providerID, *credentialMode, *executionMode)
	if err != nil {
		return err
	}
	output, err := buildCodexProfile(*executable, *model, qualification, provider)
	if err != nil {
		return err
	}
	printJSON(output)
	return nil
}

func buildCodexProfile(executable, model string, qualification codexapp.QualificationReceipt, provider provideridentity.Identity) (codexProfileOutput, error) {
	var output codexProfileOutput
	if !filepath.IsAbs(executable) || strings.TrimSpace(model) != model || model == "" || len(model) > 128 ||
		provider.Validate() != nil {
		return output, fmt.Errorf("absolute Codex executable, bounded exact model and admitted provider identity required")
	}
	if err := qualification.Validate(); err != nil || qualification.ThreadStartModel != model {
		return output, fmt.Errorf("valid compatibility qualification for exact model required")
	}
	clean := filepath.Clean(executable)
	resolved, err := filepath.EvalSymlinks(clean)
	if err != nil {
		return output, fmt.Errorf("resolve Codex executable: %w", err)
	}
	qualificationDigest, err := qualification.Digest()
	if err != nil {
		return output, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := codexapp.VerifyExecutableQualification(ctx, resolved, model, qualification); err != nil {
		return output, fmt.Errorf("Codex executable qualification: %w", err)
	}

	profile := codexexec.Profile{
		Version:                 3,
		Provider:                provider,
		CodexVersion:            qualification.Version,
		BinaryDigest:            qualification.BinaryDigest,
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
