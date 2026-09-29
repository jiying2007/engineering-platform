package relayqualification

import (
	"strings"
	"testing"

	"github.com/jiying2007/engineering-platform/internal/canonical"
)

func TestRenderEnvKeyCodexConfigIsExactAndBound(t *testing.T) {
	contract := validContract()
	got, err := contract.RenderCodexConfig()
	if err != nil {
		t.Fatal(err)
	}
	want := `model = "relay-gpt-5.6"
model_provider = "company-relay-v1"

[model_providers."company-relay-v1"]
name = "Engineering Platform company relay"
base_url = "https://relay.example.invalid/v1"
wire_api = "responses"
requires_openai_auth = false
request_max_retries = 0
stream_max_retries = 0
stream_idle_timeout_ms = 30000
supports_websockets = false
supports_standalone_web_search = false
env_key = "COMPANY_RELAY_TOKEN"
`
	if string(got) != want {
		t.Fatalf("unexpected Codex config:\n%s\nwant:\n%s", got, want)
	}
	configDigest, err := contract.CodexConfigDigest()
	if err != nil {
		t.Fatal(err)
	}
	if configDigest != canonical.BytesDigest([]byte(want)) {
		t.Fatalf("config digest mismatch: %s", configDigest)
	}
	assessment, err := Evaluate(contract)
	if err != nil {
		t.Fatal(err)
	}
	if assessment.CodexConfigDigest != configDigest ||
		assessment.ContractDigest == "" ||
		assessment.ProviderConfigDigest == "" ||
		assessment.RendererContractVersion != RendererContractVersion {
		t.Fatalf("assessment did not bind exact renderer output: %#v", assessment)
	}
}

func TestProviderConfigDigestChangesWhenRenderedConfigChanges(t *testing.T) {
	first := validContract()
	second := validContract()
	second.StreamIdleTimeoutMS = 31000

	a, err := first.ProviderConfigDigest()
	if err != nil {
		t.Fatal(err)
	}
	b, err := second.ProviderConfigDigest()
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatal("provider config digest ignored rendered Codex config drift")
	}
}

func TestRenderCommandTokenConfigBindsCommandPolicy(t *testing.T) {
	contract := validContract()
	contract.AuthMode = AuthCommandToken
	contract.EnvKey = ""
	contract.AuthCommand = "/usr/local/bin/company-relay-token"
	contract.AuthCommandTimeoutMS = 5000
	contract.AuthCommandRefreshIntervalMS = 0
	got, err := contract.RenderCodexConfig()
	if err != nil {
		t.Fatal(err)
	}
	text := string(got)
	for _, want := range []string{
		`[model_providers."company-relay-v1".auth]`,
		`command = "/usr/local/bin/company-relay-token"`,
		`timeout_ms = 5000`,
		`refresh_interval_ms = 0`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("rendered config missing %q: %s", want, text)
		}
	}
	if strings.Contains(text, "bearer_token") || strings.Contains(text, "api_key") {
		t.Fatalf("rendered config contains credential material field: %s", text)
	}

	other := contract
	other.AuthCommandTimeoutMS = 6000
	a, _ := contract.ProviderConfigDigest()
	b, _ := other.ProviderConfigDigest()
	if a == b {
		t.Fatal("provider config digest ignored auth command timeout")
	}
}
