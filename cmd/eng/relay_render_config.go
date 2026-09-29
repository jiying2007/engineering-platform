package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/jiying2007/engineering-platform/internal/relayqualification"
)

type relayRenderedConfigOutput struct {
	ProviderConfigDigest string `json:"provider_config_digest"`
	CodexConfigDigest    string `json:"codex_config_digest"`
	OutputPath           string `json:"output_path"`
}

func relayRenderCodexConfig(args []string) error {
	fs := flag.NewFlagSet("relay-render-codex-config", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	contractFile := fs.String("contract", "", "relay provider prequalification contract JSON")
	outPath := fs.String("out", "", "new owner-private Codex user-level config file")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 || *contractFile == "" || *outPath == "" {
		return fmt.Errorf("usage: eng relay-render-codex-config --contract CONTRACT.json --out CONFIG.toml")
	}
	contract, err := readRelayContract(*contractFile)
	if err != nil {
		return err
	}
	config, err := contract.RenderCodexConfig()
	if err != nil {
		return err
	}
	providerDigest, err := contract.Digest()
	if err != nil {
		return err
	}
	configDigest, err := contract.CodexConfigDigest()
	if err != nil {
		return err
	}
	file, err := os.OpenFile(*outPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create relay Codex config: %w", err)
	}
	n, writeErr := file.Write(config)
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil || n != len(config) {
		_ = os.Remove(*outPath)
		return fmt.Errorf("write relay Codex config failed")
	}
	info, err := os.Lstat(*outPath)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 || info.Size() != int64(len(config)) {
		_ = os.Remove(*outPath)
		return fmt.Errorf("rendered relay Codex config is not an exact owner-private regular file")
	}
	printJSON(relayRenderedConfigOutput{
		ProviderConfigDigest: providerDigest,
		CodexConfigDigest:    configDigest,
		OutputPath:           *outPath,
	})
	return nil
}
