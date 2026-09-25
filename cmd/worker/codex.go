package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/jiying2007/engineering-platform/internal/access"
	"github.com/jiying2007/engineering-platform/internal/codexexec"
	"github.com/jiying2007/engineering-platform/internal/controlclient"
	"github.com/jiying2007/engineering-platform/internal/preparation"
	"github.com/jiying2007/engineering-platform/internal/strictjson"
	"github.com/jiying2007/engineering-platform/internal/workeragent"
)

type codexConfiguration struct {
	Version          int               `json:"version"`
	Executable       string            `json:"codex_executable"`
	FederationRuleID string            `json:"federation_rule_id"`
	Profile          codexexec.Profile `json:"profile"`
}

func executeCodex(ctx context.Context, client *controlclient.Client, preparer *preparation.Preparer, workerProfile, runID string) error {
	configFile := os.Getenv("WORKER_CODEX_CONFIG")
	if configFile == "" {
		return fmt.Errorf("WORKER_CODEX_CONFIG is required")
	}
	if os.Getenv("OPENAI_API_KEY") != "" || os.Getenv("OPENAI_BASE_URL") != "" {
		return fmt.Errorf("Core-bound Codex Worker refuses long-lived API key or provider endpoint override")
	}
	if os.Getenv("OPENAI_WORKLOAD_IDENTITY_CONTEXT") != "" {
		return fmt.Errorf("Worker owns workload identity audit context; external override is forbidden")
	}
	tokenFile := os.Getenv("OPENAI_IDENTITY_TOKEN_FILE")
	if tokenFile == "" {
		return fmt.Errorf("OPENAI_IDENTITY_TOKEN_FILE is required")
	}
	data, err := access.ReadConfiguration(configFile, false)
	if err != nil {
		return err
	}
	var config codexConfiguration
	if err = strictjson.Decode(data, &config); err != nil {
		return err
	}
	if config.Version != 1 || strings.TrimSpace(config.Executable) == "" ||
		strings.TrimSpace(config.FederationRuleID) == "" || config.Profile.Validate() != nil {
		return fmt.Errorf("invalid Worker Codex configuration")
	}
	profileDigest, err := config.Profile.Digest()
	if err != nil {
		return err
	}
	audit, err := json.Marshal(map[string]string{
		"run_id":         runID,
		"worker_subject": client.Subject(),
		"worker_profile": workerProfile,
		"profile_digest": profileDigest,
	})
	if err != nil {
		return err
	}
	receipt, err := workeragent.ExecuteCodex(ctx, client, preparer, codexexec.Start{
		RunID: runID, WorkerProfile: workerProfile, Profile: config.Profile,
	}, workeragent.CodexRuntime{
		Executable: config.Executable, FederationRuleID: config.FederationRuleID,
		IdentityTokenFile: tokenFile, AuditContext: string(audit),
	})
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(receipt)
}
