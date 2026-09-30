package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/jiying2007/engineering-platform/internal/relayqualification"
)

type relayQualificationKitOutput struct {
	SchemaVersion   int    `json:"schema_version"`
	ManifestDigest  string `json:"manifest_digest"`
	OutputDirectory string `json:"output_directory"`
}

func relayQualificationKit(args []string) error {
	fs := flag.NewFlagSet("relay-qualification-kit", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	contractFile := fs.String("contract", "", "relay provider prequalification v2 contract JSON")
	executable := fs.String("codex", "", "absolute native Codex executable")
	qualificationFile := fs.String("qualification", "", "compatibility qualification receipt JSON")
	outDir := fs.String("out-dir", "", "new owner-private output directory")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 ||
		*contractFile == "" || *executable == "" || *qualificationFile == "" || *outDir == "" {
		return fmt.Errorf("usage: eng relay-qualification-kit --contract CONTRACT.json --codex ABSOLUTE_CODEX --qualification RECEIPT.json --out-dir NEW_DIR")
	}
	contract, err := readRelayContract(*contractFile)
	if err != nil {
		return err
	}
	qualification, err := readRelayQualification(*qualificationFile)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	kit, err := relayqualification.BuildQualificationKit(ctx, contract, *executable, qualification)
	if err != nil {
		return err
	}
	if !filepath.IsAbs(*outDir) {
		return fmt.Errorf("absolute output directory required")
	}
	if err := os.Mkdir(*outDir, 0o700); err != nil {
		return fmt.Errorf("create qualification kit directory: %w", err)
	}
	keep := false
	defer func() {
		if !keep {
			_ = os.RemoveAll(*outDir)
		}
	}()
	files := []struct {
		name string
		data []byte
	}{
		{relayqualification.KitContractFile, kit.ContractJSON},
		{relayqualification.KitCodexConfigFile, kit.CodexConfig},
		{relayqualification.KitLiveManifestFile, kit.LiveManifestJSON},
		{relayqualification.KitRuntimeHandoffFile, kit.RuntimeHandoffJSON},
		{relayqualification.KitQualificationFile, kit.QualificationJSON},
		{relayqualification.KitManifestFile, kit.ManifestJSON},
		{relayqualification.KitChecksumsFile, kit.Checksums},
	}
	for _, item := range files {
		if err := writeRelayKitFile(filepath.Join(*outDir, item.name), item.data); err != nil {
			return err
		}
	}
	info, err := os.Lstat(*outDir)
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0o700 {
		return fmt.Errorf("qualification kit directory is not owner-private")
	}
	keep = true
	printJSON(relayQualificationKitOutput{
		SchemaVersion:   relayqualification.QualificationKitSchemaVersion,
		ManifestDigest:  kit.ManifestDigest,
		OutputDirectory: *outDir,
	})
	return nil
}

func writeRelayKitFile(path string, data []byte) error {
	if len(data) == 0 || len(data) > 2<<20 {
		return fmt.Errorf("qualification kit file is empty or exceeds bound")
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create qualification kit file: %w", err)
	}
	n, writeErr := file.Write(data)
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil || n != len(data) {
		_ = os.Remove(path)
		return fmt.Errorf("write qualification kit file failed")
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 || info.Size() != int64(len(data)) {
		_ = os.Remove(path)
		return fmt.Errorf("qualification kit file is not exact owner-private regular file")
	}
	return nil
}
