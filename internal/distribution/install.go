package distribution

import (
	"context"
	"fmt"
)

type InstalledFile struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	Digest string `json:"digest"`
	Mode   uint32 `json:"mode"`
}

type InstallationManifest struct {
	Version      int             `json:"version"`
	SourceCommit string          `json:"source_commit"`
	Files        []InstalledFile `json:"files"`
}

type InstallationResult struct {
	Version              int    `json:"version"`
	Status               string `json:"status"`
	SourceCommit         string `json:"source_commit"`
	ManifestDigest       string `json:"manifest_digest"`
	BinaryCount          int    `json:"binary_count"`
	TemplateCount        int    `json:"template_count"`
	ServicesStarted      bool   `json:"services_started"`
	ConfigurationApplied bool   `json:"configuration_applied"`
	DependenciesIncluded bool   `json:"dependencies_included"`
	ExecutionAuthorized  bool   `json:"execution_authorized"`
	ProductionQualified  bool   `json:"production_qualified"`
}

// Install copies an already authenticated distribution and this source's
// non-secret deployment templates. It does not provision identities, contact
// services, invoke programs or overwrite an existing destination. Errors may
// leave partial files; even a manifest is not a success receipt.
func Install(ctx context.Context, from, into, expected string) (InstallationResult, error) {
	if !sourceCommit.MatchString(expected) {
		return InstallationResult{}, fmt.Errorf("exact expected source commit required")
	}
	return install(ctx, from, into, expected)
}

func VerifyInstallation(ctx context.Context, dir, expected string) (InstallationResult, error) {
	if !sourceCommit.MatchString(expected) {
		return InstallationResult{}, fmt.Errorf("exact expected source commit required")
	}
	return verifyInstallation(ctx, dir, expected)
}
