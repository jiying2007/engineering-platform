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
	CredentialMode   string            `json:"credential_mode"`
	FederationRuleID string            `json:"federation_rule_id,omitempty"`
	SavedLoginFile   string            `json:"saved_login_file,omitempty"`
	Profile          codexexec.Profile `json:"profile"`
}

func executeCodex(ctx context.Context, client *controlclient.Client, preparer *preparation.Preparer, workerProfile, runID string) error {
	configFile := os.Getenv("WORKER_CODEX_CONFIG")
	if configFile == "" {
		return fmt.Errorf("WORKER_CODEX_CONFIG is required")
	}
	if os.Getenv("OPENAI_API_KEY") != "" || os.Getenv("OPENAI_BASE_URL") != "" ||
		os.Getenv("CODEX_API_KEY") != "" || os.Getenv("CODEX_ACCESS_TOKEN") != "" {
		return fmt.Errorf("Core-bound Codex Worker refuses API/access tokens or provider endpoint override")
	}
	if os.Getenv("OPENAI_WORKLOAD_IDENTITY_CONTEXT") != "" {
		return fmt.Errorf("Worker owns workload identity audit context; external override is forbidden")
	}
	tokenFile := os.Getenv("OPENAI_IDENTITY_TOKEN_FILE")
	federationEnv := os.Getenv("OPENAI_FEDERATION_RULE_ID")
	data, err := access.ReadConfiguration(configFile, false)
	if err != nil {
		return err
	}
	var config codexConfiguration
	if err = strictjson.Decode(data, &config); err != nil {
		return err
	}
	if config.Version != 1 || strings.TrimSpace(config.Executable) == "" || config.Profile.Validate() != nil {
		return fmt.Errorf("invalid Worker Codex configuration")
	}
	switch config.CredentialMode {
	case "workload_identity":
		if strings.TrimSpace(config.FederationRuleID) == "" || config.SavedLoginFile != "" ||
			tokenFile == "" || federationEnv != "" {
			return fmt.Errorf("invalid workload-identity Worker Codex configuration")
		}
	case "saved_chatgpt_login":
		if config.FederationRuleID != "" || strings.TrimSpace(config.SavedLoginFile) == "" ||
			tokenFile != "" || federationEnv != "" {
			return fmt.Errorf("invalid saved-login Worker Codex configuration")
		}
	default:
		return fmt.Errorf("unsupported Worker Codex credential mode")
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
		Executable: config.Executable, CredentialMode: config.CredentialMode,
		FederationRuleID: config.FederationRuleID, IdentityTokenFile: tokenFile,
		SavedLoginFile: config.SavedLoginFile, AuditContext: string(audit),
	})
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(receipt)
}
