package main

import (
	"context"
	"errors"
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
	if !filepath.IsAbs(*outDir) || filepath.Clean(*outDir) != *outDir {
		return fmt.Errorf("absolute normalized output directory required")
	}
	if err := os.Mkdir(*outDir, 0o700); err != nil {
		return fmt.Errorf("create qualification kit directory: %w", err)
	}
	// Do not delete by a mutable path on failure: an attacker could rename
	// our directory and replace that path with a different 0700 directory.
	// A failed kit remains an occupied private output for operator inspection.
	original, err := os.Lstat(*outDir)
	if err != nil || !original.IsDir() || original.Mode().Perm() != 0o700 {
		return fmt.Errorf("qualification kit output directory must be private")
	}
	root, err := os.OpenRoot(*outDir)
	if err != nil {
		return fmt.Errorf("open qualification kit output directory: %w", err)
	}
	defer root.Close()
	if err := relayKitOutputDirectoryUnchanged(root, *outDir, original); err != nil {
		return err
	}
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
		if err := relayKitOutputDirectoryUnchanged(root, *outDir, original); err != nil {
			return err
		}
		if err := writeRelayKitFile(root, item.name, item.data); err != nil {
			return err
		}
		if err := relayKitOutputDirectoryUnchanged(root, *outDir, original); err != nil {
			return err
		}
	}
	// A complete output must also satisfy the same strict byte/manifest and
	// directory checks as a later independent offline verification.
	written, err := readRelayQualificationKitDir(*outDir)
	if err != nil {
		return fmt.Errorf("qualification kit final readback: %w", err)
	}
	if _, err := relayqualification.VerifyQualificationKit(written); err != nil {
		return fmt.Errorf("qualification kit final content: %w", err)
	}
	if err := relayKitOutputDirectoryUnchanged(root, *outDir, original); err != nil {
		return err
	}
	return printJSON(relayQualificationKitOutput{
		SchemaVersion:   relayqualification.QualificationKitSchemaVersion,
		ManifestDigest:  kit.ManifestDigest,
		OutputDirectory: *outDir,
	})
}

// The original output inode, not a replacement directory at the same pathname,
// must own every created member and the final reported qualification kit.
func relayKitOutputDirectoryUnchanged(root *os.Root, path string, original os.FileInfo) error {
	if root == nil || original == nil {
		return fmt.Errorf("qualification kit output root required")
	}
	current, pathErr := os.Lstat(path)
	opened, openedErr := root.Stat(".")
	if pathErr != nil || openedErr != nil || !os.SameFile(original, current) || !os.SameFile(original, opened) ||
		!current.IsDir() || !opened.IsDir() || current.Mode().Perm() != 0o700 || opened.Mode().Perm() != 0o700 {
		return fmt.Errorf("qualification kit output directory identity changed")
	}
	return nil
}

// Write exclusively into the pinned directory. A failed write leaves an
// occupied partial output rather than removing any replacement filesystem path.
func writeRelayKitFile(root *os.Root, name string, data []byte) error {
	if root == nil || name == "" || name == "." || filepath.Base(name) != name {
		return fmt.Errorf("exact qualification kit member name required")
	}
	if len(data) == 0 || len(data) > 2<<20 {
		return fmt.Errorf("qualification kit file is empty or exceeds bound")
	}
	file, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create qualification kit member: %w", err)
	}
	opened, statErr := file.Stat()
	if statErr != nil || !opened.Mode().IsRegular() || opened.Mode().Perm() != 0o600 {
		return fmt.Errorf("opened qualification kit member is not an owner-private regular file: %w", errors.Join(statErr, file.Close()))
	}
	n, writeErr := file.Write(data)
	if n != len(data) && writeErr == nil {
		writeErr = io.ErrShortWrite
	}
	syncErr := file.Sync()
	closeErr := file.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil {
		return fmt.Errorf("qualification kit file write/sync failed: %w", errors.Join(writeErr, syncErr, closeErr))
	}
	actual, err := root.Lstat(name)
	if err != nil || !os.SameFile(opened, actual) || !actual.Mode().IsRegular() ||
		actual.Mode().Perm() != 0o600 || actual.Size() != int64(len(data)) {
		return fmt.Errorf("qualification kit member changed after write")
	}
	return nil
}
