package preparation

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/jiying2007/engineering-platform/internal/access"
	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/contextbundle"
	"github.com/jiying2007/engineering-platform/internal/core"
	"github.com/jiying2007/engineering-platform/internal/strictjson"
	"github.com/jiying2007/engineering-platform/internal/workerqueue"
)

// Reopen re-authorizes the exact frozen inputs against the currently loaded
// operator snapshot, locates ONLY its derived owned slot, and rehashes bytes.
// It grants no execution authority and never silently creates another workspace.
func (p *Preparer) Reopen(ctx context.Context, subject string, a workerqueue.Assignment, factsDigest string) (Result, error) {
	var r Result
	if p == nil || subject != p.worker {
		return r, contextbundle.ErrDenied
	}
	if _, err := workerqueue.Validate(a); err != nil {
		return r, err
	}
	approved, ok := p.approvals[a.Intent.InputDigest]
	if !ok || approved.RunID != a.Intent.RunID || approved.TaskDigest != a.Intent.TaskDigest || approved.Repository != a.Task.Repository || len(approved.Refs) != len(a.Input.ContextRefs) {
		return r, contextbundle.ErrDenied
	}
	for i, ref := range a.Input.ContextRefs {
		if ref != approved.Refs[i] || ref.Trust != core.ContextApproved {
			return r, contextbundle.ErrDenied
		}
	}
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
		return r, err
	}
	slot, err := canonical.Digest(struct {
		Worker string
		Token  workerqueue.Token
		Intent string
	}{subject, a.Token, a.IntentDigest})
	if err != nil {
		return r, err
	}
	name := filepath.Join(p.root, "workspaces", strings.TrimPrefix(slot, "sha256:"), "prepared.json")
	data, err := access.ReadConfiguration(name, false)
	if err != nil {
		return r, err
	}
	if strictjson.Decode(data, &r) != nil {
		return r, workerqueue.ErrIdentity
	}
	actual, err := canonical.Digest(r.Facts)
	if err != nil || actual != factsDigest || r.Facts.ApprovalDigest != approvalDigest {
		return r, workerqueue.ErrIdentity
	}
	bundle, err := p.bundles.Verify(ctx, a.Input)
	if err != nil {
		return r, err
	}
	if r.BundlePath != bundle.Path {
		return r, workerqueue.ErrIdentity
	}
	if err = p.Recheck(ctx, a, r); err != nil {
		return r, err
	}
	return r, nil
}

// SaveOffline retains exact local reconciliation bytes before reporting. Names
// are deterministic fixed-alphabet IDs, never paths supplied by a model.
func (p *Preparer) SaveOffline(ctx context.Context, a workerqueue.Assignment, r Result, id string, value any) error {
	if len(id) != 64 || strings.Trim(id, "0123456789abcdef") != "" {
		return workerqueue.ErrIdentity
	}
	if err := p.Recheck(ctx, a, r); err != nil {
		return err
	}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return saveOfflineRecord(filepath.Join(p.root, "workspaces", r.Workspace.ID), "offline-"+id+".json", data)
}

func saveOfflineRecord(dir, name string, data []byte) error {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer root.Close()
	file, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(data)
	syncErr := file.Sync()
	closeErr := file.Close()
	return errors.Join(writeErr, syncErr, closeErr)
}
