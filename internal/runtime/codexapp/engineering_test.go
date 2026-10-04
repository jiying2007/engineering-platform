package codexapp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"github.com/jiying2007/engineering-platform/internal/testsupport"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/provideridentity"
)

func unsignedTimingAssertion(t *testing.T, issuedAt, expiresAt time.Time) []byte {
	t.Helper()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	payload, err := json.Marshal(map[string]int64{
		"iat": issuedAt.Unix(),
		"exp": expiresAt.Unix(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return []byte(header + "." + base64.RawURLEncoding.EncodeToString(payload) + ".fixture")
}

func TestBoundedEngineeringWIFTimeoutUsesActualAssertionLifetime(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	fresh := unsignedTimingAssertion(t, now, now.Add(5*time.Minute))
	got, err := boundedEngineeringWIFTimeout(fresh, now)
	if err != nil || got != 4*time.Minute {
		t.Fatalf("fresh GitHub-like assertion timeout=%s err=%v", got, err)
	}

	aged := unsignedTimingAssertion(t, now.Add(-2*time.Minute), now.Add(3*time.Minute))
	got, err = boundedEngineeringWIFTimeout(aged, now)
	if err != nil || got != 150*time.Second {
		t.Fatalf("aged assertion timeout=%s err=%v", got, err)
	}
}

func TestBoundedEngineeringWIFTimeoutRejectsUnsafeTiming(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	for name, assertion := range map[string][]byte{
		"too-long-upstream":    unsignedTimingAssertion(t, now, now.Add(601*time.Second)),
		"too-little-remaining": unsignedTimingAssertion(t, now.Add(-3*time.Minute), now.Add(100*time.Second)),
		"invalid":              []byte("not-a-jwt"),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := boundedEngineeringWIFTimeout(assertion, now); err == nil {
				t.Fatal("unsafe assertion timing accepted")
			}
		})
	}
}

func TestWIFEngineeringProviderWritesFixedNoNetworkProfile(t *testing.T) {
	base, spec := launchFixture(t)
	digest, err := executableDigest(base.executable)
	if err != nil {
		t.Fatal(err)
	}
	provider, err := NewPinnedWIFEngineeringProvider(base.executable, digest)
	if err != nil {
		t.Fatal(err)
	}
	spec.Env = append(spec.Env, workloadIdentityEnv(t, spec)...)
	if _, err := provider.Command(context.Background(), spec); err != nil {
		t.Fatal(err)
	}
	home := strings.TrimPrefix(spec.Env[0], "HOME=")
	data, err := os.ReadFile(filepath.Join(home, ".codex", "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != engineeringConfig || EngineeringConfigDigest() != canonicalDigestText(engineeringConfig) {
		t.Fatalf("engineering config drift: %q", data)
	}
	for _, required := range []string{`sandbox_mode = "workspace-write"`, "network_access = false", "shell_tool = true", `web_search = "disabled"`, `inherit = "none"`, "allow_login_shell = false"} {
		if !strings.Contains(string(data), required) {
			t.Fatal("engineering profile missing", required)
		}
	}
	for _, forbidden := range []string{"OPENAI_", "identity-token", "network_access = true"} {
		if strings.Contains(string(data), forbidden) {
			t.Fatal("forbidden engineering setting", forbidden)
		}
	}
}

func TestStartEngineeringThreadUsesStableWorkspaceWriteWire(t *testing.T) {
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
		if request.Method != "thread/start" {
			server <- ErrProtocol
			return
		}
		var params struct {
			CWD       string `json:"cwd"`
			Model     string `json:"model"`
			Policy    string `json:"approvalPolicy"`
			Sandbox   string `json:"sandbox"`
			Ephemeral bool   `json:"ephemeral"`
			Config    any    `json:"config,omitempty"`
		}
		if json.Unmarshal(request.Params, &params) != nil || params.Model != "gpt-test" || params.Policy != "never" || params.Sandbox != "workspace-write" || !params.Ephemeral || params.Config != nil {
			server <- ErrProtocol
			return
		}
		_, err := io.WriteString(peer, `{"id":`+string(request.ID)+`,"result":{"thread":{"id":"thread-engineering"}}}`+"\n")
		server <- err
	}()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	id, err := adapter.StartEngineeringThread(ctx, "gpt-test")
	if err != nil || id != "thread-engineering" {
		t.Fatal(id, err)
	}
	if err := <-server; err != nil {
		t.Fatal(err)
	}
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
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "completed" || got.Output != "implemented and tested" || got.CommandCount != 2 || got.FailedCommands != 1 || got.FileChangeCount != 1 || got.ApprovalRequests != 0 {
		t.Fatalf("unexpected observation: %#v", got)
	}
}

func TestObserveEngineeringTurnRejectsExternalTools(t *testing.T) {
	for _, itemType := range []string{"webSearch", "mcpToolCall", "dynamicToolCall"} {
		t.Run(itemType, func(t *testing.T) {
			params := `{"threadId":"thread-live","turnId":"turn-live","item":{"type":"` + itemType + `","id":"item-1"}}`
			adapter := liveEventAdapter(notification("item/completed", params))
			if _, err := ObserveEngineeringTurn(context.Background(), adapter, "thread-live", "turn-live"); err == nil {
				t.Fatal("disallowed tool accepted")
			}
		})
	}
}

func TestEngineeringWIFTurnDeletesAssertionBeforeModelReachableWork(t *testing.T) {
	testsupport.RequireProcessNamespaces(t)
	root := t.TempDir()
	work := filepath.Join(root, "work")
	home := filepath.Join(root, "home")
	identity := filepath.Join(root, "identity")
	for _, dir := range []string{work, home, identity} {
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	token := filepath.Join(identity, "token")
	now := time.Now()
	if err := os.WriteFile(token, unsignedTimingAssertion(t, now, now.Add(5*time.Minute)), 0o600); err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := executableDigest(executable)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	receipt, err := EngineeringWIFTurn(
		ctx, executable, testCodexVersion, digest, "sha256:"+strings.Repeat("a", 64), work, home,
		"rule-engineering-test", token, `{"run_id":"fixture"}`,
		"gpt-test", "Modify the fixture workspace.", nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !receipt.AssertionRemovedBeforeTurn || receipt.CommandCount != 1 ||
		receipt.FileChangeCount != 1 || receipt.Output != "fixture engineering change complete" {
		t.Fatalf("unexpected engineering receipt: %#v", receipt)
	}
	if _, err := os.Stat(token); !os.IsNotExist(err) {
		t.Fatal("WIF assertion still exists after engineering turn")
	}
	data, err := os.ReadFile(filepath.Join(work, "engineered.txt"))
	if err != nil || string(data) != "generated by fixture\n" {
		t.Fatalf("fixture did not produce workspace change: %q %v", data, err)
	}
}

func TestEngineeringSavedLoginTurnDeletesBootstrapBeforeModelReachableWork(t *testing.T) {
	testsupport.RequireProcessNamespaces(t)
	root := t.TempDir()
	work := filepath.Join(root, "work")
	home := filepath.Join(root, "home")
	loginDir := filepath.Join(root, "saved-login")
	for _, dir := range []string{work, home, loginDir} {
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	login := filepath.Join(loginDir, "auth.json")
	if err := os.WriteFile(login, []byte(`{"tokens":{"access_token":"fixture","refresh_token":"fixture"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := executableDigest(executable)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	receipt, err := EngineeringSavedLoginTurn(
		ctx, executable, testCodexVersion, digest, "sha256:"+strings.Repeat("a", 64), work, home, login,
		"gpt-test", "Modify the fixture workspace.", nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Provider != provideridentity.OpenAIChatGPTTrustedSelfHosted() ||
		!receipt.CredentialBootstrapRemovedBeforeTurn ||
		receipt.AssertionRemovedBeforeTurn ||
		receipt.CommandCount != 1 || receipt.FileChangeCount != 1 ||
		receipt.Output != "fixture engineering change complete" {
		t.Fatalf("unexpected saved-login engineering receipt: %#v", receipt)
	}
	if _, err := os.Stat(filepath.Join(home, ".codex", "auth.json")); !os.IsNotExist(err) {
		t.Fatal("saved login bootstrap still exists after engineering turn")
	}
	if _, err := os.Stat(login); err != nil {
		t.Fatal("operator saved login source was modified")
	}
}
