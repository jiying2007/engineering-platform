package main

import (
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
	printJSON(verification)
	return nil
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
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0o700 {
		return kit, fmt.Errorf("qualification kit directory must be owner-private mode 0700")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return kit, err
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
	files := map[string][]byte{}
	for _, entry := range entries {
		if !expected[entry.Name()] || entry.Type()&os.ModeSymlink != 0 || entry.IsDir() {
			return kit, fmt.Errorf("unexpected qualification kit entry %q", entry.Name())
		}
		data, err := readRelayKitFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return kit, fmt.Errorf("%s: %w", entry.Name(), err)
		}
		files[entry.Name()] = data
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

func readRelayKitFile(path string) ([]byte, error) {
	before, err := os.Lstat(path)
	if err != nil || !before.Mode().IsRegular() || before.Mode().Perm() != 0o600 ||
		before.Size() <= 0 || before.Size() > 2<<20 {
		return nil, fmt.Errorf("bounded owner-private regular file required")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(before, opened) {
		return nil, fmt.Errorf("qualification kit file changed before read")
	}
	data, err := io.ReadAll(io.LimitReader(file, (2<<20)+1))
	if err != nil || len(data) == 0 || len(data) > 2<<20 {
		return nil, fmt.Errorf("qualification kit file read failed or exceeds bound")
	}
	after, err := os.Lstat(path)
	if err != nil || !os.SameFile(before, after) || after.Size() != before.Size() {
		return nil, fmt.Errorf("qualification kit file changed during read")
	}
	return data, nil
}
