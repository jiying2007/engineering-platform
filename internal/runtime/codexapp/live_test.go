package codexapp

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"
)

func liveAdapter(t *testing.T) (*Adapter, io.ReadWriter) {
	t.Helper()
	client, peer := pair(t)
	adapter, err := NewAdapter(client, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	adapter.initialized = true
	adapter.thread = "thread-live"
	adapter.turn = "turn-live"
	return adapter, peer
}

func TestObserveTurnRetainsCompletedAgentMessage(t *testing.T) {
	adapter, peer := liveAdapter(t)
	go func() {
		_, _ = io.WriteString(peer, `{"method":"item/completed","params":{"threadId":"thread-live","turnId":"turn-live","item":{"type":"reasoning","id":"reasoning-1"}}}`+"\n")
		_, _ = io.WriteString(peer, `{"method":"item/completed","params":{"threadId":"thread-live","turnId":"turn-live","item":{"type":"agentMessage","id":"message-1","text":"engineering-platform live qualification"}}`+"\n")
		_, _ = io.WriteString(peer, `{"method":"turn/completed","params":{"threadId":"thread-live","turn":{"id":"turn-live","status":"completed"}}}`+"\n")
	}()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	got, err := ObserveTurn(ctx, adapter, "thread-live", "turn-live")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "completed" || got.Output != "engineering-platform live qualification" || got.ApprovalRequests != 0 || got.UnexpectedToolUse {
		t.Fatalf("unexpected observation: %#v", got)
	}
}

func TestObserveTurnRejectsApprovalAndToolItems(t *testing.T) {
	t.Run("approval", func(t *testing.T) {
		adapter, peer := liveAdapter(t)
		done := make(chan Message, 1)
		go func() {
			_, _ = io.WriteString(peer, `{"id":"approval","method":"item/commandExecution/requestApproval","params":{"threadId":"thread-live","turnId":"turn-live"}}`+"\n")
			var reply Message
			_ = json.NewDecoder(peer).Decode(&reply)
			done <- reply
		}()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		got, err := ObserveTurn(ctx, adapter, "thread-live", "turn-live")
		if err == nil || got.ApprovalRequests != 1 {
			t.Fatalf("approval accepted: %#v %v", got, err)
		}
		reply := <-done
		var result struct {
			Decision string `json:"decision"`
		}
		if json.Unmarshal(reply.Result, &result) != nil || result.Decision != "decline" {
			t.Fatalf("approval was not declined: %#v", reply)
		}
	})
	t.Run("tool-item", func(t *testing.T) {
		adapter, peer := liveAdapter(t)
		go func() {
			_, _ = io.WriteString(peer, `{"method":"item/completed","params":{"threadId":"thread-live","turnId":"turn-live","item":{"type":"commandExecution","id":"cmd-1"}}}`+"\n")
		}()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		got, err := ObserveTurn(ctx, adapter, "thread-live", "turn-live")
		if err == nil || !got.UnexpectedToolUse {
			t.Fatalf("tool item accepted: %#v %v", got, err)
		}
	})
}

func TestLiveReceiptRejectsTamper(t *testing.T) {
	r := LiveReceipt{
		SchemaVersion:     1,
		CLI:               "codex-cli",
		Version:           QualifiedCodexVersion,
		BinaryDigest:      "sha256:" + strings.Repeat("a", 64),
		CredentialMode:    "workload_identity",
		FederationRuleID:  "idpm_test",
		Model:             "gpt-5.6-sol",
		PromptDigest:      "sha256:" + strings.Repeat("b", 64),
		ThreadID:          "thread",
		TurnID:            "turn",
		TurnStatus:        "completed",
		Output:            "engineering-platform live qualification",
		OutputDigest:      canonicalDigestText("engineering-platform live qualification"),
		ApprovalRequests:  0,
		UnexpectedToolUse: false,
	}
	// The fixed qualification prompt has a deterministic digest.
	r.PromptDigest = canonicalDigestText(LiveProbePrompt)
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
	r.Output = "changed"
	if err := r.Validate(); err == nil {
		t.Fatal("changed output accepted")
	}
}

func canonicalDigestText(value string) string {
	return canonical.BytesDigest([]byte(value))
}
