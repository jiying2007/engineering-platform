package relayqualification

import (
	"fmt"
	"net"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/jiying2007/engineering-platform/internal/canonical"
)

const (
	SchemaVersion            = 2
	RendererContractVersion  = 1
	ProviderCompanyRelay     = "company-relay"
	WireAPIResponses         = "responses"
	AuthEnvKey               = "env-key"
	AuthCommandToken         = "command-token"
	StageRepositoryPrequalification = "repository-prequalification"
	NextGateLiveReadOnlyNoTool      = "live-read-only-no-tool-provider-qualification"
)

var providerIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)
var envKeyPattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,63}$`)

type Contract struct {
	SchemaVersion                    int    `json:"schema_version"`
	ProviderID                       string `json:"provider_id"`
	CodexProviderID                  string `json:"codex_provider_id"`
	BaseURL                          string `json:"base_url"`
	AllowInsecurePrivateHTTP         bool   `json:"allow_insecure_private_http"`
	WireAPI                          string `json:"wire_api"`
	AuthMode                         string `json:"auth_mode"`
	EnvKey                           string `json:"env_key,omitempty"`
	AuthCommand                      string `json:"auth_command,omitempty"`
	AuthCommandTimeoutMS             int    `json:"auth_command_timeout_ms,omitempty"`
	AuthCommandRefreshIntervalMS     int    `json:"auth_command_refresh_interval_ms,omitempty"`
	RequestedModel                   string `json:"requested_model"`
	RequestMaxRetries                int    `json:"request_max_retries"`
	StreamMaxRetries                 int    `json:"stream_max_retries"`
	StreamIdleTimeoutMS              int    `json:"stream_idle_timeout_ms"`
	SupportsWebSockets               bool   `json:"supports_websockets"`
	SupportsStandaloneWebSearch      bool   `json:"supports_standalone_web_search"`
	GatewayPolicyDigest              string `json:"gateway_policy_digest"`
	PrivacyPolicyDigest              string `json:"privacy_policy_digest"`
	ModelMappingDigest               string `json:"model_mapping_digest"`
}

type Assessment struct {
	SchemaVersion            int    `json:"schema_version"`
	Stage                    string `json:"stage"`
	ContractDigest           string `json:"contract_digest"`
	RendererContractVersion  int    `json:"renderer_contract_version"`
	CodexConfigDigest        string `json:"codex_config_digest"`
	ProviderConfigDigest     string `json:"provider_config_digest"`
	ProviderID               string `json:"provider_id"`
	CodexProviderID          string `json:"codex_provider_id"`
	RequestedModel           string `json:"requested_model"`
	AccountVerified          bool   `json:"account_verified"`
	LiveModelTurnExecuted    bool   `json:"live_model_turn_executed"`
	ProviderAdmitted         bool   `json:"provider_admitted"`
	NextGate                 string `json:"next_gate"`
}

type providerConfigIdentity struct {
	SchemaVersion           int    `json:"schema_version"`
	ContractDigest          string `json:"contract_digest"`
	RendererContractVersion int    `json:"renderer_contract_version"`
	CodexConfigDigest       string `json:"codex_config_digest"`
}

func (c Contract) Validate() error {
	if c.SchemaVersion != SchemaVersion || c.ProviderID != ProviderCompanyRelay {
		return fmt.Errorf("relay prequalification requires schema v2 and provider company-relay")
	}
	if !providerIDPattern.MatchString(c.CodexProviderID) || reservedCodexProviderID(c.CodexProviderID) {
		return fmt.Errorf("codex_provider_id must be a non-reserved custom provider id")
	}
	if err := validateBaseURL(c.BaseURL, c.AllowInsecurePrivateHTTP); err != nil {
		return err
	}
	if c.WireAPI != WireAPIResponses {
		return fmt.Errorf("relay prequalification requires responses wire API")
	}
	switch c.AuthMode {
	case AuthEnvKey:
		if !envKeyPattern.MatchString(c.EnvKey) || c.AuthCommand != "" ||
			c.AuthCommandTimeoutMS != 0 || c.AuthCommandRefreshIntervalMS != 0 {
			return fmt.Errorf("env-key auth requires exactly one bounded env_key and no command settings")
		}
		if strings.HasPrefix(c.EnvKey, "OPENAI_") || strings.HasPrefix(c.EnvKey, "CODEX_") {
			return fmt.Errorf("relay credential locator must not reuse OpenAI/Codex credential variables")
		}
	case AuthCommandToken:
		if c.EnvKey != "" || !validAbsoluteCommand(c.AuthCommand) {
			return fmt.Errorf("command-token auth requires exactly one absolute auth_command")
		}
		if c.AuthCommandTimeoutMS < 1000 || c.AuthCommandTimeoutMS > 30000 {
			return fmt.Errorf("auth command timeout must be between 1000 and 30000 ms")
		}
		if c.AuthCommandRefreshIntervalMS != 0 {
			return fmt.Errorf("initial relay qualification requires auth command refresh_interval_ms=0")
		}
	default:
		return fmt.Errorf("unsupported relay auth mode")
	}
	if !validText(c.RequestedModel, 128) {
		return fmt.Errorf("bounded exact requested_model required")
	}
	if c.RequestMaxRetries != 0 || c.StreamMaxRetries != 0 {
		return fmt.Errorf("initial relay qualification forbids automatic request or stream retries")
	}
	if c.StreamIdleTimeoutMS < 1000 || c.StreamIdleTimeoutMS > 300000 {
		return fmt.Errorf("stream idle timeout must be between 1000 and 300000 ms")
	}
	if c.SupportsWebSockets || c.SupportsStandaloneWebSearch {
		return fmt.Errorf("initial relay qualification forbids websocket and standalone web-search surfaces")
	}
	for name, digest := range map[string]string{
		"gateway_policy_digest": c.GatewayPolicyDigest,
		"privacy_policy_digest": c.PrivacyPolicyDigest,
		"model_mapping_digest":  c.ModelMappingDigest,
	} {
		if !canonical.ValidDigest(digest) {
			return fmt.Errorf("%s must be an exact sha256 digest", name)
		}
	}
	return nil
}

func (c Contract) Digest() (string, error) {
	if err := c.Validate(); err != nil {
		return "", err
	}
	return canonical.Digest(c)
}

func (c Contract) ProviderConfigDigest() (string, error) {
	contractDigest, err := c.Digest()
	if err != nil {
		return "", err
	}
	configDigest, err := c.CodexConfigDigest()
	if err != nil {
		return "", err
	}
	return canonical.Digest(providerConfigIdentity{
		SchemaVersion:           SchemaVersion,
		ContractDigest:          contractDigest,
		RendererContractVersion: RendererContractVersion,
		CodexConfigDigest:       configDigest,
	})
}

func Evaluate(c Contract) (Assessment, error) {
	contractDigest, err := c.Digest()
	if err != nil {
		return Assessment{}, err
	}
	configDigest, err := c.CodexConfigDigest()
	if err != nil {
		return Assessment{}, err
	}
	providerDigest, err := c.ProviderConfigDigest()
	if err != nil {
		return Assessment{}, err
	}
	return Assessment{
		SchemaVersion:           SchemaVersion,
		Stage:                   StageRepositoryPrequalification,
		ContractDigest:          contractDigest,
		RendererContractVersion: RendererContractVersion,
		CodexConfigDigest:       configDigest,
		ProviderConfigDigest:    providerDigest,
		ProviderID:              c.ProviderID,
		CodexProviderID:         c.CodexProviderID,
		RequestedModel:          c.RequestedModel,
		AccountVerified:         false,
		LiveModelTurnExecuted:   false,
		ProviderAdmitted:        false,
		NextGate:                NextGateLiveReadOnlyNoTool,
	}, nil
}

func reservedCodexProviderID(id string) bool {
	switch id {
	case "openai", "ollama", "lmstudio", "amazon-bedrock":
		return true
	default:
		return false
	}
}

func validateBaseURL(value string, allowInsecurePrivateHTTP bool) error {
	if !validText(value, 2048) {
		return fmt.Errorf("bounded exact relay base_url required")
	}
	u, err := url.Parse(value)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" {
		return fmt.Errorf("relay base_url must be an absolute provider URL without credentials, query, fragment or opaque form")
	}
	switch u.Scheme {
	case "https":
		if allowInsecurePrivateHTTP {
			return fmt.Errorf("allow_insecure_private_http is valid only for an HTTP private-IP endpoint")
		}
	case "http":
		if !allowInsecurePrivateHTTP {
			return fmt.Errorf("HTTP relay endpoint requires explicit allow_insecure_private_http")
		}
		ip := net.ParseIP(u.Hostname())
		if ip == nil || (!ip.IsPrivate() && !ip.IsLoopback()) {
			return fmt.Errorf("insecure HTTP relay endpoint must use a literal private or loopback IP")
		}
	default:
		return fmt.Errorf("relay base_url must use HTTPS or explicitly allowed private HTTP")
	}
	if u.Path != "" && strings.Contains(u.Path, "..") {
		return fmt.Errorf("relay base_url path must not contain traversal segments")
	}
	return nil
}

func validAbsoluteCommand(value string) bool {
	return validText(value, 512) && filepath.IsAbs(value) && filepath.Clean(value) == value
}

func validText(value string, max int) bool {
	if value == "" || len(value) > max || strings.TrimSpace(value) != value {
		return false
	}
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}
