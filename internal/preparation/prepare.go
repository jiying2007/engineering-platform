package preparation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jiying2007/engineering-platform/internal/access"
	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/contextbundle"
	"github.com/jiying2007/engineering-platform/internal/core"
	"github.com/jiying2007/engineering-platform/internal/strictjson"
	"github.com/jiying2007/engineering-platform/internal/workerqueue"
	"github.com/jiying2007/engineering-platform/internal/workspace"
)

type Approval struct {
	RunID          string            `json:"run_id"`
	TaskDigest     string            `json:"task_contract_digest"`
	InputDigest    string            `json:"run_input_manifest_digest"`
	Repository     string            `json:"repository"`
	RepositoryPath string            `json:"repository_path"`
	Refs           []core.ContextRef `json:"context_refs"`
}
type Configuration struct {
	Version       int        `json:"version"`
	Worker        string     `json:"worker_subject"`
	Root          string     `json:"root"`
	Git           string     `json:"git_executable"`
	ContextSource string     `json:"context_source"`
	Approvals     []Approval `json:"approvals"`
}
type Preparer struct {
	worker, root string
	manager      *workspace.Manager
	source       *contextbundle.LocalSource
	bundles      *contextbundle.Materializer
	approvals    map[string]Approval
}
type Result struct {
	Facts      Facts               `json:"facts"`
	Workspace  workspace.Workspace `json:"workspace"`
	BundlePath string              `json:"bundle_path"`
}

func Load(path string) (*Preparer, error) {
	data, err := access.ReadConfiguration(path, false)
	if err != nil {
		return nil, err
	}
	var c Configuration
	if err := strictjson.Decode(data, &c); err != nil {
		return nil, err
	}
	return New(c)
}

// New only accepts an operator snapshot, not Task-provided locators or grants.
func New(c Configuration) (*Preparer, error) {
	if c.Version != 1 || !strings.HasPrefix(c.Worker, "urn:engineering-platform:") || !filepath.IsAbs(c.Root) || len(c.Approvals) == 0 || len(c.Approvals) > 256 {
		return nil, contextbundle.ErrDenied
	}
	resolved, err := filepath.EvalSymlinks(c.Root)
	if err != nil || resolved != c.Root {
		return nil, contextbundle.ErrUnsafePath
	}
	info, err := os.Stat(c.Root)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0o022 != 0 {
		return nil, contextbundle.ErrUnsafePath
	}
	p := &Preparer{worker: c.Worker, root: c.Root, approvals: map[string]Approval{}}
	approvals := make([]contextbundle.LocalApproval, 0, len(c.Approvals))
	for _, a := range c.Approvals {
		if a.RunID == "" || a.Repository == "" || !filepath.IsAbs(a.RepositoryPath) || !canonical.ValidDigest(a.TaskDigest) || !canonical.ValidDigest(a.InputDigest) || core.ValidateContextRefs(a.Refs) != nil {
			return nil, contextbundle.ErrDenied
		}
		if _, exists := p.approvals[a.InputDigest]; exists {
			return nil, contextbundle.ErrDenied
		}
		a.Refs = append([]core.ContextRef(nil), a.Refs...)
		p.approvals[a.InputDigest] = a
		approvals = append(approvals, contextbundle.LocalApproval{Subject: contextbundle.Subject{RunID: a.RunID, TaskContractDigest: a.TaskDigest, RunInputDigest: a.InputDigest}, Refs: a.Refs})
	}
	p.source, err = contextbundle.NewLocalSource(c.ContextSource, approvals)
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(c.Root)
	if err != nil {
		_ = p.source.Close()
		return nil, err
	}
	defer root.Close()
	for _, dir := range []string{"workspaces", "bundles"} {
		if err := root.Mkdir(dir, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
			_ = p.source.Close()
			return nil, err
		}
	}
	p.manager, err = workspace.NewWithGit(filepath.Join(c.Root, "workspaces"), c.Git)
	if err != nil {
		_ = p.source.Close()
		return nil, err
	}
	p.bundles, err = contextbundle.New(filepath.Join(c.Root, "bundles"), p.source, p.source, contextbundle.Limits{})
	if err != nil {
		_ = p.source.Close()
		return nil, err
	}
	return p, nil
}
func (p *Preparer) Close() error    { return errors.Join(p.bundles.Close(), p.source.Close()) }
func (p *Preparer) Subject() string { return p.worker }
func (p *Preparer) Prepare(ctx context.Context, subject string, a workerqueue.Assignment) (result Result, err error) {
	if p == nil || subject != p.worker {
		return result, contextbundle.ErrDenied
	}
	if _, err := workerqueue.Validate(a); err != nil {
		return result, err
	}
	approved, ok := p.approvals[a.Intent.InputDigest]
	if !ok || approved.RunID != a.Intent.RunID || approved.TaskDigest != a.Intent.TaskDigest || approved.Repository != a.Task.Repository || len(approved.Refs) != len(a.Input.ContextRefs) {
		return result, contextbundle.ErrDenied
	}
	for i, ref := range a.Input.ContextRefs {
		if ref != approved.Refs[i] || ref.Trust != core.ContextApproved {
			return result, contextbundle.ErrDenied
		}
	}
	// Machine locators are deliberately excluded from the approval/Run identities.
	approvalIdentity := struct {
		Worker      string            `json:"worker_subject"`
		RunID       string            `json:"run_id"`
		InputDigest string            `json:"run_input_manifest_digest"`
		TaskDigest  string            `json:"task_contract_digest"`
		Repository  string            `json:"repository"`
		Refs        []core.ContextRef `json:"context_refs"`
	}{subject, approved.RunID, approved.InputDigest, approved.TaskDigest, approved.Repository, approved.Refs}
	approvalDigest, err := canonical.Digest(approvalIdentity)
	if err != nil {
		return result, err
	}
	slot, err := canonical.Digest(struct {
		Worker string
		Token  workerqueue.Token
		Intent string
	}{subject, a.Token, a.IntentDigest})
	if err != nil {
		return result, err
	}
	w, err := p.manager.Create(ctx, workspace.Spec{ID: strings.TrimPrefix(slot, "sha256:"), Repository: approved.RepositoryPath, BaseCommit: a.Task.BaseCommit})
	if err != nil {
		return result, err
	}
	ready := false
	defer func() {
		if !ready {
			err = errors.Join(err, p.manager.Cleanup(context.Background(), w))
		}
	}()
	bundle, err := p.bundles.Materialize(ctx, a.Input, w.WorktreePath)
	if errors.Is(err, contextbundle.ErrExists) {
		bundle, err = p.bundles.Verify(ctx, a.Input)
	}
	if err != nil {
		return result, err
	}
	checked, err := p.bundles.Verify(ctx, a.Input)
	if err != nil || checked.Digest != bundle.Digest {
		return result, contextbundle.ErrDigest
	}
	clean, err := p.manager.IsClean(ctx, w)
	if err != nil {
		return result, err
	}
	if !clean {
		return result, workspace.ErrDirtyWorkspace
	}
	result = Result{Workspace: w, BundlePath: checked.Path, Facts: Facts{Version: 1, IntentDigest: a.IntentDigest, InputDigest: a.Intent.InputDigest, TaskDigest: a.Intent.TaskDigest, ApprovalDigest: approvalDigest, BaseCommit: w.BaseCommit, TreeCommit: w.TreeCommit, WorkspaceRecipe: workspace.Recipe, SourceDigest: w.SourceDigest, ConfigDigest: w.ConfigDigest, BundleDigest: checked.Digest, Context: checked.Manifest}}
	if err := result.Facts.Check(a); err != nil {
		return result, err
	}
	// An operator can locate/reconcile a prepared slot after a transport ambiguity.
	// This is a local locator record, not part of the remotely retained identity.
	data, err := json.Marshal(result)
	if err != nil {
		return result, err
	}
	root, err := os.OpenRoot(filepath.Join(p.root, "workspaces", w.ID))
	if err != nil {
		return result, err
	}
	defer root.Close()
	file, err := root.OpenFile("prepared.json", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return result, err
	}
	_, writeErr := file.Write(data)
	syncErr := file.Sync()
	closeErr := file.Close()
	if err = errors.Join(writeErr, syncErr, closeErr); err != nil {
		return result, err
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	ready = true
	return result, nil
}
func (p *Preparer) Recheck(ctx context.Context, a workerqueue.Assignment, result Result) error {
	if result.Facts.SourceDigest != result.Workspace.SourceDigest || result.Facts.ConfigDigest != result.Workspace.ConfigDigest || result.Facts.TreeCommit != result.Workspace.TreeCommit {
		return workerqueue.ErrIdentity
	}
	if err := result.Facts.Check(a); err != nil {
		return err
	}
	bundle, err := p.bundles.Verify(ctx, a.Input)
	if err != nil || bundle.Digest != result.Facts.BundleDigest {
		return contextbundle.ErrDigest
	}
	clean, err := p.manager.IsClean(ctx, result.Workspace)
	if err != nil {
		return err
	}
	if !clean {
		return workspace.ErrDirtyWorkspace
	}
	return nil
}
func (p *Preparer) Cleanup(ctx context.Context, result Result) error {
	if p == nil {
		return fmt.Errorf("preparer required")
	}
	return p.manager.Cleanup(ctx, result.Workspace)
}
