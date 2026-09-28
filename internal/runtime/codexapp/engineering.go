package codexapp

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	runtimeprovider "github.com/jiying2007/engineering-platform/internal/runtime"
)

const (
	engineeringWIFTurnTimeout           = 4 * time.Minute
	engineeringSavedLoginTurnTimeout    = 12 * time.Minute
	engineeringWIFAssertionSafetyMargin = 30 * time.Second
	engineeringWIFMinimumWindow         = 90 * time.Second
	engineeringWIFProviderLifetimeLimit = 10 * time.Minute
)

func boundedEngineeringWIFTimeout(assertion []byte, now time.Time) (time.Duration, error) {
	parts := strings.Split(strings.TrimSpace(string(assertion)), ".")
	if len(parts) != 3 {
		return 0, fmt.Errorf("workload identity assertion is not a compact JWT")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return 0, fmt.Errorf("decode workload identity assertion: %w", err)
	}
	var claims struct {
		IssuedAt  int64 `json:"iat"`
		ExpiresAt int64 `json:"exp"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return 0, fmt.Errorf("decode workload identity timing claims: %w", err)
	}
	if claims.IssuedAt <= 0 || claims.ExpiresAt <= claims.IssuedAt {
		return 0, fmt.Errorf("valid workload identity iat/exp required")
	}
	lifetime := time.Duration(claims.ExpiresAt-claims.IssuedAt) * time.Second
	if lifetime > engineeringWIFProviderLifetimeLimit {
		return 0, fmt.Errorf("workload identity assertion lifetime exceeds provider policy")
	}
	remaining := time.Unix(claims.ExpiresAt, 0).Sub(now) - engineeringWIFAssertionSafetyMargin
	if remaining < engineeringWIFMinimumWindow {
		return 0, fmt.Errorf("insufficient workload identity lifetime for retained engineering")
	}
	if remaining > engineeringWIFTurnTimeout {
		remaining = engineeringWIFTurnTimeout
	}
	return remaining, nil
}

type EngineeringObservation struct {
	Status           string
	Output           string
	CommandCount     int
	FailedCommands   int
	FileChangeCount  int
	ApprovalRequests int
}

type EngineeringReceipt struct {
	SchemaVersion                        int    `json:"schema_version"`
	CLI                                  string `json:"cli"`
	Version                              string `json:"version"`
	BinaryDigest                         string `json:"binary_digest"`
	QualificationDigest                  string `json:"qualification_digest"`
	EngineeringConfigDigest              string `json:"engineering_config_digest"`
	CredentialMode                       string `json:"credential_mode"`
	FederationRuleID                     string `json:"federation_rule_id"`
	Model                                string `json:"model"`
	PromptDigest                         string `json:"prompt_digest"`
	ThreadID                             string `json:"thread_id"`
	TurnID                               string `json:"turn_id"`
	TurnStatus                           string `json:"turn_status"`
	Output                               string `json:"output"`
	OutputDigest                         string `json:"output_digest"`
	CommandCount                         int    `json:"command_count"`
	FailedCommands                       int    `json:"failed_commands"`
	FileChangeCount                      int    `json:"file_change_count"`
	ApprovalRequests                     int    `json:"approval_requests"`
	AssertionRemovedBeforeTurn           bool   `json:"assertion_removed_before_turn"`
	CredentialBootstrapRemovedBeforeTurn bool   `json:"credential_bootstrap_removed_before_turn"`
}

func ObserveEngineeringTurn(ctx context.Context, adapter *Adapter, threadID, turnID string) (EngineeringObservation, error) {
	var out EngineeringObservation
	if adapter == nil || !remoteID(threadID) || !remoteID(turnID) {
		return out, ErrLifecycle
	}
	messages := []string{}
	total := 0
	for {
		event, err := adapter.Next(ctx)
		if err != nil {
			return out, err
		}
		if event.Kind == ServerRequest {
			out.ApprovalRequests++
			return out, fmt.Errorf("engineering turn attempted an approval request")
		}
		if event.Kind != Notification {
			continue
		}
		switch event.Message.Method {
		case "item/completed":
			var p struct {
				ThreadID string `json:"threadId"`
				TurnID   string `json:"turnId"`
				Item     struct {
					Type   string `json:"type"`
					ID     string `json:"id"`
					Text   string `json:"text,omitempty"`
					Status string `json:"status,omitempty"`
				} `json:"item"`
			}
			if json.Unmarshal(event.Message.Params, &p) != nil || p.ThreadID != threadID ||
				p.TurnID != turnID || !remoteID(p.Item.ID) || p.Item.Type == "" {
				return out, ErrProtocol
			}
			switch p.Item.Type {
			case "agentMessage":
				if !utf8.ValidString(p.Item.Text) || strings.TrimSpace(p.Item.Text) == "" {
					return out, ErrProtocol
				}
				total += len(p.Item.Text)
				if total > 64<<10 {
					return out, fmt.Errorf("engineering output exceeded limit")
				}
				messages = append(messages, p.Item.Text)
			case "commandExecution":
				out.CommandCount++
				if p.Item.Status == "failed" {
					out.FailedCommands++
				}
			case "fileChange":
				out.FileChangeCount++
			case "userMessage", "reasoning", "plan":
				// Retained timeline-only items.
			default:
				return out, fmt.Errorf("engineering turn emitted disallowed item type %q", p.Item.Type)
			}
		case "turn/completed":
			var p struct {
				ThreadID string `json:"threadId"`
				Turn     struct {
					ID     string `json:"id"`
					Status string `json:"status"`
				} `json:"turn"`
			}
			if json.Unmarshal(event.Message.Params, &p) != nil || p.ThreadID != threadID || p.Turn.ID != turnID {
				return out, ErrProtocol
			}
			out.Status = p.Turn.Status
			if out.Status != "completed" {
				return out, fmt.Errorf("engineering turn ended %q", out.Status)
			}
			out.Output = strings.TrimSpace(strings.Join(messages, "\n"))
			if out.Output == "" {
				return out, fmt.Errorf("engineering turn produced no final agent message")
			}
			return out, nil
		}
	}
}

func EngineeringWIFTurn(ctx context.Context, executable, codexVersion, binaryDigest, qualificationDigest, work, home, ruleID, tokenFile, auditContext, model, prompt string) (EngineeringReceipt, error) {
	var receipt EngineeringReceipt
	if !ValidCodexVersion(codexVersion) || !canonical.ValidDigest(binaryDigest) ||
		!canonical.ValidDigest(qualificationDigest) || strings.TrimSpace(model) == "" || len(model) > 128 ||
		strings.TrimSpace(prompt) == "" || !utf8.ValidString(prompt) || len(prompt) > 64<<10 {
		return receipt, fmt.Errorf("qualified binary, bounded model and prompt required")
	}
	provider, err := NewPinnedWIFEngineeringProvider(executable, binaryDigest)
	if err != nil {
		return receipt, err
	}
	tokenPath, err := privateIdentityToken(tokenFile)
	if err != nil {
		return receipt, err
	}
	defer func() { _ = os.Remove(tokenPath) }()
	versionHome, err := os.MkdirTemp("", "engineering-platform-codex-engineering-version-")
	if err != nil {
		return receipt, err
	}
	defer os.RemoveAll(versionHome)
	versionOut, diagnostics, err := codexVersion(ctx, executable, versionHome)
	actualVersion, parseErr := ParseCodexVersionOutput(string(versionOut))
	if err != nil || parseErr != nil || actualVersion != codexVersion {
		return receipt, fmt.Errorf("engineering execution binary/version drift: got %q want %q; probe=%v parse=%v; stderr=%s", strings.TrimSpace(string(versionOut)), "codex-cli "+codexVersion, err, parseErr, strings.TrimSpace(diagnostics))
	}
	assertion, err := os.ReadFile(tokenPath)
	if err != nil {
		return receipt, fmt.Errorf("read workload identity assertion timing: %w", err)
	}
	runTimeout, err := boundedEngineeringWIFTimeout(assertion, time.Now())
	if err != nil {
		return receipt, err
	}
	runCtx, cancel := context.WithTimeout(ctx, runTimeout)
	defer cancel()
	env := []string{
		"HOME=" + home,
		"OPENAI_FEDERATION_RULE_ID=" + ruleID,
		"OPENAI_IDENTITY_TOKEN_FILE=" + tokenPath,
	}
	if auditContext != "" {
		env = append(env, "OPENAI_WORKLOAD_IDENTITY_CONTEXT="+auditContext)
	}
	cmd, err := provider.Command(runCtx, runtimeprovider.LaunchSpec{Dir: work, Env: env})
	if err != nil {
		return receipt, err
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return receipt, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return receipt, err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &boundedWriter{writer: &stderr, remaining: 128 << 10}
	if err := cmd.Start(); err != nil {
		return receipt, err
	}
	waitDone := make(chan error, 1)
	go func() { waitDone <- cmd.Wait() }()
	client := NewClient(stdout, stdin)
	processJoined := false
	defer func() {
		_ = client.Close()
		cancel()
		if !processJoined {
			select {
			case <-waitDone:
			case <-time.After(5 * time.Second):
				_ = cmd.Process.Kill()
				<-waitDone
			}
		}
	}()
	adapter, err := NewAdapter(client, work)
	if err != nil {
		return receipt, err
	}
	if err := adapter.Initialize(runCtx, "engineering-platform-codex-core-v1"); err != nil {
		return receipt, fmt.Errorf("initialize: %w; stderr=%s", err, strings.TrimSpace(stderr.String()))
	}
	if err := adapter.WarmWorkloadIdentity(runCtx); err != nil {
		return receipt, fmt.Errorf("workload identity prewarm: %w; stderr=%s", err, strings.TrimSpace(stderr.String()))
	}
	if err := os.Remove(tokenPath); err != nil {
		return receipt, fmt.Errorf("remove workload identity assertion before engineering turn: %w", err)
	}
	if _, err := os.Stat(tokenPath); !os.IsNotExist(err) {
		return receipt, fmt.Errorf("workload identity assertion remained reachable before engineering turn")
	}
	threadID, err := adapter.StartEngineeringThread(runCtx, model)
	if err != nil {
		return receipt, fmt.Errorf("thread/start: %w; stderr=%s", err, strings.TrimSpace(stderr.String()))
	}
	turnID, err := adapter.StartTurn(runCtx, prompt)
	if err != nil {
		return receipt, fmt.Errorf("turn/start: %w; stderr=%s", err, strings.TrimSpace(stderr.String()))
	}
	observation, err := ObserveEngineeringTurn(runCtx, adapter, threadID, turnID)
	if err != nil {
		return receipt, err
	}
	_ = client.Close()
	select {
	case <-time.After(5 * time.Second):
		cancel()
		return receipt, fmt.Errorf("app-server did not stop after engineering turn")
	case waitErr := <-waitDone:
		processJoined = true
		if waitErr != nil {
			return receipt, fmt.Errorf("app-server exited after engineering turn: %w", waitErr)
		}
	}
	receipt = EngineeringReceipt{
		SchemaVersion: 2, CLI: "codex-cli", Version: codexVersion,
		BinaryDigest: binaryDigest, QualificationDigest: qualificationDigest,
		EngineeringConfigDigest: EngineeringConfigDigest(),
		CredentialMode: CredentialModeWorkloadIdentity, FederationRuleID: ruleID, Model: model,
		PromptDigest: canonical.BytesDigest([]byte(prompt)), ThreadID: threadID, TurnID: turnID,
		TurnStatus: observation.Status, Output: observation.Output,
		OutputDigest: canonical.BytesDigest([]byte(observation.Output)),
		CommandCount: observation.CommandCount, FailedCommands: observation.FailedCommands,
		FileChangeCount: observation.FileChangeCount, ApprovalRequests: observation.ApprovalRequests,
		AssertionRemovedBeforeTurn:           true,
		CredentialBootstrapRemovedBeforeTurn: false,
	}
	return receipt, receipt.Validate()
}

func EngineeringSavedLoginTurn(ctx context.Context, executable, codexVersion, binaryDigest, qualificationDigest, work, home, savedLoginFile, model, prompt string) (EngineeringReceipt, error) {
	var receipt EngineeringReceipt
	if !ValidCodexVersion(codexVersion) || !canonical.ValidDigest(binaryDigest) ||
		!canonical.ValidDigest(qualificationDigest) || strings.TrimSpace(model) == "" || len(model) > 128 ||
		strings.TrimSpace(prompt) == "" || !utf8.ValidString(prompt) || len(prompt) > 64<<10 {
		return receipt, fmt.Errorf("qualified binary, bounded model and prompt required")
	}
	provider, err := NewPinnedSavedLoginEngineeringProvider(executable, binaryDigest, savedLoginFile)
	if err != nil {
		return receipt, err
	}
	versionHome, err := os.MkdirTemp("", "engineering-platform-codex-engineering-version-")
	if err != nil {
		return receipt, err
	}
	defer os.RemoveAll(versionHome)
	versionOut, diagnostics, err := codexVersion(ctx, executable, versionHome)
	actualVersion, parseErr := ParseCodexVersionOutput(string(versionOut))
	if err != nil || parseErr != nil || actualVersion != codexVersion {
		return receipt, fmt.Errorf("engineering execution binary/version drift: got %q want %q; probe=%v parse=%v; stderr=%s", strings.TrimSpace(string(versionOut)), "codex-cli "+codexVersion, err, parseErr, strings.TrimSpace(diagnostics))
	}
	runCtx, cancel := context.WithTimeout(ctx, engineeringSavedLoginTurnTimeout)
	defer cancel()
	cmd, err := provider.Command(runCtx, runtimeprovider.LaunchSpec{Dir: work, Env: []string{"HOME=" + home}})
	if err != nil {
		return receipt, err
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return receipt, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return receipt, err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &boundedWriter{writer: &stderr, remaining: 128 << 10}
	if err := cmd.Start(); err != nil {
		return receipt, err
	}
	waitDone := make(chan error, 1)
	go func() { waitDone <- cmd.Wait() }()
	client := NewClient(stdout, stdin)
	processJoined := false
	defer func() {
		_ = client.Close()
		cancel()
		if !processJoined {
			select {
			case <-waitDone:
			case <-time.After(5 * time.Second):
				_ = cmd.Process.Kill()
				<-waitDone
			}
		}
	}()
	adapter, err := NewAdapter(client, work)
	if err != nil {
		return receipt, err
	}
	if err := adapter.Initialize(runCtx, "engineering-platform-codex-core-saved-login-v1"); err != nil {
		return receipt, fmt.Errorf("initialize: %w; stderr=%s", err, strings.TrimSpace(stderr.String()))
	}
	if err := adapter.WarmCredential(runCtx); err != nil {
		return receipt, fmt.Errorf("saved ChatGPT login prewarm: %w; stderr=%s", err, strings.TrimSpace(stderr.String()))
	}
	bootstrap := filepath.Join(home, ".codex", "auth.json")
	if err := os.Remove(bootstrap); err != nil {
		return receipt, fmt.Errorf("remove saved ChatGPT login bootstrap before engineering turn: %w", err)
	}
	if _, err := os.Stat(bootstrap); !os.IsNotExist(err) {
		return receipt, fmt.Errorf("saved ChatGPT login bootstrap remained reachable before engineering turn")
	}
	threadID, err := adapter.StartEngineeringThread(runCtx, model)
	if err != nil {
		return receipt, fmt.Errorf("thread/start: %w; stderr=%s", err, strings.TrimSpace(stderr.String()))
	}
	turnID, err := adapter.StartTurn(runCtx, prompt)
	if err != nil {
		return receipt, fmt.Errorf("turn/start: %w; stderr=%s", err, strings.TrimSpace(stderr.String()))
	}
	observation, err := ObserveEngineeringTurn(runCtx, adapter, threadID, turnID)
	if err != nil {
		return receipt, err
	}
	_ = client.Close()
	select {
	case <-time.After(5 * time.Second):
		cancel()
		return receipt, fmt.Errorf("app-server did not stop after engineering turn")
	case waitErr := <-waitDone:
		processJoined = true
		if waitErr != nil {
			return receipt, fmt.Errorf("app-server exited after engineering turn: %w", waitErr)
		}
	}
	receipt = EngineeringReceipt{
		SchemaVersion: 2, CLI: "codex-cli", Version: codexVersion,
		BinaryDigest: binaryDigest, QualificationDigest: qualificationDigest,
		EngineeringConfigDigest: EngineeringConfigDigest(),
		CredentialMode: CredentialModeSavedChatGPTLogin, FederationRuleID: "", Model: model,
		PromptDigest: canonical.BytesDigest([]byte(prompt)), ThreadID: threadID, TurnID: turnID,
		TurnStatus: observation.Status, Output: observation.Output,
		OutputDigest: canonical.BytesDigest([]byte(observation.Output)),
		CommandCount: observation.CommandCount, FailedCommands: observation.FailedCommands,
		FileChangeCount: observation.FileChangeCount, ApprovalRequests: observation.ApprovalRequests,
		AssertionRemovedBeforeTurn:           false,
		CredentialBootstrapRemovedBeforeTurn: true,
	}
	return receipt, receipt.Validate()
}

func (r EngineeringReceipt) Validate() error {
	if r.SchemaVersion != 2 || r.CLI != "codex-cli" || !ValidCodexVersion(r.Version) ||
		!canonical.ValidDigest(r.BinaryDigest) || !canonical.ValidDigest(r.QualificationDigest) ||
		r.EngineeringConfigDigest != EngineeringConfigDigest() ||
		strings.TrimSpace(r.Model) == "" || len(r.Model) > 128 || !canonical.ValidDigest(r.PromptDigest) ||
		!remoteID(r.ThreadID) || !remoteID(r.TurnID) || r.TurnStatus != "completed" ||
		strings.TrimSpace(r.Output) == "" || len(r.Output) > 64<<10 ||
		r.OutputDigest != canonical.BytesDigest([]byte(r.Output)) || r.CommandCount < 0 ||
		r.FailedCommands < 0 || r.FailedCommands > r.CommandCount || r.FileChangeCount < 0 ||
		r.ApprovalRequests != 0 {
		return fmt.Errorf("invalid engineering Codex receipt")
	}
	switch r.CredentialMode {
	case CredentialModeWorkloadIdentity:
		if !validFederationRuleID(r.FederationRuleID) || !r.AssertionRemovedBeforeTurn ||
			r.CredentialBootstrapRemovedBeforeTurn {
			return fmt.Errorf("invalid engineering workload-identity receipt")
		}
	case CredentialModeSavedChatGPTLogin:
		if r.FederationRuleID != "" || r.AssertionRemovedBeforeTurn ||
			!r.CredentialBootstrapRemovedBeforeTurn {
			return fmt.Errorf("invalid engineering saved-login receipt")
		}
	default:
		return fmt.Errorf("invalid engineering credential mode")
	}
	return nil
}
