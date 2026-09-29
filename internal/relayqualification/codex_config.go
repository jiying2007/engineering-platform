package relayqualification

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jiying2007/engineering-platform/internal/canonical"
)

func (c Contract) RenderCodexConfig() ([]byte, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "model = %s\n", tomlString(c.RequestedModel))
	fmt.Fprintf(&b, "model_provider = %s\n\n", tomlString(c.CodexProviderID))
	fmt.Fprintf(&b, "[model_providers.%s]\n", tomlString(c.CodexProviderID))
	fmt.Fprintln(&b, "name = \"Engineering Platform company relay\"")
	fmt.Fprintf(&b, "base_url = %s\n", tomlString(c.BaseURL))
	fmt.Fprintf(&b, "wire_api = %s\n", tomlString(c.WireAPI))
	fmt.Fprintln(&b, "requires_openai_auth = false")
	fmt.Fprintf(&b, "request_max_retries = %d\n", c.RequestMaxRetries)
	fmt.Fprintf(&b, "stream_max_retries = %d\n", c.StreamMaxRetries)
	fmt.Fprintf(&b, "stream_idle_timeout_ms = %d\n", c.StreamIdleTimeoutMS)
	fmt.Fprintf(&b, "supports_websockets = %t\n", c.SupportsWebSockets)
	fmt.Fprintf(&b, "supports_standalone_web_search = %t\n", c.SupportsStandaloneWebSearch)
	switch c.AuthMode {
	case AuthEnvKey:
		fmt.Fprintf(&b, "env_key = %s\n", tomlString(c.EnvKey))
	case AuthCommandToken:
		fmt.Fprintf(&b, "\n[model_providers.%s.auth]\n", tomlString(c.CodexProviderID))
		fmt.Fprintf(&b, "command = %s\n", tomlString(c.AuthCommand))
		fmt.Fprintf(&b, "timeout_ms = %d\n", c.AuthCommandTimeoutMS)
		fmt.Fprintf(&b, "refresh_interval_ms = %d\n", c.AuthCommandRefreshIntervalMS)
	default:
		return nil, fmt.Errorf("unsupported relay auth mode")
	}
	return []byte(b.String()), nil
}

func (c Contract) CodexConfigDigest() (string, error) {
	data, err := c.RenderCodexConfig()
	if err != nil {
		return "", err
	}
	return canonical.BytesDigest(data), nil
}

func tomlString(value string) string {
	return strconv.Quote(value)
}
