package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/jiying2007/engineering-platform/internal/relayqualification"
)

func relayPrequalificationPack(args []string) error {
	fs := flag.NewFlagSet("relay-prequalification-pack", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	codexProvider := fs.String("codex-provider", "", "non-reserved Codex custom provider id")
	baseURL := fs.String("base-url", "", "exact relay base URL; HTTPS by default")
	allowPrivateHTTP := fs.Bool("allow-insecure-private-http", false, "allow HTTP only for a literal private/loopback IP relay endpoint")
	authEnvKey := fs.String("auth-env-key", "", "dedicated relay token environment variable name")
	authCommand := fs.String("auth-command", "", "absolute command path that returns a relay bearer token")
	authCommandTimeoutMS := fs.Int("auth-command-timeout-ms", 5000, "bounded token-helper timeout in milliseconds")
	model := fs.String("model", "", "exact requested model")
	streamIdleMS := fs.Int("stream-idle-timeout-ms", 30000, "bounded stream idle timeout in milliseconds")
	gatewayPolicy := fs.String("gateway-policy", "", "gateway protocol/security policy file")
	privacyPolicy := fs.String("privacy-policy", "", "privacy/retention policy file")
	modelMapping := fs.String("model-mapping", "", "requested/effective model mapping policy file")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 {
		return fmt.Errorf("invalid relay-prequalification-pack arguments")
	}
	if *codexProvider == "" || *baseURL == "" || *model == "" ||
		*gatewayPolicy == "" || *privacyPolicy == "" || *modelMapping == "" {
		return fmt.Errorf("relay provider, base URL, model and all three policy files are required")
	}
	if (*authEnvKey == "") == (*authCommand == "") {
		return fmt.Errorf("select exactly one of --auth-env-key or --auth-command")
	}

	gatewayBytes, err := readRelayPolicyFile(*gatewayPolicy)
	if err != nil {
		return fmt.Errorf("gateway policy: %w", err)
	}
	privacyBytes, err := readRelayPolicyFile(*privacyPolicy)
	if err != nil {
		return fmt.Errorf("privacy policy: %w", err)
	}
	mappingBytes, err := readRelayPolicyFile(*modelMapping)
	if err != nil {
		return fmt.Errorf("model mapping: %w", err)
	}

	input := relayqualification.BuildInput{
		CodexProviderID:          *codexProvider,
		BaseURL:                  *baseURL,
		AllowInsecurePrivateHTTP: *allowPrivateHTTP,
		RequestedModel:           *model,
		StreamIdleTimeoutMS:      *streamIdleMS,
		GatewayPolicy:            gatewayBytes,
		PrivacyPolicy:            privacyBytes,
		ModelMapping:             mappingBytes,
	}
	if *authEnvKey != "" {
		input.AuthMode = relayqualification.AuthEnvKey
		input.EnvKey = *authEnvKey
	} else {
		input.AuthMode = relayqualification.AuthCommandToken
		input.AuthCommand = *authCommand
		input.AuthCommandTimeoutMS = *authCommandTimeoutMS
		input.AuthCommandRefreshIntervalMS = 0
	}
	pack, err := relayqualification.BuildPack(input)
	if err != nil {
		return err
	}
	printJSON(pack)
	return nil
}

func readRelayPolicyFile(path string) ([]byte, error) {
	before, err := os.Lstat(path)
	if err != nil || !before.Mode().IsRegular() || before.Size() <= 0 || before.Size() > 1<<20 {
		return nil, fmt.Errorf("bounded non-empty regular file required")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(before, opened) {
		return nil, fmt.Errorf("policy file changed before read")
	}
	data, err := io.ReadAll(io.LimitReader(file, (1<<20)+1))
	if err != nil || len(data) == 0 || len(data) > 1<<20 {
		return nil, fmt.Errorf("policy file read failed or exceeds bound")
	}
	after, err := os.Lstat(path)
	if err != nil || !os.SameFile(before, after) || after.Size() != before.Size() {
		return nil, fmt.Errorf("policy file changed during read")
	}
	return data, nil
}
