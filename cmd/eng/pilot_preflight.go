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
	repository := fs.String("repository", "", "absolute operator repository checkout")
	base := fs.String("base", "", "exact frozen main commit")
	profileFile := fs.String("codex-profile", "", "eng codex-profile JSON")
	policyFile := fs.String("access-policy", "", "rendered access policy JSON")
	preparationFile := fs.String("preparation", "", "rendered Worker preparation JSON")
	workerProfile := fs.String("worker-profile", "", "exact WorkerProfile")
	workerCodexFile := fs.String("worker-codex", "", "rendered Worker Codex config JSON; optional until WIF rule exists")
	publisherFile := fs.String("publisher", "", "publisher config JSON; optional until publisher credential exists")
	wifReceiptFile := fs.String("wif-receipt", "", "real codex-wif-live receipt JSON; optional until administrator WIF succeeds")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 {
		return fmt.Errorf("invalid pilot-preflight arguments")
	}
	for _, value := range []string{*repository, *base, *profileFile, *policyFile, *preparationFile, *workerProfile} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("repository, base, codex-profile, access-policy, preparation and worker-profile are required")
		}
	}
	if !filepath.IsAbs(*repository) || len(*base) != 40 {
		return fmt.Errorf("absolute repository path and full base commit are required")
	}

	profile, err := readPilotProfile(*profileFile)
	if err != nil {
		return err
	}
	if profile.ToolProfile != "codex/"+profile.ProfileDigest || profile.ProfileDigest == "" {
		return fmt.Errorf("invalid retained Codex profile output")
	}
	current, err := buildCodexProfile(profile.CodexExecutable, profile.Profile.Model)
	if err != nil {
		return err
	}
	if current.ProfileDigest != profile.ProfileDigest || current.CodexExecutable != profile.CodexExecutable {
		return fmt.Errorf("installed Codex profile bytes/model drifted")
	}
	mainCommit, err := pilotGitCommit(*repository, "refs/heads/main")
	if err != nil {
		return err
	}
	if strings.ToLower(*base) != mainCommit {
		return fmt.Errorf("repository main no longer matches frozen base")
	}

	doc, err := readPilotAccessPolicy(*policyFile)
	if err != nil {
		return err
	}
	if err := validatePilotPolicy(doc, *workerProfile, profile.ProfileDigest); err != nil {
		return err
	}
	if err := validatePilotPreparation(*preparationFile, *repository); err != nil {
		return err
	}

	result := pilotPreflightResult{
		Version: 1, Repository: "jiying2007/engineering-platform",
		BaseCommit: mainCommit, ProfileDigest: profile.ProfileDigest,
		WorkerProfile: *workerProfile, Internal: "READY",
		ModelExecution: "BLOCKED_EXTERNAL_WIF", Publication: "BLOCKED_EXTERNAL_PUBLISHER",
	}

	var workerCodex *pilotWorkerCodexConfig
	if *workerCodexFile == "" {
		result.Blockers = append(result.Blockers, "worker_codex_federation_rule")
	} else {
		config, err := readPilotWorkerCodex(*workerCodexFile)
		if err != nil {
			return err
		}
		if config.Executable != profile.CodexExecutable || config.Profile != profile.Profile {
			return fmt.Errorf("Worker Codex config does not bind exact generated profile")
		}
		workerCodex = &config
	}

	if *wifReceiptFile == "" {
		result.Blockers = append(result.Blockers, "managed_workspace_wif_qualification")
	} else {
		if workerCodex == nil {
			return fmt.Errorf("WIF receipt cannot be checked without Worker Codex configuration")
		}
		receipt, err := readPilotWIFReceipt(*wifReceiptFile)
		if err != nil {
			return err
		}
		if receipt.BinaryDigest != profile.Profile.BinaryDigest ||
			receipt.Model != profile.Profile.Model ||
			receipt.FederationRuleID != workerCodex.FederationRuleID {
			return fmt.Errorf("WIF qualification does not bind the exact Worker Codex profile")
		}
		result.ModelExecution = "READY"
	}

	if *publisherFile == "" {
		result.Blockers = append(result.Blockers, "publisher_configuration_or_credential")
	} else if err := validatePilotPublisher(*publisherFile); err != nil {
		return err
	} else {
		result.Publication = "READY"
	}

	if len(result.Blockers) == 0 {
		result.ModelExecution = "READY"
		result.Publication = "READY"
	}
	printJSON(result)
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
	var workerOK, publisherOK bool
	for _, principal := range doc.Principals {
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
	for _, path := range []string{config.Root, config.ContextSource, config.Git, config.Approvals[0].RepositoryPath} {
		if !filepath.IsAbs(path) {
			return fmt.Errorf("pilot preparation paths must be absolute")
		}
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
