package engineeringevidence

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/jiying2007/engineering-platform/internal/access"
	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/core"
	"github.com/jiying2007/engineering-platform/internal/workspace"
)

const (
	GitIssuer       = access.GitEvidenceIssuer
	GitProcedure    = access.GitEvidenceProcedure
	GitImporter     = access.GitEvidenceImporterSubject
	GitArtifactType = "application/vnd.engineering-platform.git-changed-tree+json"
)

var gitObjectPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

type GitChangeManifest struct {
	SchemaVersion      int    `json:"schema_version"`
	BaseCommit         string `json:"base_commit"`
	BaseTree           string `json:"base_tree"`
	BaseSourceDigest   string `json:"base_source_digest"`
	ResultCommit       string `json:"result_commit"`
	ResultTree         string `json:"result_tree"`
	ResultSourceDigest string `json:"result_source_digest"`
}

func (m GitChangeManifest) Validate() error {
	if m.SchemaVersion != 1 ||
		!gitObjectPattern.MatchString(m.BaseCommit) ||
		!gitObjectPattern.MatchString(m.BaseTree) ||
		!canonical.ValidDigest(m.BaseSourceDigest) ||
		!gitObjectPattern.MatchString(m.ResultCommit) ||
		!gitObjectPattern.MatchString(m.ResultTree) ||
		!canonical.ValidDigest(m.ResultSourceDigest) ||
		m.BaseCommit == m.ResultCommit ||
		m.BaseTree == m.ResultTree ||
		m.BaseSourceDigest == m.ResultSourceDigest {
		return fmt.Errorf("invalid or unchanged Git tree manifest")
	}
	return nil
}

func CaptureGitChange(ctx context.Context, repository, baseCommit, resultCommit string) (GitChangeManifest, error) {
	var manifest GitChangeManifest
	if !filepath.IsAbs(repository) || !gitObjectPattern.MatchString(strings.ToLower(baseCommit)) ||
		!gitObjectPattern.MatchString(strings.ToLower(resultCommit)) {
		return manifest, fmt.Errorf("absolute repository path and full Git commits required")
	}
	root, err := os.MkdirTemp("", "engineering-platform-git-evidence-")
	if err != nil {
		return manifest, err
	}
	defer os.RemoveAll(root)
	manager, err := workspace.New(root)
	if err != nil {
		return manifest, err
	}
	base, err := manager.Create(ctx, workspace.Spec{ID: "base", Repository: repository, BaseCommit: strings.ToLower(baseCommit)})
	if err != nil {
		return manifest, err
	}
	result, err := manager.Create(ctx, workspace.Spec{ID: "result", Repository: repository, BaseCommit: strings.ToLower(resultCommit)})
	if err != nil {
		return manifest, err
	}
	manifest = GitChangeManifest{
		SchemaVersion:      1,
		BaseCommit:         base.BaseCommit,
		BaseTree:           base.TreeCommit,
		BaseSourceDigest:   base.SourceDigest,
		ResultCommit:       result.BaseCommit,
		ResultTree:         result.TreeCommit,
		ResultSourceDigest: result.SourceDigest,
	}
	if err := manifest.Validate(); err != nil {
		return GitChangeManifest{}, err
	}
	return manifest, nil
}

func MarshalGitChangeManifest(manifest GitChangeManifest) ([]byte, error) {
	if err := manifest.Validate(); err != nil {
		return nil, err
	}
	return json.MarshalIndent(manifest, "", "  ")
}

func ReadGitChangeManifest(path string) (GitChangeManifest, string, error) {
	var manifest GitChangeManifest
	before, err := os.Lstat(path)
	if err != nil || !before.Mode().IsRegular() || before.Mode()&os.ModeSymlink != 0 || before.Size() <= 0 || before.Size() > 1<<20 {
		return manifest, "", fmt.Errorf("bounded regular Git manifest required")
	}
	file, err := os.Open(path)
	if err != nil {
		return manifest, "", err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(before, opened) {
		return manifest, "", fmt.Errorf("Git manifest changed before read")
	}
	data, err := io.ReadAll(io.LimitReader(file, (1<<20)+1))
	if err != nil || len(data) > 1<<20 {
		return manifest, "", fmt.Errorf("Git manifest read failed or exceeds limit")
	}
	after, err := os.Lstat(path)
	if err != nil || !os.SameFile(before, after) || after.Size() != before.Size() {
		return manifest, "", fmt.Errorf("Git manifest changed during read")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil || decoder.Decode(new(any)) != io.EOF {
		return GitChangeManifest{}, "", fmt.Errorf("invalid Git manifest JSON")
	}
	if err := manifest.Validate(); err != nil {
		return GitChangeManifest{}, "", err
	}
	return manifest, canonical.BytesDigest(data), nil
}

type GitImportRequest struct {
	EvidenceID         string
	RequirementID      string
	EvidenceArtifactID string
	Delivery           core.DeliveryReceipt
	Manifest           GitChangeManifest
	ManifestFileDigest string
	Observed           GitChangeManifest
}

func VerifyGitImport(req GitImportRequest) (core.EvidenceRef, error) {
	var empty core.EvidenceRef
	if !validLogicalID(req.EvidenceID) || !validLogicalID(req.RequirementID) || !validLogicalID(req.EvidenceArtifactID) {
		return empty, fmt.Errorf("bounded evidence, requirement and artifact IDs required")
	}
	if err := verifyDelivery(req.Delivery); err != nil {
		return empty, err
	}
	if req.Delivery.ResultCommit == "" ||
		req.Manifest.BaseCommit != strings.ToLower(req.Delivery.BaseCommit) ||
		req.Manifest.ResultCommit != strings.ToLower(req.Delivery.ResultCommit) {
		return empty, fmt.Errorf("Git manifest does not bind delivery commits")
	}
	if err := req.Manifest.Validate(); err != nil {
		return empty, err
	}
	if err := req.Observed.Validate(); err != nil || req.Manifest != req.Observed {
		return empty, fmt.Errorf("live Git snapshots do not match retained manifest")
	}
	if !canonical.ValidDigest(req.ManifestFileDigest) {
		return empty, fmt.Errorf("Git manifest file digest required")
	}
	artifact, ok := exactArtifact(req.Delivery, req.EvidenceArtifactID)
	if !ok || artifact.Digest != req.ManifestFileDigest {
		return empty, fmt.Errorf("delivery must bind exact Git manifest file digest")
	}
	if artifact.MediaType != "" && artifact.MediaType != GitArtifactType {
		return empty, fmt.Errorf("Git manifest delivery artifact has wrong media type")
	}
	return core.EvidenceRef{
		ID:                req.EvidenceID,
		DeliveryReceiptID: req.Delivery.ID,
		RequirementID:     req.RequirementID,
		SubjectDigest:     req.Delivery.SubjectDigest,
		Issuer:            GitIssuer,
		Procedure:         GitProcedure,
		Result:            "PASS",
		ArtifactRefs:      []string{artifact.ID},
		Applicable:        true,
	}, nil
}
