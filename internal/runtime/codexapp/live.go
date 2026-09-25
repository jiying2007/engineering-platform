package codexapp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	runtimeprovider "github.com/jiying2007/engineering-platform/internal/runtime"
)

const (
	LiveProbePrompt   = "Reply with only: engineering-platform live qualification"
	LiveProbeExpected = "engineering-platform live qualification"
)

type LiveReceipt struct {
	SchemaVersion              int    `json:"schema_version"`
	CLI                        string `json:"cli"`
	Version                    string `json:"version"`
	BinaryDigest               string `json:"binary_digest"`
	CredentialSafeConfigDigest string `json:"credential_safe_config_digest"`
	CredentialMode             string `json:"credential_mode"`
	FederationRuleID           string `json:"federation_rule_id"`
	Model                      string `json:"model"`
	PromptDigest               string `json:"prompt_digest"`
	ThreadID                   string `json:"thread_id"`
	TurnID                     string `json:"turn_id"`
	TurnStatus                 string `json:"turn_status"`
	Output                     string `json:"output"`
	OutputDigest               string `json:"output_digest"`
	ApprovalRequests           int    `json:"approval_requests"`
	UnexpectedToolUse          bool   `json:"unexpected_tool_use"`
	AssertionRemovedBeforeTurn bool   `json:"assertion_removed_before_turn"`
}

type TurnObservation struct {
	Status            string
	Output            string
	ApprovalRequests  int
	UnexpectedToolUse bool
}

func ObserveTurn(ctx context.Context, adapter *Adapter, threadID, turnID string) (TurnObservation, error) {
	var out TurnObservation
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
			return out, fmt.Errorf("live qualification attempted a tool approval")
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
					Type string `json:"type"`
					ID   string `json:"id"`
					Text string `json:"text,omitempty"`
				} `json:"item"`
			}
			if json.Unmarshal(event.Message.Params, &p) != nil || p.ThreadID != threadID || p.TurnID != turnID || !remoteID(p.Item.ID) || p.Item.Type == "" {
				return out, ErrProtocol
			}
			switch p.Item.Type {
			case "agentMessage":
				if !utf8.ValidString(p.Item.Text) || strings.TrimSpace(p.Item.Text) == "" {
					return out, ErrProtocol
				}
				total += len(p.Item.Text)
				if total > 64<<10 {
					return out, fmt.Errorf("live qualification output exceeded limit")
				}
				messages = append(messages, p.Item.Text)
			case "userMessage", "reasoning", "plan":
				// These are non-effect timeline items. They carry no authority.
			default:
				out.UnexpectedToolUse = true
				return out, fmt.Errorf("live qualification emitted unexpected item type %q", p.Item.Type)
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
				return out, fmt.Errorf("live qualification turn ended %q", out.Status)
			}
			out.Output = strings.TrimSpace(strings.Join(messages, "\n"))
			if out.Output == "" {
				return out, fmt.Errorf("live qualification produced no completed agent message")
			}
			return out, nil
		}
	}
}

// LiveWIFProbe performs exactly one read-only model turn using Codex workload
// identity. It never accepts tool approvals and retains no token bytes or path.
// The host prewarms the WIF exchange and removes the upstream assertion before
// any thread/model turn starts. The caller still owns directory cleanup.
func LiveWIFProbe(ctx context.Context, executable, binaryDigest, work, home, ruleID, tokenFile, auditContext, model string) (LiveReceipt, error) {
	var receipt LiveReceipt
	if !canonical.ValidDigest(binaryDigest) || strings.TrimSpace(model) == "" || len(model) > 128 {
		return receipt, fmt.Errorf("qualified binary digest and bounded model required")
	}
	provider, err := NewPinnedWIFProvider(executable, binaryDigest)
	if err != nil {
		return receipt, err
	}
	tokenPath, err := privateIdentityToken(tokenFile)
	if err != nil {
		return receipt, err
	}
	defer func() { _ = os.Remove(tokenPath) }()
	versionHome, err := os.MkdirTemp("", "engineering-platform-codex-live-version-")
	if err != nil {
		return receipt, err
	}
	defer os.RemoveAll(versionHome)
	versionOut, diagnostics, err := codexVersion(ctx, executable, versionHome)
	if err != nil || strings.TrimSpace(string(versionOut)) != "codex-cli "+QualifiedCodexVersion {
		return receipt, fmt.Errorf("live qualification requires exact codex-cli %s: %v; stderr=%s", QualifiedCodexVersion, err, strings.TrimSpace(diagnostics))
	}
	probeCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	env := []string{
		"HOME=" + home,
		"OPENAI_FEDERATION_RULE_ID=" + ruleID,
		"OPENAI_IDENTITY_TOKEN_FILE=" + tokenPath,
	}
	if auditContext != "" {
		env = append(env, "OPENAI_WORKLOAD_IDENTITY_CONTEXT="+auditContext)
	}
	cmd, err := provider.Command(probeCtx, runtimeprovider.LaunchSpec{Dir: work, Env: env})
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
	cmd.Stderr = &boundedWriter{writer: &stderr, remaining: 64 << 10}
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
	if err := adapter.Initialize(probeCtx, "engineering-platform-codex-live-wif-v1"); err != nil {
		return receipt, fmt.Errorf("initialize: %w; stderr=%s", err, strings.TrimSpace(stderr.String()))
	}
	if err := adapter.WarmWorkloadIdentity(probeCtx); err != nil {
		return receipt, fmt.Errorf("workload identity prewarm: %w; stderr=%s", err, strings.TrimSpace(stderr.String()))
	}
	if err := os.Remove(tokenPath); err != nil {
		return receipt, fmt.Errorf("remove workload identity assertion before model turn: %w", err)
	}
	if _, err := os.Stat(tokenPath); !os.IsNotExist(err) {
		return receipt, fmt.Errorf("workload identity assertion remained reachable before model turn")
	}
	assertionRemoved := true
	threadID, err := adapter.StartThread(probeCtx, model)
	if err != nil {
		return receipt, fmt.Errorf("thread/start: %w; stderr=%s", err, strings.TrimSpace(stderr.String()))
	}
	turnID, err := adapter.StartTurn(probeCtx, LiveProbePrompt)
	if err != nil {
		return receipt, fmt.Errorf("turn/start: %w; stderr=%s", err, strings.TrimSpace(stderr.String()))
	}
	observation, err := ObserveTurn(probeCtx, adapter, threadID, turnID)
	if err != nil {
		return receipt, err
	}
	_ = client.Close()
	select {
	case <-time.After(5 * time.Second):
		cancel()
		return receipt, fmt.Errorf("app-server did not stop after completed live qualification")
	case waitErr := <-waitDone:
		processJoined = true
		if waitErr != nil {
			return receipt, fmt.Errorf("app-server exited after live qualification: %w", waitErr)
		}
	}
	receipt = LiveReceipt{
		SchemaVersion:              1,
		CLI:                        "codex-cli",
		Version:                    QualifiedCodexVersion,
		BinaryDigest:               binaryDigest,
		CredentialSafeConfigDigest: canonical.BytesDigest([]byte(credentialSafeConfig)),
		CredentialMode:             "workload_identity",
		FederationRuleID:           ruleID,
		Model:                      model,
		PromptDigest:               canonical.BytesDigest([]byte(LiveProbePrompt)),
		ThreadID:                   threadID,
		TurnID:                     turnID,
		TurnStatus:                 observation.Status,
		Output:                     observation.Output,
		OutputDigest:               canonical.BytesDigest([]byte(observation.Output)),
		ApprovalRequests:           observation.ApprovalRequests,
		UnexpectedToolUse:          observation.UnexpectedToolUse,
		AssertionRemovedBeforeTurn: assertionRemoved,
	}
	return receipt, receipt.Validate()
}

func (r LiveReceipt) Validate() error {
	if r.SchemaVersion != 1 || r.CLI != "codex-cli" || r.Version != QualifiedCodexVersion || !canonical.ValidDigest(r.BinaryDigest) || r.CredentialSafeConfigDigest != canonical.BytesDigest([]byte(credentialSafeConfig)) || r.CredentialMode != "workload_identity" || !validFederationRuleID(r.FederationRuleID) || strings.TrimSpace(r.Model) == "" || len(r.Model) > 128 || r.PromptDigest != canonical.BytesDigest([]byte(LiveProbePrompt)) || !remoteID(r.ThreadID) || !remoteID(r.TurnID) || r.TurnStatus != "completed" || r.Output != LiveProbeExpected || r.OutputDigest != canonical.BytesDigest([]byte(r.Output)) || r.ApprovalRequests != 0 || r.UnexpectedToolUse || !r.AssertionRemovedBeforeTurn {
		return fmt.Errorf("invalid live qualification receipt")
	}
	return nil
}

func MarshalLiveReceipt(r LiveReceipt) ([]byte, error) {
	if err := r.Validate(); err != nil {
		return nil, err
	}
	return json.MarshalIndent(r, "", "  ")
}
