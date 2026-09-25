package codexapp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/canonical"
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

func liveEventAdapter(events ...Event) *Adapter {
	queue := make(chan Event, len(events))
	for _, event := range events {
		queue <- event
	}
	client := &Client{events: queue, done: make(chan struct{})}
	return &Adapter{client: client, thread: "thread-live", turn: "turn-live"}
}

func notification(method, params string) Event {
	return Event{Kind: Notification, Message: Message{Method: method, Params: json.RawMessage(params)}}
}

func TestObserveTurnRetainsCompletedAgentMessage(t *testing.T) {
	adapter := liveEventAdapter(
		notification("item/completed", `{"threadId":"thread-live","turnId":"turn-live","item":{"type":"reasoning","id":"reasoning-1"}}`),
		notification("item/completed", `{"threadId":"thread-live","turnId":"turn-live","item":{"type":"agentMessage","id":"message-1","text":"engineering-platform live qualification"}}`),
		notification("turn/completed", `{"threadId":"thread-live","turn":{"id":"turn-live","status":"completed"}}`),
	)
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
		adapter := liveEventAdapter(
			notification("item/completed", `{"threadId":"thread-live","turnId":"turn-live","item":{"type":"commandExecution","id":"cmd-1"}}`),
		)
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
		SchemaVersion:              1,
		CLI:                        "codex-cli",
		Version:                    QualifiedCodexVersion,
		BinaryDigest:               "sha256:" + strings.Repeat("a", 64),
		CredentialSafeConfigDigest: canonical.BytesDigest([]byte(credentialSafeConfig)),
		CredentialMode:             "workload_identity",
		FederationRuleID:           "rule-test",
		Model:                      "gpt-5.6-sol",
		PromptDigest:               "sha256:" + strings.Repeat("b", 64),
		ThreadID:                   "thread",
		TurnID:                     "turn",
		TurnStatus:                 "completed",
		Output:                     "engineering-platform live qualification",
		OutputDigest:               canonicalDigestText("engineering-platform live qualification"),
		ApprovalRequests:           0,
		UnexpectedToolUse:          false,
		AssertionRemovedBeforeTurn: true,
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

func TestWarmWorkloadIdentityUsesAuthenticatedRateLimitReadBeforeThread(t *testing.T) {
	client, peer := pair(t)
	adapter, err := NewAdapter(client, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	adapter.initialized = true
	server := make(chan error, 1)
	go func() {
		var request Message
		if err := json.NewDecoder(peer).Decode(&request); err != nil {
			server <- err
			return
		}
		if request.Method != "account/rateLimits/read" {
			server <- fmt.Errorf("unexpected prewarm method %q", request.Method)
			return
		}
		var params struct {
			SupportsLunaReserve       bool `json:"supportsLunaReserve"`
			ExcludeResetCreditDetails bool `json:"excludeResetCreditDetails"`
		}
		if err := json.Unmarshal(request.Params, &params); err != nil {
			server <- err
			return
		}
		if params.SupportsLunaReserve || !params.ExcludeResetCreditDetails {
			server <- fmt.Errorf("unsafe rate-limit prewarm params: %#v", params)
			return
		}
		_, err := io.WriteString(peer, `{"id":`+string(request.ID)+`,"result":{"rateLimits":{"primary":null}}}`+"\n")
		server <- err
	}()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := adapter.WarmWorkloadIdentity(ctx); err != nil {
		t.Fatal(err)
	}
	if adapter.thread != "" || adapter.turn != "" {
		t.Fatal("prewarm started model lifecycle")
	}
	if err := <-server; err != nil {
		t.Fatal(err)
	}
}

func TestLiveReceiptRequiresAssertionRemovalFence(t *testing.T) {
	r := LiveReceipt{
		SchemaVersion:              1,
		CLI:                        "codex-cli",
		Version:                    QualifiedCodexVersion,
		BinaryDigest:               "sha256:" + strings.Repeat("a", 64),
		CredentialSafeConfigDigest: canonical.BytesDigest([]byte(credentialSafeConfig)),
		CredentialMode:             "workload_identity",
		FederationRuleID:           "rule-test",
		Model:                      "gpt-5.6-sol",
		PromptDigest:               canonicalDigestText(LiveProbePrompt),
		ThreadID:                   "thread",
		TurnID:                     "turn",
		TurnStatus:                 "completed",
		Output:                     LiveProbeExpected,
		OutputDigest:               canonicalDigestText(LiveProbeExpected),
	}
	if err := r.Validate(); err == nil {
		t.Fatal("receipt without assertion-removal fence accepted")
	}
	r.AssertionRemovedBeforeTurn = true
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
}

func canonicalDigestText(value string) string {
	return canonical.BytesDigest([]byte(value))
}
