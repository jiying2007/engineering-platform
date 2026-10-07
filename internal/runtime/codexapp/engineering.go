package codexapp

import (
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
	"github.com/jiying2007/engineering-platform/internal/processscope"
	"github.com/jiying2007/engineering-platform/internal/provideridentity"
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
	History          EngineeringHistory
}

type EngineeringReceipt struct {
	ProcessScope                         processscope.Proof        `json:"process_scope"`
	SchemaVersion                        int                       `json:"schema_version"`
	CLI                                  string                    `json:"cli"`
	Version                              string                    `json:"version"`
	BinaryDigest                         string                    `json:"binary_digest"`
	QualificationDigest                  string                    `json:"qualification_digest"`
	EngineeringConfigDigest              string                    `json:"engineering_config_digest"`
	Provider                             provideridentity.Identity `json:"provider"`
	FederationRuleID                     string                    `json:"federation_rule_id"`
	Model                                string                    `json:"model"`
	PromptDigest                         string                    `json:"prompt_digest"`
	ItemHistoryDigest                    string                    `json:"item_history_digest"`
	ItemHistoryCount                     int                       `json:"item_history_count"`
	ThreadID                             string                    `json:"thread_id"`
	TurnID                               string                    `json:"turn_id"`
	TurnStatus                           string                    `json:"turn_status"`
	Output                               string                    `json:"output"`
	OutputDigest                         string                    `json:"output_digest"`
	CommandCount                         int                       `json:"command_count"`
	FailedCommands                       int                       `json:"failed_commands"`
	FileChangeCount                      int                       `json:"file_change_count"`
	ApprovalRequests                     int                       `json:"approval_requests"`
	AssertionRemovedBeforeTurn           bool                      `json:"assertion_removed_before_turn"`
	CredentialBootstrapRemovedBeforeTurn bool                      `json:"credential_bootstrap_removed_before_turn"`
}

func ObserveEngineeringTurn(ctx context.Context, adapter *Adapter, threadID, turnID string) (EngineeringObservation, error) {
	var out EngineeringObservation
	if adapter == nil || !remoteID(threadID) || !remoteID(turnID) {
		return out, ErrLifecycle
	}
	out.History = EngineeringHistory{Version: 1, ThreadID: threadID, TurnID: turnID, Items: [][]byte{}}
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
			if err := out.History.appendItem(event.Message.Params); err != nil {
				return out, err
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
			if err := out.History.complete(event.Message.Params); err != nil {
				return out, err
			}
			out.Status = p.Turn.Status
			if out.Status == "interrupted" {
				return out, ErrEngineeringInterrupted
			}
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

func retainEngineeringHistory(controller EngineeringController, history EngineeringHistory) (string, int, error) {
	digest, err := history.Digest()
	if err != nil {
		return "", 0, err
	}
	if controller != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := controller.RetainHistory(ctx, history); err != nil {
			return "", 0, err
		}
	}
	return digest, len(history.Items), nil
}

func EngineeringWIFTurn(ctx context.Context, executable, qualifiedVersion, binaryDigest, qualificationDigest, work, home, ruleID, tokenFile, auditContext, model, prompt string, controller EngineeringController) (receipt EngineeringReceipt, runErr error) {
	if !ValidCodexVersion(qualifiedVersion) || !canonical.ValidDigest(binaryDigest) ||
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
	if err != nil || parseErr != nil || actualVersion != qualifiedVersion {
		return receipt, fmt.Errorf("engineering execution binary/version drift: got %q want %q; probe=%v parse=%v; stderr=%s", strings.TrimSpace(string(versionOut)), "codex-cli "+qualifiedVersion, err, parseErr, strings.TrimSpace(diagnostics))
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
	var stderr diagnosticBuffer
	cmd.Stderr = &boundedWriter{writer: &stderr, remaining: 128 << 10}
	scope, err := processscope.Start(cmd)
	if err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return receipt, err
	}
	client := NewClient(stdout, stdin)
	var proof processscope.Proof
	controlStarted := false
	var observation EngineeringObservation
	defer func() {
		_ = client.Close()
		cancel()
		if !proof.Quiescent() {
			stopCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
			var stopErr error
			proof, stopErr = scope.Stop(stopCtx)
			stop()
			if !proof.Quiescent() {
				receipt = EngineeringReceipt{}
				runErr = fmt.Errorf("runtime process-tree termination unconfirmed: %v", stopErr)
			}
		}
		if controller != nil && controlStarted {
			terminal := observation.Status
			if terminal == "" {
				terminal = "unknown"
			}
			reportCtx, reportCancel := context.WithTimeout(context.Background(), 6*time.Second)
			closeErr := controller.Close(reportCtx, terminal, proof)
			reportCancel()
			if closeErr != nil && runErr == nil {
				receipt = EngineeringReceipt{}
				runErr = closeErr
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
	controlStarted = true
	observation, err = observeControlledEngineering(runCtx, adapter, threadID, turnID, controller)
	historyDigest, historyCount, historyErr := retainEngineeringHistory(controller, observation.History)
	if err != nil {
		if historyErr != nil && len(observation.History.Completion) != 0 {
			return receipt, fmt.Errorf("%w; private item history retention failed: %v", err, historyErr)
		}
		return receipt, err
	}
	if historyErr != nil {
		return receipt, historyErr
	}
	_ = client.Close()
	joinCtx, joinCancel := context.WithTimeout(ctx, 5*time.Second)
	proof, err = scope.Wait(joinCtx)
	joinCancel()
	if err != nil || !proof.Quiescent() {
		return receipt, fmt.Errorf("app-server namespace did not terminate cleanly: %v", err)
	}
	receipt = EngineeringReceipt{
		SchemaVersion: 5, ProcessScope: proof, CLI: "codex-cli", Version: qualifiedVersion,
		BinaryDigest: binaryDigest, QualificationDigest: qualificationDigest,
		EngineeringConfigDigest: EngineeringConfigDigest(),
		Provider:                provideridentity.OpenAIWIFUnattended(), FederationRuleID: ruleID, Model: model,
		PromptDigest: canonical.BytesDigest([]byte(prompt)), ItemHistoryDigest: historyDigest, ItemHistoryCount: historyCount,
		ThreadID: threadID, TurnID: turnID,
		TurnStatus: observation.Status, Output: observation.Output,
		OutputDigest: canonical.BytesDigest([]byte(observation.Output)),
		CommandCount: observation.CommandCount, FailedCommands: observation.FailedCommands,
		FileChangeCount: observation.FileChangeCount, ApprovalRequests: observation.ApprovalRequests,
		AssertionRemovedBeforeTurn:           true,
		CredentialBootstrapRemovedBeforeTurn: false,
	}
	return receipt, receipt.Validate()
}

func EngineeringSavedLoginTurn(ctx context.Context, executable, qualifiedVersion, binaryDigest, qualificationDigest, work, home, savedLoginFile, model, prompt string, controller EngineeringController) (receipt EngineeringReceipt, runErr error) {
	if !ValidCodexVersion(qualifiedVersion) || !canonical.ValidDigest(binaryDigest) ||
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
	if err != nil || parseErr != nil || actualVersion != qualifiedVersion {
		return receipt, fmt.Errorf("engineering execution binary/version drift: got %q want %q; probe=%v parse=%v; stderr=%s", strings.TrimSpace(string(versionOut)), "codex-cli "+qualifiedVersion, err, parseErr, strings.TrimSpace(diagnostics))
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
	var stderr diagnosticBuffer
	cmd.Stderr = &boundedWriter{writer: &stderr, remaining: 128 << 10}
	scope, err := processscope.Start(cmd)
	if err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return receipt, err
	}
	client := NewClient(stdout, stdin)
	var proof processscope.Proof
	controlStarted := false
	var observation EngineeringObservation
	defer func() {
		_ = client.Close()
		cancel()
		if !proof.Quiescent() {
			stopCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
			var stopErr error
			proof, stopErr = scope.Stop(stopCtx)
			stop()
			if !proof.Quiescent() {
				receipt = EngineeringReceipt{}
				runErr = fmt.Errorf("runtime process-tree termination unconfirmed: %v", stopErr)
			}
		}
		if controller != nil && controlStarted {
			terminal := observation.Status
			if terminal == "" {
				terminal = "unknown"
			}
			reportCtx, reportCancel := context.WithTimeout(context.Background(), 6*time.Second)
			closeErr := controller.Close(reportCtx, terminal, proof)
			reportCancel()
			if closeErr != nil && runErr == nil {
				receipt = EngineeringReceipt{}
				runErr = closeErr
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
	controlStarted = true
	observation, err = observeControlledEngineering(runCtx, adapter, threadID, turnID, controller)
	historyDigest, historyCount, historyErr := retainEngineeringHistory(controller, observation.History)
	if err != nil {
		if historyErr != nil && len(observation.History.Completion) != 0 {
			return receipt, fmt.Errorf("%w; private item history retention failed: %v", err, historyErr)
		}
		return receipt, err
	}
	if historyErr != nil {
		return receipt, historyErr
	}
	_ = client.Close()
	joinCtx, joinCancel := context.WithTimeout(ctx, 5*time.Second)
	proof, err = scope.Wait(joinCtx)
	joinCancel()
	if err != nil || !proof.Quiescent() {
		return receipt, fmt.Errorf("app-server namespace did not terminate cleanly: %v", err)
	}
	receipt = EngineeringReceipt{
		SchemaVersion: 5, ProcessScope: proof, CLI: "codex-cli", Version: qualifiedVersion,
		BinaryDigest: binaryDigest, QualificationDigest: qualificationDigest,
		EngineeringConfigDigest: EngineeringConfigDigest(),
		Provider:                provideridentity.OpenAIChatGPTTrustedSelfHosted(), FederationRuleID: "", Model: model,
		PromptDigest: canonical.BytesDigest([]byte(prompt)), ItemHistoryDigest: historyDigest, ItemHistoryCount: historyCount,
		ThreadID: threadID, TurnID: turnID,
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
	if r.SchemaVersion != 5 || !r.ProcessScope.Quiescent() || r.CLI != "codex-cli" || !ValidCodexVersion(r.Version) ||
		!canonical.ValidDigest(r.BinaryDigest) || !canonical.ValidDigest(r.QualificationDigest) ||
		r.EngineeringConfigDigest != EngineeringConfigDigest() || r.Provider.Validate() != nil ||
		strings.TrimSpace(r.Model) == "" || len(r.Model) > 128 || !canonical.ValidDigest(r.PromptDigest) ||
		!canonical.ValidDigest(r.ItemHistoryDigest) || r.ItemHistoryCount < 0 || r.ItemHistoryCount > MaxEngineeringHistoryItems ||
		!remoteID(r.ThreadID) || !remoteID(r.TurnID) || r.TurnStatus != "completed" ||
		strings.TrimSpace(r.Output) == "" || len(r.Output) > 64<<10 ||
		r.OutputDigest != canonical.BytesDigest([]byte(r.Output)) || r.CommandCount < 0 ||
		r.FailedCommands < 0 || r.FailedCommands > r.CommandCount || r.FileChangeCount < 0 ||
		r.ApprovalRequests != 0 {
		return fmt.Errorf("invalid engineering Codex receipt")
	}
	switch r.Provider.CredentialMode {
	case provideridentity.CredentialWorkloadIdentity:
		if !validFederationRuleID(r.FederationRuleID) || !r.AssertionRemovedBeforeTurn ||
			r.CredentialBootstrapRemovedBeforeTurn {
			return fmt.Errorf("invalid engineering workload-identity receipt")
		}
	case provideridentity.CredentialChatGPTSession:
		if r.FederationRuleID != "" || r.AssertionRemovedBeforeTurn ||
			!r.CredentialBootstrapRemovedBeforeTurn {
			return fmt.Errorf("invalid engineering saved-login receipt")
		}
	default:
		return fmt.Errorf("invalid engineering provider credential mode")
	}
	return nil
}
