package githubpublish

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/jiying2007/engineering-platform/internal/access"
	"github.com/jiying2007/engineering-platform/internal/strictjson"
)

var (
	repositoryPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,100}/[A-Za-z0-9_.-]{1,100}$`)
	fullSHA           = regexp.MustCompile(`^[0-9a-f]{40}$`)
	digestPattern     = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	executionID       = regexp.MustCompile(`^[0-9a-f]{64}$`)
	refPattern        = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,159}$`)
)

type TargetPolicy struct {
	Repository   string `json:"repository"`
	BaseRef      string `json:"base_ref"`
	BranchPrefix string `json:"branch_prefix"`
}

type Configuration struct {
	Version       int            `json:"version"`
	ArtifactRoot  string         `json:"artifact_root"`
	GitExecutable string         `json:"git_executable"`
	TokenFile     string         `json:"token_file"`
	Targets       []TargetPolicy `json:"targets"`
}

func Load(path string, state State) (*Provider, error) {
	data, err := access.ReadConfiguration(path, false)
	if err != nil {
		return nil, err
	}
	var config Configuration
	if err := strictjson.Decode(data, &config); err != nil {
		return nil, err
	}
	remote, err := newGitHubRemote(config.GitExecutable, config.TokenFile)
	if err != nil {
		return nil, err
	}
	return New(config, state, remote)
}

func New(config Configuration, state State, remote Remote) (*Provider, error) {
	if config.Version != 1 || state == nil || remote == nil || len(config.Targets) == 0 || len(config.Targets) > 64 {
		return nil, fmt.Errorf("publisher requires version 1, state, remote and 1..64 targets")
	}
	artifactRoot, info, err := canonicalDirectory(config.ArtifactRoot)
	if err != nil {
		return nil, err
	}
	git, err := canonicalExecutable(config.GitExecutable)
	if err != nil {
		return nil, err
	}
	tokenFile, err := canonicalSecret(config.TokenFile)
	if err != nil {
		return nil, err
	}
	config.ArtifactRoot = artifactRoot
	config.GitExecutable = git
	config.TokenFile = tokenFile
	targets := make(map[string]TargetPolicy, len(config.Targets))
	for _, target := range config.Targets {
		if !validRepository(target.Repository) || !validRef(target.BaseRef) ||
			!validBranchPrefix(target.BranchPrefix) {
			return nil, fmt.Errorf("invalid GitHub publication target policy")
		}
		if _, exists := targets[target.Repository]; exists {
			return nil, fmt.Errorf("duplicate GitHub publication repository policy")
		}
		targets[target.Repository] = target
	}
	return &Provider{state: state, artifactRoot: artifactRoot, artifactIdentity: info, targets: targets, remote: remote}, nil
}

func validRepository(value string) bool {
	return repositoryPattern.MatchString(value) && !strings.HasSuffix(value, ".")
}

func validRef(value string) bool {
	if !refPattern.MatchString(value) || strings.HasPrefix(value, "/") || strings.HasSuffix(value, "/") ||
		strings.Contains(value, "..") || strings.Contains(value, "//") || strings.Contains(value, "@{") ||
		strings.HasSuffix(value, ".") || strings.HasSuffix(value, ".lock") {
		return false
	}
	return true
}

func validBranchPrefix(value string) bool {
	if !strings.HasSuffix(value, "/") || len(value) > 96 {
		return false
	}
	return validRef(strings.TrimSuffix(value, "/"))
}

func canonicalDirectory(path string) (string, os.FileInfo, error) {
	if !filepath.IsAbs(path) {
		return "", nil, fmt.Errorf("publisher artifact root must be absolute")
	}
	clean := filepath.Clean(path)
	resolved, err := filepath.EvalSymlinks(clean)
	if err != nil || resolved != clean {
		return "", nil, fmt.Errorf("publisher artifact root must be canonical")
	}
	info, err := os.Lstat(clean)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0o022 != 0 {
		return "", nil, fmt.Errorf("publisher artifact root is unsafe")
	}
	return clean, info, nil
}

func canonicalExecutable(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("publisher Git executable must be absolute")
	}
	clean := filepath.Clean(path)
	resolved, err := filepath.EvalSymlinks(clean)
	if err != nil || resolved != clean {
		return "", fmt.Errorf("publisher Git executable must be canonical")
	}
	info, err := os.Lstat(clean)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o022 != 0 || info.Mode().Perm()&0o111 == 0 {
		return "", fmt.Errorf("publisher Git executable is unsafe")
	}
	return clean, nil
}

func canonicalSecret(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("publisher token file must be absolute")
	}
	clean := filepath.Clean(path)
	resolved, err := filepath.EvalSymlinks(clean)
	if err != nil || resolved != clean {
		return "", fmt.Errorf("publisher token file must be canonical")
	}
	data, err := access.ReadConfiguration(clean, true)
	if err != nil {
		return "", err
	}
	token := strings.TrimSuffix(strings.TrimSuffix(string(data), "\n"), "\r")
	if token == "" || len(token) > 4096 || strings.TrimSpace(token) != token {
		return "", fmt.Errorf("publisher token file is invalid")
	}
	return clean, nil
}
