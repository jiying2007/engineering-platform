package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/jiying2007/engineering-platform/internal/relayqualification"
)

func relayVerifyQualificationKit(args []string) error {
	fs := flag.NewFlagSet("relay-verify-qualification-kit", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	dir := fs.String("dir", "", "absolute owner-private relay qualification kit directory")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 || *dir == "" {
		return fmt.Errorf("usage: eng relay-verify-qualification-kit --dir ABSOLUTE_KIT_DIR")
	}
	kit, err := readRelayQualificationKitDir(*dir)
	if err != nil {
		return err
	}
	verification, err := relayqualification.VerifyQualificationKit(kit)
	if err != nil {
		return err
	}
	return printJSON(verification)
}

func readRelayQualificationKitDir(dir string) (relayqualification.QualificationKit, error) {
	var kit relayqualification.QualificationKit
	if !filepath.IsAbs(dir) || filepath.Clean(dir) != dir {
		return kit, fmt.Errorf("absolute normalized qualification kit directory required")
	}
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil || resolved != dir {
		return kit, fmt.Errorf("qualification kit directory symlink alias is not allowed")
	}
	before, err := os.Lstat(dir)
	if err != nil || !before.IsDir() || before.Mode().Perm() != 0o700 {
		return kit, fmt.Errorf("qualification kit directory must be owner-private mode 0700")
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return kit, err
	}
	defer root.Close()
	opened, err := root.Stat(".")
	if err != nil || !os.SameFile(before, opened) || !opened.IsDir() || opened.Mode().Perm() != 0o700 {
		return kit, fmt.Errorf("qualification kit directory changed before read")
	}
	directoryFile, err := root.Open(".")
	if err != nil {
		return kit, err
	}
	entries, readErr := directoryFile.ReadDir(-1)
	closeErr := directoryFile.Close()
	if readErr != nil || closeErr != nil {
		return kit, fmt.Errorf("read qualification kit directory: %w", errors.Join(readErr, closeErr))
	}
	expected := map[string]bool{
		relayqualification.KitContractFile:       true,
		relayqualification.KitCodexConfigFile:    true,
		relayqualification.KitLiveManifestFile:   true,
		relayqualification.KitRuntimeHandoffFile: true,
		relayqualification.KitQualificationFile:  true,
		relayqualification.KitManifestFile:       true,
		relayqualification.KitChecksumsFile:      true,
	}
	if len(entries) != len(expected) {
		return kit, fmt.Errorf("qualification kit must contain exactly %d files", len(expected))
	}
	files := make(map[string][]byte, len(expected))
	for _, entry := range entries {
		if !expected[entry.Name()] || entry.Type()&os.ModeSymlink != 0 || entry.IsDir() {
			return kit, fmt.Errorf("unexpected qualification kit entry %q", entry.Name())
		}
		data, err := readRelayRootInput(root, entry.Name(), 2<<20, true)
		if err != nil {
			return kit, fmt.Errorf("%s: %w", entry.Name(), err)
		}
		files[entry.Name()] = data
	}
	after, err := os.Lstat(dir)
	rootAfter, rootErr := root.Stat(".")
	if err != nil || rootErr != nil || !os.SameFile(before, after) || !os.SameFile(before, rootAfter) ||
		!after.IsDir() || after.Mode().Perm() != 0o700 || !rootAfter.IsDir() || rootAfter.Mode().Perm() != 0o700 {
		return kit, fmt.Errorf("qualification kit directory changed during read")
	}
	kit.ContractJSON = files[relayqualification.KitContractFile]
	kit.CodexConfig = files[relayqualification.KitCodexConfigFile]
	kit.LiveManifestJSON = files[relayqualification.KitLiveManifestFile]
	kit.RuntimeHandoffJSON = files[relayqualification.KitRuntimeHandoffFile]
	kit.QualificationJSON = files[relayqualification.KitQualificationFile]
	kit.ManifestJSON = files[relayqualification.KitManifestFile]
	kit.Checksums = files[relayqualification.KitChecksumsFile]
	return kit, nil
}
