package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/jiying2007/engineering-platform/internal/access"
	"github.com/jiying2007/engineering-platform/internal/codexexec"
	"github.com/jiying2007/engineering-platform/internal/core"
	"github.com/jiying2007/engineering-platform/internal/githubpublish"
	"github.com/jiying2007/engineering-platform/internal/preparation"
	"github.com/jiying2007/engineering-platform/internal/run"
	"github.com/jiying2007/engineering-platform/internal/runtime/codexapp"
	"github.com/jiying2007/engineering-platform/internal/session"
	"github.com/jiying2007/engineering-platform/internal/strictjson"
)

type pilotPreflightOptions struct {
	Repository        string
	Base              string
	ProfileFile       string
	PolicyFile        string
	PreparationFile   string
	WorkerProfile     string
	WorkerCodexFile   string
	PublisherFile     string
	WIFReceiptFile    string
}

type pilotPreflightResult struct {
	Version        int      `json:"version"`
	Repository     string   `json:"repository"`
	BaseCommit     string   `json:"base_commit"`
	ProfileDigest  string   `json:"profile_digest"`
	WorkerProfile  string   `json:"worker_profile"`
	Internal       string   `json:"internal"`
	ModelExecution string   `json:"model_execution"`
	Publication    string   `json:"publication"`
	Blockers       []string `json:"external_blockers,omitempty"`
}

type pilotWorkerCodexConfig struct {
	Version          int               `json:"version"`
	Executable       string            `json:"codex_executable"`
	FederationRuleID string            `json:"federation_rule_id"`
	Profile          codexexec.Profile `json:"profile"`
}

func pilotPreflight(args []string) error {
	fs := flag.NewFlagSet("pilot-preflight", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var options pilotPreflightOptions
	fs.StringVar(&options.Repository, "repository", "", "absolute operator repository checkout")
	fs.StringVar(&options.Base, "base", "", "exact frozen main commit")
	fs.StringVar(&options.ProfileFile, "codex-profile", "", "eng codex-profile JSON")
	fs.StringVar(&options.PolicyFile, "access-policy", "", "rendered access policy JSON")
	fs.StringVar(&options.PreparationFile, "preparation", "", "rendered Worker preparation JSON")
	fs.StringVar(&options.WorkerProfile, "worker-profile", "", "exact WorkerProfile")
	fs.StringVar(&options.WorkerCodexFile, "worker-codex", "", "rendered Worker Codex config JSON; optional until WIF rule exists")
	fs.StringVar(&options.PublisherFile, "publisher", "", "publisher config JSON; optional until publisher credential exists")
	fs.StringVar(&options.WIFReceiptFile, "wif-receipt", "", "real codex-wif-live receipt JSON; optional until administrator WIF succeeds")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 {
		return fmt.Errorf("invalid pilot-preflight arguments")
	}
	result, err := checkPilotPreflight(options)
	if err != nil {
		return err
	}
	printJSON(result)
	return nil
}

func checkPilotPreflight(options pilotPreflightOptions) (pilotPreflightResult, error) {
	var empty pilotPreflightResult
	for _, value := range []string{options.Repository, options.Base, options.ProfileFile, options.PolicyFile, options.PreparationFile, options.WorkerProfile} {
		if strings.TrimSpace(value) == "" {
			return empty, fmt.Errorf("repository, base, codex-profile, access-policy, preparation and worker-profile are required")
		}
	}
	if err := pilotSafeDir(options.Repository); err != nil {
		return empty, fmt.Errorf("repository checkout: %w", err)
	}
	if len(options.Base) != 40 {
		return empty, fmt.Errorf("full base commit required")
	}

	profile, err := readPilotProfile(options.ProfileFile)
	if err != nil {
		return empty, err
	}
	if profile.ToolProfile != "codex/"+profile.ProfileDigest || profile.ProfileDigest == "" {
		return empty, fmt.Errorf("invalid retained Codex profile output")
	}
	current, err := buildCodexProfile(profile.CodexExecutable, profile.Profile.Model)
	if err != nil {
		return empty, err
	}
	if current.ProfileDigest != profile.ProfileDigest || current.CodexExecutable != profile.CodexExecutable {
		return empty, fmt.Errorf("installed Codex profile bytes/model drifted")
	}
	mainCommit, err := pilotGitCommit(options.Repository, "refs/heads/main")
	if err != nil {
		return empty, err
	}
	if strings.ToLower(options.Base) != mainCommit {
		return empty, fmt.Errorf("repository main no longer matches frozen base")
	}

	doc, err := readPilotAccessPolicy(options.PolicyFile)
	if err != nil {
		return empty, err
	}
	if err := validatePilotPolicy(doc, options.WorkerProfile, profile.ProfileDigest); err != nil {
		return empty, err
	}
	if err := validatePilotPreparation(options.PreparationFile, options.Repository); err != nil {
		return empty, err
	}

	result := pilotPreflightResult{
		Version: 1, Repository: "jiying2007/engineering-platform",
		BaseCommit: mainCommit, ProfileDigest: profile.ProfileDigest,
		WorkerProfile: options.WorkerProfile, Internal: "READY",
		ModelExecution: "BLOCKED_EXTERNAL_WIF", Publication: "BLOCKED_EXTERNAL_PUBLISHER",
	}

	var workerCodex *pilotWorkerCodexConfig
	if options.WorkerCodexFile == "" {
		result.Blockers = append(result.Blockers, "worker_codex_federation_rule")
	} else {
		config, err := readPilotWorkerCodex(options.WorkerCodexFile)
		if err != nil {
			return empty, err
		}
		if config.Executable != profile.CodexExecutable || config.Profile != profile.Profile {
			return empty, fmt.Errorf("Worker Codex config does not bind exact generated profile")
		}
		workerCodex = &config
	}

	if options.WIFReceiptFile == "" {
		result.Blockers = append(result.Blockers, "managed_workspace_wif_qualification")
	} else {
		if workerCodex == nil {
			return empty, fmt.Errorf("WIF receipt cannot be checked without Worker Codex configuration")
		}
		receipt, err := readPilotWIFReceipt(options.WIFReceiptFile)
		if err != nil {
			return empty, err
		}
		if receipt.BinaryDigest != profile.Profile.BinaryDigest ||
			receipt.Model != profile.Profile.Model ||
			receipt.FederationRuleID != workerCodex.FederationRuleID {
			return empty, fmt.Errorf("WIF qualification does not bind the exact Worker Codex profile")
		}
		result.ModelExecution = "READY"
	}

	if options.PublisherFile == "" {
		result.Blockers = append(result.Blockers, "publisher_configuration_or_credential")
	} else if err := validatePilotPublisher(options.PublisherFile); err != nil {
		return empty, err
	} else {
		result.Publication = "READY"
	}

	return result, nil
}
func pilotSafeDir(path string) error {
	if !filepath.IsAbs(path) {
		return fmt.Errorf("absolute directory required")
	}
	clean := filepath.Clean(path)
	resolved, err := filepath.EvalSymlinks(clean)
	if err != nil || resolved != clean {
		return fmt.Errorf("canonical directory required")
	}
	info, err := os.Lstat(clean)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("safe non-group/world-writable directory required")
	}
	return nil
}

func pilotSafeExecutable(path string) error {
	if !filepath.IsAbs(path) {
		return fmt.Errorf("absolute executable required")
	}
	clean := filepath.Clean(path)
	resolved, err := filepath.EvalSymlinks(clean)
	if err != nil || resolved != clean {
		return fmt.Errorf("canonical executable required")
	}
	info, err := os.Lstat(clean)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o022 != 0 || info.Mode().Perm()&0o111 == 0 {
		return fmt.Errorf("safe executable required")
	}
	return nil
}

func readPilotProfile(path string) (codexProfileOutput, error) {
	var output codexProfileOutput
	data, err := os.ReadFile(path)
	if err != nil {
		return output, err
	}
	if err := strictjson.Decode(data, &output); err != nil {
		return output, err
	}
	return output, nil
}

func readPilotAccessPolicy(path string) (access.Document, error) {
	var doc access.Document
	data, err := access.ReadConfiguration(path, false)
	if err != nil {
		return doc, err
	}
	if _, err := access.Decode(data); err != nil {
		return doc, err
	}
	if err := strictjson.Decode(data, &doc); err != nil {
		return doc, err
	}
	return doc, nil
}

func validatePilotPolicy(doc access.Document, workerProfile, profileDigest string) error {
	required := map[string]bool{
		"urn:engineering-platform:operator:pilot-owner": false,
		"urn:engineering-platform:worker:codex-pilot": false,
		"urn:engineering-platform:operator:pilot-publisher": false,
		access.CodexEvidenceImporterSubject: false,
		access.GitEvidenceImporterSubject: false,
		access.TrustedCIImporterSubject: false,
		"urn:engineering-platform:verifier:pilot": false,
		"urn:engineering-platform:reviewer:pilot": false,
		"urn:engineering-platform:closure:pilot": false,
	}
	var workerOK, publisherOK bool
	for _, principal := range doc.Principals {
		if _, ok := required[principal.Subject]; ok {
			required[principal.Subject] = true
		}
		switch principal.Subject {
		case "urn:engineering-platform:worker:codex-pilot":
			if len(principal.WorkerProfiles) != 1 || principal.WorkerProfiles[0] != workerProfile {
				return fmt.Errorf("pilot WorkerProfile grant mismatch")
			}
			for _, grant := range principal.Actions {
				if grant.Action == "github.publish-pr" {
					return fmt.Errorf("pilot Worker must not hold publication grant")
				}
				if grant.Action == codexexec.Action && grant.RiskClass == "CONTROLLED_MUTATION" && grant.Capability == profileDigest {
					workerOK = true
				}
			}
		case "urn:engineering-platform:operator:pilot-publisher":
			for _, grant := range principal.Actions {
				if grant.Action == "github.publish-pr" && grant.RiskClass == "CONTROLLED_MUTATION" && grant.Capability == "github.publish-pr" {
					publisherOK = true
				}
				if grant.Action == codexexec.Action {
					return fmt.Errorf("publisher requester must not hold Codex execution grant")
				}
			}
		}
	}
	for subject, found := range required {
		if !found {
			return fmt.Errorf("pilot access policy missing principal %s", subject)
		}
	}
	if !workerOK || !publisherOK {
		return fmt.Errorf("pilot access policy lacks exact Worker/publisher grants")
	}
	return nil
}

func validatePilotPreparation(path, repository string) error {
	data, err := access.ReadConfiguration(path, false)
	if err != nil {
		return err
	}
	var config preparation.Configuration
	if err := strictjson.Decode(data, &config); err != nil {
		return err
	}
	if config.Version != 1 || config.Worker != "urn:engineering-platform:worker:codex-pilot" ||
		len(config.Approvals) != 1 || config.Approvals[0].Repository != "jiying2007/engineering-platform" ||
		filepath.Clean(config.Approvals[0].RepositoryPath) != filepath.Clean(repository) {
		return fmt.Errorf("pilot preparation config identity mismatch")
	}
	if err := pilotSafeDir(config.Root); err != nil {
		return fmt.Errorf("preparation root: %w", err)
	}
	if err := pilotSafeDir(config.ContextSource); err != nil {
		return fmt.Errorf("context source: %w", err)
	}
	if err := pilotSafeDir(config.Approvals[0].RepositoryPath); err != nil {
		return fmt.Errorf("approved repository: %w", err)
	}
	if err := pilotSafeExecutable(config.Git); err != nil {
		return fmt.Errorf("preparation git: %w", err)
	}
	return nil
}

func readPilotWorkerCodex(path string) (pilotWorkerCodexConfig, error) {
	var config pilotWorkerCodexConfig
	data, err := access.ReadConfiguration(path, false)
	if err != nil {
		return config, err
	}
	if err := strictjson.Decode(data, &config); err != nil {
		return config, err
	}
	if config.Version != 1 || strings.TrimSpace(config.FederationRuleID) == "" || config.Profile.Validate() != nil {
		return config, fmt.Errorf("invalid Worker Codex configuration")
	}
	return config, nil
}

func readPilotWIFReceipt(path string) (codexapp.LiveReceipt, error) {
	var receipt codexapp.LiveReceipt
	data, err := os.ReadFile(path)
	if err != nil {
		return receipt, err
	}
	if err := strictjson.Decode(data, &receipt); err != nil {
		return receipt, err
	}
	if err := receipt.Validate(); err != nil {
		return receipt, err
	}
	return receipt, nil
}

func validatePilotPublisher(path string) error {
	data, err := access.ReadConfiguration(path, false)
	if err != nil {
		return err
	}
	var config githubpublish.Configuration
	if err := strictjson.Decode(data, &config); err != nil {
		return err
	}
	found := false
	for _, target := range config.Targets {
		if target.Repository == "jiying2007/engineering-platform" && target.BaseRef == "main" {
			found = true
		}
	}
	if !found {
		return fmt.Errorf("publisher config lacks exact engineering-platform/main target")
	}
	_, err = githubpublish.New(config, pilotPublisherState{}, pilotPublisherRemote{})
	return err
}

func pilotGitCommit(repository, ref string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "-C", repository, "rev-parse", "--verify", ref+"^{commit}")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("resolve repository %s: %w", ref, err)
	}
	commit := strings.ToLower(strings.TrimSpace(string(out)))
	if len(commit) != 40 {
		return "", fmt.Errorf("repository returned invalid commit")
	}
	return commit, nil
}

type pilotPublisherState struct{}

func (pilotPublisherState) GetExecution(string) (run.Run, session.Session, error) {
	return run.Run{}, session.Session{}, nil
}
func (pilotPublisherState) GetTaskByDigest(string) (core.TaskContract, error) {
	return core.TaskContract{}, nil
}
func (pilotPublisherState) GetCodex(context.Context, string) (codexexec.Status, error) {
	return codexexec.Status{}, nil
}

type pilotPublisherRemote struct{}

func (pilotPublisherRemote) Publish(context.Context, githubpublish.Plan, string) (githubpublish.PublicationReceipt, error) {
	return githubpublish.PublicationReceipt{}, nil
}
func (pilotPublisherRemote) Observe(context.Context, githubpublish.Plan) (githubpublish.ObserveResult, error) {
	return githubpublish.ObserveResult{}, nil
}
