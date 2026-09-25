package codexapp

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWIFEngineeringProviderWritesFixedNoNetworkProfile(t *testing.T) {
	base, spec := launchFixture(t)
	digest, err := executableDigest(base.executable)
	if err != nil { t.Fatal(err) }
	provider, err := NewPinnedWIFEngineeringProvider(base.executable, digest)
	if err != nil { t.Fatal(err) }
	spec.Env = append(spec.Env, workloadIdentityEnv(t, spec)...)
	if _, err := provider.Command(context.Background(), spec); err != nil { t.Fatal(err) }
	home := strings.TrimPrefix(spec.Env[0], "HOME=")
	data, err := os.ReadFile(filepath.Join(home, ".codex", "config.toml"))
	if err != nil { t.Fatal(err) }
	if string(data) != engineeringConfig || EngineeringConfigDigest() != canonicalDigestText(engineeringConfig) {
		t.Fatalf("engineering config drift: %q", data)
	}
	for _, required := range []string{`sandbox_mode = "workspace-write"`, "network_access = false", "shell_tool = true", `web_search = "disabled"`, `inherit = "none"`, "allow_login_shell = false"} {
		if !strings.Contains(string(data), required) { t.Fatal("engineering profile missing", required) }
	}
	for _, forbidden := range []string{"OPENAI_", "identity-token", "network_access = true"} {
		if strings.Contains(string(data), forbidden) { t.Fatal("forbidden engineering setting", forbidden) }
	}
}

func TestStartEngineeringThreadUsesStableWorkspaceWriteWire(t *testing.T) {
	client, peer := pair(t)
	adapter, err := NewAdapter(client, t.TempDir())
	if err != nil { t.Fatal(err) }
	adapter.initialized = true
	server := make(chan error, 1)
	go func() {
		var request Message
		if err := json.NewDecoder(peer).Decode(&request); err != nil { server <- err; return }
		if request.Method != "thread/start" { server <- ErrProtocol; return }
		var params struct {
			CWD string `json:"cwd"`
			Model string `json:"model"`
			Policy string `json:"approvalPolicy"`
			Sandbox string `json:"sandbox"`
			Ephemeral bool `json:"ephemeral"`
			Config any `json:"config,omitempty"`
		}
		if json.Unmarshal(request.Params, &params) != nil || params.Model != "gpt-test" || params.Policy != "never" || params.Sandbox != "workspace-write" || !params.Ephemeral || params.Config != nil { server <- ErrProtocol; return }
		_, err := io.WriteString(peer, `{"id":`+string(request.ID)+`,"result":{"thread":{"id":"thread-engineering"}}}`+"\n")
		server <- err
	}()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	id, err := adapter.StartEngineeringThread(ctx, "gpt-test")
	if err != nil || id != "thread-engineering" { t.Fatal(id, err) }
	if err := <-server; err != nil { t.Fatal(err) }
}

func TestObserveEngineeringTurnAcceptsLocalItems(t *testing.T) {
	adapter := liveEventAdapter(
		notification("item/completed", `{"threadId":"thread-live","turnId":"turn-live","item":{"type":"commandExecution","id":"cmd-1","status":"completed"}}`),
		notification("item/completed", `{"threadId":"thread-live","turnId":"turn-live","item":{"type":"commandExecution","id":"cmd-2","status":"failed"}}`),
		notification("item/completed", `{"threadId":"thread-live","turnId":"turn-live","item":{"type":"fileChange","id":"file-1"}}`),
		notification("item/completed", `{"threadId":"thread-live","turnId":"turn-live","item":{"type":"agentMessage","id":"message-1","text":"implemented and tested"}}`),
		notification("turn/completed", `{"threadId":"thread-live","turn":{"id":"turn-live","status":"completed"}}`),
	)
	got, err := ObserveEngineeringTurn(context.Background(), adapter, "thread-live", "turn-live")
	if err != nil { t.Fatal(err) }
	if got.Status != "completed" || got.Output != "implemented and tested" || got.CommandCount != 2 || got.FailedCommands != 1 || got.FileChangeCount != 1 || got.ApprovalRequests != 0 { t.Fatalf("unexpected observation: %#v", got) }
}

func TestObserveEngineeringTurnRejectsExternalTools(t *testing.T) {
	for _, itemType := range []string{"webSearch", "mcpToolCall", "dynamicToolCall"} {
		t.Run(itemType, func(t *testing.T) {
			params := `{"threadId":"thread-live","turnId":"turn-live","item":{"type":"`+itemType+`","id":"item-1"}}`
			adapter := liveEventAdapter(notification("item/completed", params))
			if _, err := ObserveEngineeringTurn(context.Background(), adapter, "thread-live", "turn-live"); err == nil { t.Fatal("disallowed tool accepted") }
		})
	}
}
