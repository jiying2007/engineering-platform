// Package distribution owns the complete executable delivery contract.
// Historical receipts retain their original source/tool version; they are not
// rewritten or admitted as a current distribution by this contract.
package distribution

import (
	"bytes"
	"crypto/sha256"
	"debug/buildinfo"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/jiying2007/engineering-platform/internal/strictjson"
)

//go:embed binaries.txt
var binaryNames string

const Module = "github.com/jiying2007/engineering-platform"

var sourceCommit = regexp.MustCompile(`^[0-9a-f]{40}$`)

func Names() []string { return strings.Fields(binaryNames) }

type File struct {
	Path   string `json:"path"`
	Digest string `json:"digest"`
	Size   int64  `json:"size"`
}

type Result struct {
	Status       string `json:"status"`
	SourceCommit string `json:"source_commit"`
	BinaryCount  int    `json:"binary_count"`
}

// Verify reads an already delivered directory. It neither builds missing roles
// nor invokes the candidate executables. Trust in the archive digest belongs
// to its authenticated distributor, not to this self-contained byte check.
func Verify(dir string) (Result, error) {
	var result Result
	if !filepath.IsAbs(dir) || filepath.Clean(dir) != dir {
		return result, fmt.Errorf("absolute normalized distribution directory required")
	}
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil || resolved != dir {
		return result, fmt.Errorf("distribution aliases are forbidden")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return result, err
	}
	names := Names()
	if len(entries) != len(names)+2 {
		return result, fmt.Errorf("distribution has missing or extra members")
	}
	data, err := readFile(filepath.Join(dir, "file-manifest.json"), 1<<20, false)
	if err != nil {
		return result, err
	}
	if err := strictjson.ValidateObject(append(append([]byte(`{"files":`), data...), byte('}'))); err != nil {
		return result, fmt.Errorf("ambiguous file manifest")
	}
	var files []File
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&files) != nil || decoder.Decode(new(any)) != io.EOF || len(files) != len(names) {
		return result, fmt.Errorf("invalid executable manifest")
	}
	var sums strings.Builder
	revision := ""
	for i, name := range names {
		fact := files[i]
		if fact.Path != name || fact.Size <= 0 || fact.Size > 128<<20 {
			return result, fmt.Errorf("incomplete or unordered role manifest")
		}
		path := filepath.Join(dir, name)
		content, e := readFile(path, 128<<20, true)
		if e != nil {
			return result, e
		}
		digest := sha256.Sum256(content)
		raw := hex.EncodeToString(digest[:])
		if int64(len(content)) != fact.Size || "sha256:"+raw != fact.Digest {
			return result, fmt.Errorf("binary %s differs from manifest", name)
		}
		info, e := buildinfo.Read(bytes.NewReader(content))
		if e != nil || info.Path != Module+"/cmd/"+name {
			return result, fmt.Errorf("binary %s has wrong build role", name)
		}
		settings := map[string]string{}
		for _, s := range info.Settings {
			settings[s.Key] = s.Value
		}
		if !sourceCommit.MatchString(settings["vcs.revision"]) || settings["vcs.modified"] != "false" || settings["GOOS"] != "linux" || settings["GOARCH"] != "amd64" {
			return result, fmt.Errorf("binary %s lacks clean Linux/amd64 source identity", name)
		}
		if revision != "" && revision != settings["vcs.revision"] {
			return result, fmt.Errorf("distribution mixes source commits")
		}
		revision = settings["vcs.revision"]
		fmt.Fprintf(&sums, "%s  %s\n", raw, name)
	}
	actual, err := readFile(filepath.Join(dir, "SHA256SUMS"), 64<<10, false)
	if err != nil || !bytes.Equal(actual, []byte(sums.String())) {
		return result, fmt.Errorf("SHA256SUMS differs from exact executable manifest")
	}
	return Result{Status: "BYTES_VERIFIED", SourceCommit: revision, BinaryCount: len(names)}, nil
}

func readFile(path string, limit int64, executable bool) ([]byte, error) {
	before, err := os.Lstat(path)
	if err != nil || !before.Mode().IsRegular() || before.Mode().Perm()&0o022 != 0 || before.Size() <= 0 || before.Size() > limit || (executable && before.Mode().Perm()&0o111 == 0) {
		return nil, fmt.Errorf("unsafe distribution member %s", filepath.Base(path))
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !os.SameFile(before, opened) {
		return nil, fmt.Errorf("distribution member changed")
	}
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil || int64(len(data)) > limit {
		return nil, fmt.Errorf("distribution member read failed")
	}
	after, err := os.Lstat(path)
	if err != nil || !os.SameFile(before, after) || after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) {
		return nil, fmt.Errorf("distribution member changed during read")
	}
	return data, nil
}
